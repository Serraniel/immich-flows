// Package queue is a persistent job queue on SQLite with retries and exponential backoff.
package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// Job is a unit of work passed to a Handler.
type Job struct {
	ID      int64
	Kind    string
	Payload json.RawMessage
	Attempt int // 1-based

	maxAttempts int
}

// Handler processes jobs of one kind. Returning an error retries the job, unless it is Permanent.
type Handler func(ctx context.Context, job Job) error

type permanent struct{ error }

func (p permanent) Unwrap() error { return p.error }

// Permanent marks err as not retryable.
func Permanent(err error) error { return permanent{err} }

// Options configures a Queue.
type Options struct {
	Workers     int           // default 4
	MaxAttempts int           // default 8
	BaseBackoff time.Duration // default 5s, doubled per attempt
	MaxBackoff  time.Duration // default 1h
	Poll        time.Duration // default 1s
	Retention   time.Duration // done jobs are deleted after this; default 7 days
}

// Queue stores jobs and runs them with registered handlers.
type Queue struct {
	db   *sql.DB
	log  *slog.Logger
	opts Options
	now  func() time.Time

	mu       sync.RWMutex
	handlers map[string]Handler
	wake     chan struct{}
}

// New returns a Queue. Call Handle for each kind, then Run.
func New(db *sql.DB, log *slog.Logger, opts Options) *Queue {
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 8
	}
	if opts.BaseBackoff <= 0 {
		opts.BaseBackoff = 5 * time.Second
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = time.Hour
	}
	if opts.Poll <= 0 {
		opts.Poll = time.Second
	}
	if opts.Retention <= 0 {
		opts.Retention = 7 * 24 * time.Hour
	}
	return &Queue{db: db, log: log, opts: opts, now: time.Now, handlers: map[string]Handler{}, wake: make(chan struct{}, 1)}
}

// Handle registers the handler for kind.
func (q *Queue) Handle(kind string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[kind] = h
}

// Enqueue stores a job. It is persisted when Enqueue returns.
func (q *Queue) Enqueue(ctx context.Context, kind string, payload any) (int64, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	now := q.now().UnixMilli()
	res, err := q.db.ExecContext(ctx,
		`INSERT INTO jobs (kind, payload, max_attempts, run_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		kind, data, q.opts.MaxAttempts, now, now, now)
	if err != nil {
		return 0, err
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return res.LastInsertId()
}

// Run processes jobs until ctx is cancelled. Jobs left running by a previous process are retried.
func (q *Queue) Run(ctx context.Context) error {
	if _, err := q.db.ExecContext(ctx, `UPDATE jobs SET status = 'pending' WHERE status = 'running'`); err != nil {
		return err
	}
	var wg sync.WaitGroup
	for range q.opts.Workers {
		wg.Go(func() { q.worker(ctx) })
	}
	wg.Go(func() { q.cleanup(ctx) })
	wg.Wait()
	return nil
}

func (q *Queue) worker(ctx context.Context) {
	t := time.NewTicker(q.opts.Poll)
	defer t.Stop()
	for {
		for {
			job, err := q.claim(ctx)
			if err != nil {
				if ctx.Err() == nil {
					q.log.Error("claim job", "err", err)
				}
				break
			}
			if job == nil {
				break
			}
			q.process(ctx, *job)
		}
		select {
		case <-ctx.Done():
			return
		case <-q.wake:
		case <-t.C:
		}
	}
}

func (q *Queue) claim(ctx context.Context) (*Job, error) {
	now := q.now().UnixMilli()
	var j Job
	err := q.db.QueryRowContext(ctx, `
		UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = ?
		WHERE id = (SELECT id FROM jobs WHERE status = 'pending' AND run_at <= ? ORDER BY run_at, id LIMIT 1)
		RETURNING id, kind, payload, attempts, max_attempts`, now, now).Scan(&j.ID, &j.Kind, &j.Payload, &j.Attempt, &j.maxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (q *Queue) process(ctx context.Context, j Job) {
	q.mu.RLock()
	h := q.handlers[j.Kind]
	q.mu.RUnlock()

	var err error
	if h == nil {
		err = Permanent(fmt.Errorf("no handler for kind %q", j.Kind))
	} else {
		err = safe(ctx, h, j)
	}
	if ctx.Err() != nil && err != nil {
		// Shutdown: leave as running; Run resets it on next start.
		return
	}

	// Use a fresh context so the result is recorded even if ctx is cancelled after the handler finished.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := q.now()
	switch {
	case err == nil:
		_, err = q.db.ExecContext(wctx, `UPDATE jobs SET status = 'done', last_error = NULL, updated_at = ? WHERE id = ?`, now.UnixMilli(), j.ID)
	case IsPermanent(err) || j.Attempt >= j.maxAttempts:
		q.log.Error("job failed permanently", "id", j.ID, "kind", j.Kind, "attempt", j.Attempt, "err", err)
		_, err = q.db.ExecContext(wctx, `UPDATE jobs SET status = 'dead', last_error = ?, updated_at = ? WHERE id = ?`, err.Error(), now.UnixMilli(), j.ID)
	default:
		next := now.Add(q.backoff(j.Attempt))
		q.log.Warn("job failed, retrying", "id", j.ID, "kind", j.Kind, "attempt", j.Attempt, "next", next, "err", err)
		_, err = q.db.ExecContext(wctx, `UPDATE jobs SET status = 'pending', last_error = ?, run_at = ?, updated_at = ? WHERE id = ?`,
			err.Error(), next.UnixMilli(), now.UnixMilli(), j.ID)
	}
	if err != nil {
		q.log.Error("record job result", "id", j.ID, "err", err)
	}
}

func safe(ctx context.Context, h Handler, j Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = Permanent(fmt.Errorf("panic: %v", r))
		}
	}()
	return h(ctx, j)
}

// backoff returns BaseBackoff * 2^(attempt-1), capped at MaxBackoff, with ±20% jitter.
func (q *Queue) backoff(attempt int) time.Duration {
	d := q.opts.BaseBackoff
	for i := 1; i < attempt && d < q.opts.MaxBackoff; i++ {
		d *= 2
	}
	d = min(d, q.opts.MaxBackoff)
	jitter := time.Duration(float64(d) * 0.2 * (rand.Float64()*2 - 1)) // #nosec G404 -- jitter only
	return d + jitter
}

func (q *Queue) cleanup(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		cutoff := q.now().Add(-q.opts.Retention).UnixMilli()
		if _, err := q.db.ExecContext(ctx, `DELETE FROM jobs WHERE status = 'done' AND updated_at < ?`, cutoff); err != nil && ctx.Err() == nil {
			q.log.Error("cleanup jobs", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Stats counts jobs by status.
func (q *Queue) Stats(ctx context.Context) (map[string]int, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT status, count(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// IsPermanent reports whether err or an error it wraps was marked with Permanent.
func IsPermanent(err error) bool {
	return errors.As(err, new(permanent))
}
