package engine

import (
	"context"
	"encoding/json"

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
