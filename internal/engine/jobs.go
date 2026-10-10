package engine

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/queue"
)

// Job kinds.
const (
	JobEvent = "event"    // payload: events.Event
	JobRun   = "flow.run" // payload: RunRequest
)

// Enqueuer stores jobs.
type Enqueuer interface {
	Enqueue(ctx context.Context, kind string, payload any) (int64, error)
}

// Publish stores an event for asynchronous dispatch.
func Publish(ctx context.Context, q Enqueuer, ev events.Event) error {
	_, err := q.Enqueue(ctx, JobEvent, ev)
	return err
}

// Register connects the engine to the queue: events fan out to one run job per flow and asset,
// so each run is retried independently.
func (e *Engine) Register(q *queue.Queue) {
	q.Handle(JobEvent, func(ctx context.Context, j queue.Job) error {
		var ev events.Event
		if err := json.Unmarshal(j.Payload, &ev); err != nil {
			return queue.Permanent(err)
		}
		for _, req := range e.Dispatch(ev) {
			if _, err := q.Enqueue(ctx, JobRun, req); err != nil {
				return err
			}
		}
		return nil
	})
	q.Handle(JobRun, func(ctx context.Context, j queue.Job) error {
		var req RunRequest
		if err := json.Unmarshal(j.Payload, &req); err != nil {
			return queue.Permanent(err)
		}
		return e.Execute(ctx, req)
	})
}

// RunFlow queues one run of flowID per asset, regardless of the flow's trigger. Used for manual
// runs and backfills; the event type is "manual".
func (e *Engine) RunFlow(ctx context.Context, q Enqueuer, flowID, userID string, assetIDs []string) (int, error) {
	if _, ok := e.flows.Flow(flowID); !ok {
		return 0, ErrUnknownFlow
	}
	ev := events.Event{Type: events.Manual, Time: e.now().UTC(), UserID: userID}
	for i, id := range assetIDs {
		if _, err := q.Enqueue(ctx, JobRun, RunRequest{FlowID: flowID, Event: ev, AssetID: id}); err != nil {
			return i, err
		}
	}
	return len(assetIDs), nil
}

// Trigger starts one flow for an event: one run per asset, or one without an asset.
// The flow must be enabled and triggered by ev.Type.
func (e *Engine) Trigger(ctx context.Context, q Enqueuer, flowID string, ev events.Event) (int, error) {
	f, ok := e.flows.Flow(flowID)
	if !ok || !f.IsEnabled() || f.Trigger.On != ev.Type {
		return 0, ErrUnknownFlow
	}
	ids := ev.AssetIDs
	if len(ids) == 0 {
		ids = []string{""}
	}
	for i, id := range ids {
		if _, err := q.Enqueue(ctx, JobRun, RunRequest{FlowID: flowID, Event: ev, AssetID: id}); err != nil {
			return i, err
		}
	}
	return len(ids), nil
}

// ErrUnknownFlow is returned for flow IDs that are not loaded or invalid.
var ErrUnknownFlow = errors.New("unknown flow")
