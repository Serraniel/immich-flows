package schedule

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/flow"
)

func TestTick(t *testing.T) {
	parse := func(src string) *flow.Flow {
		f, err := flow.Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	flows := []*flow.Flow{
		parse("id: nightly\ntrigger: {on: schedule, cron: '0 3 * * *'}\nsteps: [{use: log}]\n"),
		parse("id: quarter\ntrigger: {on: schedule, cron: '*/15 * * * *'}\nsteps: [{use: log}]\n"),
		parse("id: off\nenabled: false\ntrigger: {on: schedule, cron: '* * * * *'}\nsteps: [{use: log}]\n"),
		parse("id: manual\ntrigger: {on: manual}\nsteps: [{use: log}]\n"),
	}
	var started []string
	userCalls := 0
	d := Deps{
		Flows: func() []*flow.Flow { return flows },
		Users: func(context.Context) ([]string, error) { userCalls++; return []string{"u1", "u2"}, nil },
		Trigger: func(_ context.Context, _ engine.Enqueuer, id string, ev events.Event) (int, error) {
			if ev.Type != events.Schedule {
				t.Errorf("type %s", ev.Type)
			}
			started = append(started, id+"/"+ev.UserID)
			return 1, nil
		},
		Log: slog.New(slog.DiscardHandler),
	}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 10, h, m, 20, 0, time.Local) }

	Tick(context.Background(), d, at(3, 0))
	if len(started) != 4 || started[0] != "nightly/u1" || started[3] != "quarter/u2" || userCalls != 1 {
		t.Errorf("03:00: %v (users listed %d times)", started, userCalls)
	}
	started = nil
	Tick(context.Background(), d, at(3, 7))
	if len(started) != 0 || userCalls != 1 {
		t.Errorf("03:07: %v, users listed %d times", started, userCalls)
	}
}
