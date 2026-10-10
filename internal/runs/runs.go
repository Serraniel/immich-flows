// Package runs reads the run log.
package runs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrNotFound is returned for unknown runs.
var ErrNotFound = errors.New("run not found")

// Run is one flow execution.
type Run struct {
	ID         int64      `json:"id"`
	FlowID     string     `json:"flowId"`
	EventType  string     `json:"eventType"`
	UserID     string     `json:"userId"`
	AssetID    string     `json:"assetId,omitempty"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Steps      []Step     `json:"steps,omitempty"`

	request string
}

// Step is one step of a run.
type Step struct {
	Seq        int             `json:"seq"`
	StepID     string          `json:"stepId,omitempty"`
	Use        string          `json:"use"`
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	StartedAt  time.Time       `json:"startedAt"`
	DurationMs int64           `json:"durationMs"`
}

// Filter selects runs. Empty fields match all.
type Filter struct {
	UserID string // restrict to a user (non-admins)
	FlowID string
	Status string
	Before int64 // run ID for paging; 0 = newest
	Limit  int   // default 50, max 200
}

// Store reads runs.
type Store struct{ db *sql.DB }

// New returns a Store.
func New(db *sql.DB) *Store { return &Store{db: db} }

const cols = `id, flow_id, event_type, user_id, coalesce(asset_id, ''), status, coalesce(error, ''), started_at, finished_at, coalesce(request, '')`

func scanRun(sc interface{ Scan(...any) error }) (Run, error) {
	var r Run
	var started int64
	var finished sql.NullInt64
	if err := sc.Scan(&r.ID, &r.FlowID, &r.EventType, &r.UserID, &r.AssetID, &r.Status, &r.Error, &started, &finished, &r.request); err != nil {
		return r, err
	}
	r.StartedAt = time.UnixMilli(started).UTC()
	if finished.Valid {
		t := time.UnixMilli(finished.Int64).UTC()
		r.FinishedAt = &t
	}
	return r, nil
}

// List returns runs, newest first.
func (s *Store) List(ctx context.Context, f Filter) ([]Run, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		where = append(where, cond)
		args = append(args, v)
	}
	if f.UserID != "" {
		add("user_id = ?", f.UserID)
	}
	if f.FlowID != "" {
		add("flow_id = ?", f.FlowID)
	}
	if f.Status != "" {
		add("status = ?", f.Status)
	}
	if f.Before > 0 {
		add("id < ?", f.Before)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	q := "SELECT " + cols + " FROM runs"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ") // #nosec G202 -- constant fragments, values are bound
	}
	q += " ORDER BY id DESC LIMIT ?"
	rows, err := s.db.QueryContext(ctx, q, append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns a run with its steps.
func (s *Store) Get(ctx context.Context, id int64) (*Run, error) {
	r, err := scanRun(s.db.QueryRowContext(ctx, "SELECT "+cols+" FROM runs WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT seq, coalesce(step_id, ''), use, status, coalesce(output, ''), coalesce(error, ''), started_at, duration_ms
		FROM run_steps WHERE run_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	r.Steps = []Step{}
	for rows.Next() {
		var st Step
		var out string
		var started int64
		if err := rows.Scan(&st.Seq, &st.StepID, &st.Use, &st.Status, &out, &st.Error, &started, &st.DurationMs); err != nil {
			return nil, err
		}
		if out != "" {
			st.Output = json.RawMessage(out)
		}
		st.StartedAt = time.UnixMilli(started).UTC()
		r.Steps = append(r.Steps, st)
	}
	return &r, rows.Err()
}

// Request returns the original request of a run for retrying, decoded into v.
func (s *Store) Request(ctx context.Context, id int64, v any) error {
	r, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if r.request == "" {
		return errors.New("run has no stored request (started before retries were supported)")
	}
	return json.Unmarshal([]byte(r.request), v)
}
