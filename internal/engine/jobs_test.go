package engine

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/queue"
)

func TestRegisterRunsThroughQueue(t *testing.T) {
	h := newHarness(t, parse(t, "id: f\ntrigger: {on: asset.uploaded}\nsteps: [{use: record, with: {a: '{{ asset.id }}'}}]\n"))
	q := queue.New(h.db, slog.New(slog.DiscardHandler), queue.Options{Workers: 1, Poll: 5 * time.Millisecond})
	h.e.Register(q)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = q.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	if err := Publish(ctx, q, events.Event{Type: events.AssetUploaded, AssetIDs: []string{"a1", "a2"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, _ := q.Stats(ctx); s["done"] == 3 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s, _ := q.Stats(ctx)
	t.Fatalf("jobs: %v", s)
}

func TestRunFlow(t *testing.T) {
	h := newHarness(t, parse(t, "id: f\ntrigger: {on: asset.uploaded}\nsteps: [{use: record, with: {t: '{{ trigger.type }}'}}]\n"))
	q := queue.New(h.db, slog.New(slog.DiscardHandler), queue.Options{Workers: 1, Poll: 5 * time.Millisecond})
	h.e.Register(q)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = q.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	if _, err := h.e.RunFlow(ctx, q, "nope", "u", []string{"a"}); !errors.Is(err, ErrUnknownFlow) {
		t.Errorf("unknown flow: %v", err)
	}
	n, err := h.e.RunFlow(ctx, q, "f", "u", []string{"a", "b"})
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s, _ := q.Stats(ctx); s["done"] == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s, _ := q.Stats(ctx); s["done"] != 2 {
		t.Fatalf("jobs %v", s)
	}
}

type recordQueue struct{ reqs []RunRequest }

func (r *recordQueue) Enqueue(_ context.Context, kind string, p any) (int64, error) {
	if kind == JobRun {
		r.reqs = append(r.reqs, p.(RunRequest))
	}
	return int64(len(r.reqs)), nil
}

func TestTrigger(t *testing.T) {
	h := newHarness(t, parse(t, "id: w\ntrigger: {on: webhook}\nsteps: [{use: record}]\n"),
		parse(t, "id: m\ntrigger: {on: manual}\nsteps: [{use: record}]\n"))
	q := &recordQueue{}
	ctx := context.Background()
	ev := events.Event{Type: events.Webhook, UserID: "u"}
	if _, err := h.e.Trigger(ctx, q, "m", ev); !errors.Is(err, ErrUnknownFlow) {
		t.Errorf("other trigger: %v", err)
	}
	if n, err := h.e.Trigger(ctx, q, "w", ev); err != nil || n != 1 || q.reqs[0].AssetID != "" {
		t.Errorf("no assets: %d %v %+v", n, err, q.reqs)
	}
	ev.AssetIDs = []string{"a", "b"}
	if n, err := h.e.Trigger(ctx, q, "w", ev); err != nil || n != 2 || q.reqs[2].AssetID != "b" {
		t.Errorf("assets: %d %v %+v", n, err, q.reqs)
	}
}
