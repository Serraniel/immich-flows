package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/serraniel/immich-flows/internal/eval"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/flow"
	"github.com/serraniel/immich-flows/internal/queue"
)

const (
	runTimeout = 10 * time.Minute
	maxOutput  = 64 << 10

	// Immich's metadata extraction overwrites tags set before it ran, so upload runs wait for it.
	// The re-check delay grows with the event's age: quick for single uploads, gentle on Immich
	// during large imports.
	minProcessingSnooze = 2 * time.Second
	maxProcessingSnooze = 5 * time.Minute
	maxProcessingWait   = 24 * time.Hour
)

// FlowSource provides the current flows.
type FlowSource interface {
	Flows() []*flow.Flow
	Flow(id string) (*flow.Flow, bool)
}

// StaticFlows is a fixed FlowSource.
type StaticFlows []*flow.Flow

// Flows implements FlowSource.
func (s StaticFlows) Flows() []*flow.Flow { return s }

// Flow implements FlowSource.
func (s StaticFlows) Flow(id string) (*flow.Flow, bool) {
	for _, f := range s {
		if f.ID == id {
			return f, true
		}
	}
	return nil, false
}

// Deps are external services steps use. Fields may be nil in tests.
type Deps struct {
	// Immich returns a client acting as the given user.
	Immich ClientFunc
	// LoadAsset returns the asset as exposed to expressions (env.asset).
	LoadAsset func(ctx context.Context, userID, assetID string) (map[string]any, error)
}

// Engine dispatches events to flows and executes runs.
type Engine struct {
	reg   *Registry
	db    *sql.DB
	flows FlowSource
	deps  Deps
	log   *slog.Logger
	now   func() time.Time
}

// New returns an Engine.
func New(reg *Registry, db *sql.DB, flows FlowSource, deps Deps, log *slog.Logger) *Engine {
	return &Engine{reg: reg, db: db, flows: flows, deps: deps, log: log, now: time.Now}
}

// RunRequest is one flow run for one event (and asset).
type RunRequest struct {
	FlowID  string       `json:"flowId"`
	Event   events.Event `json:"event"`
	AssetID string       `json:"assetId,omitempty"`
}

// Dispatch returns the runs an event causes: one per enabled flow with a matching trigger and per asset.
func (e *Engine) Dispatch(ev events.Event) []RunRequest {
	var out []RunRequest
	for _, f := range e.flows.Flows() {
		if !f.IsEnabled() || f.Trigger.On != ev.Type {
			continue
		}
		if len(ev.AssetIDs) == 0 {
			out = append(out, RunRequest{FlowID: f.ID, Event: ev})
			continue
		}
		for _, id := range ev.AssetIDs {
			out = append(out, RunRequest{FlowID: f.ID, Event: ev, AssetID: id})
		}
	}
	return out
}

type stepError struct {
	step string
	err  error
}

func (s stepError) Error() string { return fmt.Sprintf("step %s: %v", s.step, s.err) }
func (s stepError) Unwrap() error { return s.err }

// Execute runs one flow. A returned error means the run failed and may be retried;
// errors wrapped with queue.Permanent will fail again on retry.
func (e *Engine) Execute(ctx context.Context, req RunRequest) error {
	f, ok := e.flows.Flow(req.FlowID)
	if !ok || !f.IsEnabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	env := map[string]any{
		"trigger": map[string]any{
			"type":   req.Event.Type,
			"time":   req.Event.Time,
			"userId": req.Event.UserID,
			"data":   req.Event.Data,
		},
		"steps": map[string]any{},
		"vars":  map[string]any{},
	}
	if req.AssetID != "" {
		asset := map[string]any{"id": req.AssetID}
		snap := snapshot(req.Event.Data, req.AssetID)
		switch {
		case snap != nil:
			// Taken by the hook before Immich changed the asset (e.g. deleted it).
			asset = snap
		case req.Event.Type == events.AssetDeleted:
			// Gone from Immich and no snapshot: only the ID is known.
		case e.deps.LoadAsset != nil:
			a, err := e.deps.LoadAsset(ctx, req.Event.UserID, req.AssetID)
			if err != nil {
				return fmt.Errorf("load asset %s: %w", req.AssetID, err)
			}
			asset = a
		}
		if req.Event.Type == events.AssetUploaded && !processed(asset) {
			if age := e.now().Sub(req.Event.Time); age < maxProcessingWait {
				return queue.Snooze(min(max(age/2, minProcessingSnooze), maxProcessingSnooze))
			}
			e.log.Warn("asset still not processed by Immich, running anyway", "asset", req.AssetID)
		}
		env["asset"] = asset
	}

	ok, err := eval.Bool(f.Trigger.If, env)
	if err != nil {
		return queue.Permanent(fmt.Errorf("flow %s trigger.if: %w", f.ID, err))
	}
	if !ok {
		return nil
	}

	r := &run{e: e, flow: f, req: req, env: env}
	if err := r.start(ctx); err != nil {
		return err
	}
	runErr := r.steps(ctx, f.Steps)
	if err := r.finish(runErr); err != nil {
		e.log.Error("record run", "flow", f.ID, "err", err)
	}
	if runErr != nil {
		e.log.Warn("run failed", "flow", f.ID, "run", r.id, "asset", req.AssetID, "err", runErr)
	}
	return runErr
}

