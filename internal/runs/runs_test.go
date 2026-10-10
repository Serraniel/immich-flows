package runs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/serraniel/immich-flows/internal/store"
)

func TestListGetRequest(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for i, r := range []struct{ flow, user, status string }{{"a", "u1", "success"}, {"b", "u2", "failed"}, {"a", "u1", "failed"}} {
		_, err := db.Exec(`INSERT INTO runs (id, flow_id, event_type, user_id, status, started_at, finished_at, request) VALUES (?, ?, 'manual', ?, ?, ?, ?, ?)`,
			i+1, r.flow, r.user, r.status, 1000*(i+1), 2000*(i+1), `{"flowId":"`+r.flow+`"}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _ = db.Exec(`INSERT INTO run_steps (run_id, seq, step_id, use, status, output, started_at, duration_ms) VALUES (3, 1, 'h', 'hash', 'success', '{"md5":"x"}', 3000, 5)`)
	_, _ = db.Exec(`INSERT INTO run_steps (run_id, seq, use, status, error, started_at, duration_ms) VALUES (3, 2, 'tag.assign', 'failed', 'boom', 3005, 1)`)

	s := New(db)
	ids := func(f Filter) (out []int64) {
		list, err := s.List(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range list {
			out = append(out, r.ID)
		}
		return out
	}
	if got := ids(Filter{}); len(got) != 3 || got[0] != 3 {
		t.Errorf("all: %v", got)
	}
	if got := ids(Filter{UserID: "u1", Status: "failed"}); len(got) != 1 || got[0] != 3 {
		t.Errorf("filter: %v", got)
	}
	if got := ids(Filter{Before: 3, Limit: 1}); len(got) != 1 || got[0] != 2 {
		t.Errorf("paging: %v", got)
	}

	r, err := s.Get(ctx, 3)
	if err != nil || len(r.Steps) != 2 || string(r.Steps[0].Output) != `{"md5":"x"}` || r.Steps[1].Error != "boom" || r.FinishedAt == nil {
		t.Fatalf("get: %+v %v", r, err)
	}
	var req struct{ FlowID string }
	if err := s.Request(ctx, 3, &req); err != nil || req.FlowID != "a" {
		t.Errorf("request: %+v %v", req, err)
	}
	if _, err := s.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}
