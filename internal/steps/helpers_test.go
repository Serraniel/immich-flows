package steps

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/flow"
	"github.com/serraniel/immich-flows/internal/store"
)

type result struct {
	err     error
	outputs map[string]map[string]any // step id -> output
	vars    map[string]any
}

// runFlow validates and executes a flow for one asset with deps, returning step outputs.
func runFlow(t *testing.T, src string, asset map[string]any, data map[string]any, deps engine.Deps) result {
	t.Helper()
	f, err := flow.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	reg := engine.NewRegistry()
	Register(reg)
	var vars map[string]any
	reg.Register(engine.StepType{Name: "capture", Run: func(_ context.Context, sc *engine.StepContext, _ map[string]any) (map[string]any, error) {
		vars = sc.Env["vars"].(map[string]any)
		return nil, nil
	}})
	if err := reg.Validate(f); err != nil {
		t.Fatalf("validate: %v", err)
	}
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if deps.LoadAsset == nil {
		deps.LoadAsset = func(context.Context, string, string) (map[string]any, error) { return asset, nil }
	}
	e := engine.New(reg, db, engine.StaticFlows{f}, deps, slog.New(slog.DiscardHandler))
	err = e.Execute(context.Background(), engine.RunRequest{
		FlowID:  f.ID,
		AssetID: "11111111-1111-4111-8111-111111111111",
		Event:   events.Event{Type: f.Trigger.On, UserID: "u1", Time: time.Now(), Data: data},
	})
	return result{err: err, outputs: outputs(t, db), vars: vars}
}

func outputs(t *testing.T, db *sql.DB) map[string]map[string]any {
	t.Helper()
	rows, err := db.Query(`SELECT step_id, output FROM run_steps WHERE step_id IS NOT NULL AND output IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]any{}
	for rows.Next() {
		var id, js string
		_ = rows.Scan(&id, &js)
		var m map[string]any
		_ = json.Unmarshal([]byte(js), &m)
		out[id] = m
	}
	return out
}
