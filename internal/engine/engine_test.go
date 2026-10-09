package engine

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/flow"
	"github.com/serraniel/immich-flows/internal/queue"
	"github.com/serraniel/immich-flows/internal/store"
)

func parse(t *testing.T, src string) *flow.Flow {
	t.Helper()
	f, err := flow.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

type harness struct {
	db    *sql.DB
	e     *Engine
	calls []map[string]any
}

func newHarness(t *testing.T, fs ...*flow.Flow) *harness {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h := &harness{db: db}
	reg := NewRegistry()
	reg.Register(StepType{Name: "record", Run: func(_ context.Context, _ *StepContext, with map[string]any) (map[string]any, error) {
		h.calls = append(h.calls, with)
		return with, nil
	}})
	reg.Register(StepType{Name: "fail", Run: func(context.Context, *StepContext, map[string]any) (map[string]any, error) {
		return nil, errors.New("boom")
	}})
	for _, f := range fs {
		if err := reg.Validate(f); err != nil {
			t.Fatal(err)
		}
	}
	h.e = New(reg, db, StaticFlows(fs), Deps{
		LoadAsset: func(_ context.Context, _, id string) (map[string]any, error) {
			return map[string]any{"id": id, "originalFileName": "SERVICE-A_1_cat_dog.jpg"}, nil
		},
	}, slog.New(slog.DiscardHandler))
	return h
}

func (h *harness) steps(t *testing.T) []string {
	t.Helper()
	rows, err := h.db.Query(`SELECT use || ':' || status FROM run_steps ORDER BY run_id, seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

func (h *harness) runStatus(t *testing.T) string {
	t.Helper()
	var s string
	if err := h.db.QueryRow(`SELECT group_concat(status) FROM runs`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDispatch(t *testing.T) {
	h := newHarness(t,
		parse(t, "id: a\ntrigger: {on: asset.uploaded}\nsteps: [{use: log, with: {message: x}}]\n"),
		parse(t, "id: b\ntrigger: {on: asset.trashed}\nsteps: [{use: log, with: {message: x}}]\n"),
		parse(t, "id: c\nenabled: false\ntrigger: {on: asset.uploaded}\nsteps: [{use: log, with: {message: x}}]\n"),
	)
	reqs := h.e.Dispatch(events.Event{Type: events.AssetUploaded, AssetIDs: []string{"1", "2"}})
	if len(reqs) != 2 || reqs[0].FlowID != "a" || reqs[1].AssetID != "2" {
		t.Errorf("%+v", reqs)
	}
}

func TestExecuteBranchesAndOutputs(t *testing.T) {
	h := newHarness(t, parse(t, `id: f
trigger:
  on: asset.uploaded
  if: 'asset.originalFileName endsWith ".jpg"'
steps:
  - id: first
    use: record
    with: {name: '{{ asset.originalFileName }}'}
  - use: record
    if: 'false'
  - use: switch
    cases:
      - if: 'asset.originalFileName startsWith "SERVICE-B"'
        steps: [{use: record, with: {branch: b}}]
      - if: 'asset.originalFileName startsWith "SERVICE-A"'
        steps:
          - use: record
            with: {branch: a, prev: '{{ steps.first.name }}', user: '{{ trigger.userId }}'}
      - steps: [{use: record, with: {branch: default}}]
`))
	err := h.e.Execute(context.Background(), RunRequest{FlowID: "f", AssetID: "x", Event: events.Event{Type: events.AssetUploaded, UserID: "u1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 2 || h.calls[1]["branch"] != "a" || h.calls[1]["prev"] != "SERVICE-A_1_cat_dog.jpg" || h.calls[1]["user"] != "u1" {
		t.Errorf("calls: %+v", h.calls)
	}
	if got := strings.Join(h.steps(t), " "); got != "record:success record:skipped switch:success record:success" {
		t.Errorf("steps: %s", got)
	}
	if h.runStatus(t) != "success" {
		t.Errorf("run: %s", h.runStatus(t))
	}
}

func TestTriggerIfFalseRecordsNothing(t *testing.T) {
	h := newHarness(t, parse(t, "id: f\ntrigger: {on: manual, if: 'asset.id == \"other\"'}\nsteps: [{use: record}]\n"))
	if err := h.e.Execute(context.Background(), RunRequest{FlowID: "f", AssetID: "x", Event: events.Event{Type: events.Manual}}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = h.db.QueryRow(`SELECT count(*) FROM runs`).Scan(&n)
	if n != 0 || len(h.calls) != 0 {
		t.Errorf("runs=%d calls=%d", n, len(h.calls))
	}
}

func TestFailureStopsUnlessContinueOnError(t *testing.T) {
	h := newHarness(t, parse(t, `id: f
trigger: {on: manual}
steps:
  - {use: fail, continueOnError: true}
  - {use: record}
  - {use: fail}
  - {use: record}
`))
	err := h.e.Execute(context.Background(), RunRequest{FlowID: "f", Event: events.Event{Type: events.Manual}})
	if err == nil || queue.IsPermanent(err) {
		t.Fatalf("err %v", err)
	}
	if got := strings.Join(h.steps(t), " "); got != "fail:failed record:success fail:failed" {
		t.Errorf("steps: %s", got)
	}
	if h.runStatus(t) != "failed" {
		t.Errorf("run: %s", h.runStatus(t))
	}
}

func TestRuntimeExpressionErrorIsPermanent(t *testing.T) {
	h := newHarness(t, parse(t, "id: f\ntrigger: {on: manual}\nsteps: [{use: record, with: {x: '{{ 1 / nope.x }}'}}]\n"))
	err := h.e.Execute(context.Background(), RunRequest{FlowID: "f", Event: events.Event{Type: events.Manual}})
	if err == nil || !queue.IsPermanent(err) {
		t.Errorf("want permanent error, got %v", err)
	}
}

func TestValidate(t *testing.T) {
	reg := NewRegistry()
	f := parse(t, `id: f
trigger: {on: manual, if: '(('}
steps:
  - use: nope
  - use: log
    with: {}
  - use: log
    if: '))'
    with: {message: '{{ (( }}'}
`)
	err := reg.Validate(f)
	var errs flow.Errors
	if !errors.As(err, &errs) || len(errs) != 5 {
		t.Fatalf("%v", err)
	}
	for _, want := range []string{"2:10: trigger.if", `4:5: unknown step "nope"`, "5:5: log: with.message is required", "7:5: if:", "7:5: with:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in\n%v", want, err)
		}
	}
}

func TestUploadWaitsForImmichProcessing(t *testing.T) {
	h := newHarness(t, parse(t, "id: f\ntrigger: {on: asset.uploaded}\nsteps: [{use: record}]\n"))
	exif := map[string]any{"modifyDate": nil}
	h.e.deps.LoadAsset = func(_ context.Context, _, id string) (map[string]any, error) {
		return map[string]any{"id": id, "exifInfo": exif}, nil
	}
	req := RunRequest{FlowID: "f", AssetID: "a", Event: events.Event{Type: events.AssetUploaded, Time: time.Now()}}

	if err := h.e.Execute(context.Background(), req); err == nil || len(h.calls) != 0 {
		t.Fatalf("want snooze before processing, got %v calls=%d", err, len(h.calls))
	}
	exif["modifyDate"] = "2026-01-01T00:00:00Z"
	if err := h.e.Execute(context.Background(), req); err != nil || len(h.calls) != 1 {
		t.Fatalf("after processing: %v calls=%d", err, len(h.calls))
	}

	exif["modifyDate"] = nil
	req.Event.Time = time.Now().Add(-25 * time.Hour)
	if err := h.e.Execute(context.Background(), req); err != nil || len(h.calls) != 2 {
		t.Fatalf("old event must run anyway: %v calls=%d", err, len(h.calls))
	}
}