// snapshot returns the asset captured in event data (data.assets[id]), if any.
func snapshot(data map[string]any, id string) map[string]any {
	assets, _ := data["assets"].(map[string]any)
	m, _ := assets[id].(map[string]any)
	return m
}

// Wants reports whether an enabled flow is triggered by eventType.
func (e *Engine) Wants(eventType string) bool {
	for _, f := range e.flows.Flows() {
		if f.IsEnabled() && f.Trigger.On == eventType {
			return true
		}
	}
	return false
}

// processed reports whether Immich finished metadata extraction: exifInfo.modifyDate is only
// set by it. Assets without exifInfo (tests, other sources) count as processed.
func processed(asset map[string]any) bool {
	exif, ok := asset["exifInfo"].(map[string]any)
	return !ok || exif["modifyDate"] != nil
}

type run struct {
	e    *Engine
	flow *flow.Flow
	req  RunRequest
	env  map[string]any
	id   int64
	seq  int
}

func (r *run) start(ctx context.Context) error {
	res, err := r.e.db.ExecContext(ctx,
		`INSERT INTO runs (flow_id, event_type, user_id, asset_id, status, started_at) VALUES (?, ?, ?, ?, 'running', ?)`,
		r.flow.ID, r.req.Event.Type, r.req.Event.UserID, nullable(r.req.AssetID), r.e.now().UnixMilli())
	if err != nil {
		return err
	}
	r.id, err = res.LastInsertId()
	return err
}

func (r *run) finish(runErr error) error {
	status, msg := "success", sql.NullString{}
	if runErr != nil {
		status, msg = "failed", sql.NullString{String: runErr.Error(), Valid: true}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.e.db.ExecContext(ctx, `UPDATE runs SET status = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, msg, r.e.now().UnixMilli(), r.id)
	return err
}

func (r *run) steps(ctx context.Context, steps []flow.Step) error {
	for i := range steps {
		if err := r.step(ctx, &steps[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) step(ctx context.Context, s *flow.Step) error {
	started := r.e.now()
	name := s.ID
	if name == "" {
		name = fmt.Sprintf("%s#%d", s.Use, r.seq+1)
	}

	ok, err := eval.Bool(s.If, r.env)
	if err != nil {
		err = queue.Permanent(fmt.Errorf("if: %w", err))
		r.record(s, "failed", nil, err, started)
		return stepError{name, err}
	}
	if !ok {
		r.record(s, "skipped", nil, nil, started)
		return nil
	}

	var out map[string]any
	if s.Use == flow.UseSwitch {
		out, err = r.switchStep(ctx, s, started)
	} else {
		out, err = r.call(ctx, s)
		r.record(s, status(err), out, err, started)
	}
	if s.ID != "" && out != nil {
		r.env["steps"].(map[string]any)[s.ID] = out
	}
	if err != nil && !s.ContinueOnError {
		return stepError{name, err}
	}
	return nil
}

func (r *run) switchStep(ctx context.Context, s *flow.Step, started time.Time) (map[string]any, error) {
	for i, c := range s.Cases {
		ok, err := eval.Bool(c.If, r.env)
		if err != nil {
			err = queue.Permanent(fmt.Errorf("case %d if: %w", i, err))
			r.record(s, "failed", nil, err, started)
			return nil, err
		}
		if ok {
			out := map[string]any{"case": i}
			r.record(s, "success", out, nil, started)
			return out, r.steps(ctx, c.Steps)
		}
	}
	out := map[string]any{"case": -1}
	r.record(s, "success", out, nil, started)
	return out, nil
}

func (r *run) call(ctx context.Context, s *flow.Step) (map[string]any, error) {
	t, ok := r.e.reg.Get(s.Use)
	if !ok {
		return nil, queue.Permanent(fmt.Errorf("unknown step %q", s.Use))
	}
	rendered, err := eval.Render(anyMap(s.With), r.env)
	if err != nil {
		return nil, queue.Permanent(fmt.Errorf("with: %w", err))
	}
	with, _ := rendered.(map[string]any)
	if with == nil {
		with = map[string]any{}
	}
	sc := &StepContext{
		FlowID:  r.flow.ID,
		UserID:  r.req.Event.UserID,
		AssetID: r.req.AssetID,
		Env:     r.env,
		Log:     r.e.log.With("flow", r.flow.ID, "run", r.id, "step", s.ID),
		Deps:    r.e.deps,
	}
	return t.Run(ctx, sc, with)
}

func (r *run) record(s *flow.Step, status string, out map[string]any, stepErr error, started time.Time) {
	r.seq++
	var outJSON, errMsg sql.NullString
	if out != nil {
		b, err := json.Marshal(out)
		if err != nil {
			b = []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
		}
		if len(b) > maxOutput {
			b = []byte(`{"truncated":true}`)
		}
		outJSON = sql.NullString{String: string(b), Valid: true}
	}
	if stepErr != nil {
		errMsg = sql.NullString{String: stepErr.Error(), Valid: true}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.e.db.ExecContext(ctx,
		`INSERT INTO run_steps (run_id, seq, step_id, use, status, output, error, started_at, duration_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.id, r.seq, nullable(s.ID), s.Use, status, outJSON, errMsg, started.UnixMilli(), r.e.now().Sub(started).Milliseconds())
	if err != nil {
		r.e.log.Error("record step", "run", r.id, "err", err)
	}
}

func status(err error) string {
	if err != nil {
		return "failed"
	}
	return "success"
}

func nullable(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
