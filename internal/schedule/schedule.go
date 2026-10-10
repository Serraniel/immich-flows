// Package schedule starts flows with trigger.on: schedule at their cron times, once per
// connected user. Times are evaluated in the container's time zone (TZ). Runs missed while
// immich-flows was not running are skipped.
package schedule

import (
	"context"
	"log/slog"
	"time"

	"github.com/serraniel/immich-flows/internal/cron"
	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/flow"
)

// Deps are the scheduler's dependencies.
type Deps struct {
	Flows   func() []*flow.Flow
	Users   func(ctx context.Context) ([]string, error)
	Trigger func(ctx context.Context, q engine.Enqueuer, flowID string, ev events.Event) (int, error)
	Jobs    engine.Enqueuer
	Log     *slog.Logger
}

// Run ticks at every minute until ctx is cancelled.
func Run(ctx context.Context, d Deps) {
	for {
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		select {
		case <-ctx.Done():
			return
		case <-time.After(next.Sub(now)):
		}
		Tick(ctx, d, next)
	}
}

// Tick starts the flows scheduled at minute t.
func Tick(ctx context.Context, d Deps, t time.Time) {
	t = t.Truncate(time.Minute)
	var users []string
	for _, f := range d.Flows() {
		if !f.IsEnabled() || f.Trigger.On != events.Schedule {
			continue
		}
		s, err := cron.Parse(f.Trigger.Cron)
		if err != nil || !s.Match(t.In(time.Local)) {
			continue
		}
		if users == nil {
			if users, err = d.Users(ctx); err != nil {
				d.Log.Error("schedule: list users", "err", err)
				return
			}
		}
		for _, u := range users {
			ev := events.Event{Type: events.Schedule, Time: t.UTC(), UserID: u, Data: map[string]any{"scheduledAt": t.UTC()}}
			if _, err := d.Trigger(ctx, d.Jobs, f.ID, ev); err != nil {
				d.Log.Error("schedule: start flow", "flow", f.ID, "user", u, "err", err)
			}
		}
	}
}
