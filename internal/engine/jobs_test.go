package engine

import (
	"context"
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
