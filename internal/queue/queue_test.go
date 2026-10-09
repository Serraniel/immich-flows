package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/store"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fast() Options {
	return Options{Workers: 4, MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond, Poll: 5 * time.Millisecond}
}

func start(t *testing.T, q *Queue) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = q.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return cancel
}

func waitStatus(t *testing.T, q *Queue, status string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s, err := q.Stats(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if s[status] == n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	s, _ := q.Stats(context.Background())
	t.Fatalf("want %d %s, have %v", n, status, s)
}

func TestProcessesEachJobOnce(t *testing.T) {
	q := New(openDB(t), slog.New(slog.DiscardHandler), fast())
	var mu sync.Mutex
	seen := map[int]int{}
	q.Handle("k", func(_ context.Context, j Job) error {
		var v int
		if err := json.Unmarshal(j.Payload, &v); err != nil {
			return err
		}
		mu.Lock()
		seen[v]++
		mu.Unlock()
		return nil
	})
	start(t, q)
	for i := range 200 {
		if _, err := q.Enqueue(context.Background(), "k", i); err != nil {
			t.Fatal(err)
		}
	}
	waitStatus(t, q, "done", 200)
	mu.Lock()
	defer mu.Unlock()
	for i := range 200 {
		if seen[i] != 1 {
			t.Fatalf("job %d ran %d times", i, seen[i])
		}
	}
}

func TestRetryThenSucceed(t *testing.T) {
	q := New(openDB(t), slog.New(slog.DiscardHandler), fast())
	var calls atomic.Int32
	q.Handle("k", func(_ context.Context, j Job) error {
		if calls.Add(1) < 3 {
			return errors.New("transient")
		}
		if j.Attempt != 3 {
			t.Errorf("attempt %d", j.Attempt)
		}
		return nil
	})
	start(t, q)
	_, _ = q.Enqueue(context.Background(), "k", nil)
	waitStatus(t, q, "done", 1)
}

func TestDeadAfterMaxAttemptsAndPermanent(t *testing.T) {
	db := openDB(t)
	q := New(db, slog.New(slog.DiscardHandler), fast())
	var retried, perm, panicked atomic.Int32
	q.Handle("retry", func(context.Context, Job) error { retried.Add(1); return errors.New("always") })
	q.Handle("perm", func(context.Context, Job) error { perm.Add(1); return Permanent(errors.New("bad input")) })
	q.Handle("panic", func(context.Context, Job) error { panicked.Add(1); panic("boom") })
	start(t, q)
	for _, k := range []string{"retry", "perm", "panic", "unknown"} {
		_, _ = q.Enqueue(context.Background(), k, nil)
	}
	waitStatus(t, q, "dead", 4)
	if retried.Load() != 3 || perm.Load() != 1 || panicked.Load() != 1 {
		t.Errorf("retried=%d perm=%d panicked=%d", retried.Load(), perm.Load(), panicked.Load())
	}
	var msg string
	_ = db.QueryRow(`SELECT last_error FROM jobs WHERE kind = 'unknown'`).Scan(&msg)
	if msg == "" {
		t.Error("last_error not recorded")
	}
}

func TestRecoversRunningJobs(t *testing.T) {
	db := openDB(t)
	q := New(db, slog.New(slog.DiscardHandler), fast())
	id, _ := q.Enqueue(context.Background(), "k", nil)
	// Simulate a crash mid-job.
	if _, err := db.Exec(`UPDATE jobs SET status = 'running', attempts = 1 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	q.Handle("k", func(context.Context, Job) error { return nil })
	start(t, q)
	waitStatus(t, q, "done", 1)
}

func TestShutdownLeavesJobForRetry(t *testing.T) {
	db := openDB(t)
	q := New(db, slog.New(slog.DiscardHandler), fast())
	started := make(chan struct{})
	q.Handle("k", func(ctx context.Context, _ Job) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	cancel := start(t, q)
	_, _ = q.Enqueue(context.Background(), "k", nil)
	<-started
	cancel()
	time.Sleep(50 * time.Millisecond)
	var status string
	_ = db.QueryRow(`SELECT status FROM jobs`).Scan(&status)
	if status != "running" {
		t.Errorf("status %q, want running (reset on next start)", status)
	}
}

func TestBackoff(t *testing.T) {
	q := New(nil, nil, Options{BaseBackoff: time.Second, MaxBackoff: 10 * time.Second})
	for attempt, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 4: 8 * time.Second, 10: 10 * time.Second} {
		got := q.backoff(attempt)
		if got < want*8/10 || got > want*12/10 {
			t.Errorf("attempt %d: %v, want ~%v", attempt, got, want)
		}
	}
}

func TestSnoozeDoesNotCountAttempts(t *testing.T) {
	q := New(openDB(t), slog.New(slog.DiscardHandler), fast())
	var calls atomic.Int32
	q.Handle("k", func(_ context.Context, j Job) error {
		if calls.Add(1) <= 5 {
			return Snooze(time.Millisecond)
		}
		if j.Attempt != 1 {
			t.Errorf("attempt %d after snoozes, want 1", j.Attempt)
		}
		return nil
	})
	start(t, q)
	_, _ = q.Enqueue(context.Background(), "k", nil)
	waitStatus(t, q, "done", 1)
}
