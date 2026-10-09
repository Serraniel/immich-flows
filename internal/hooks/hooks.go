// Package hooks turns proxied Immich API calls into events.
package hooks

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/serraniel/immich-flows/internal/auth"
	"github.com/serraniel/immich-flows/internal/events"
)

// Users resolves the Immich user behind a request.
type Users interface {
	Authenticate(r *http.Request) (*auth.User, error)
}

// Publisher stores an event for processing.
type Publisher func(ctx context.Context, ev events.Event) error

// Deps are shared by all hooks.
type Deps struct {
	Users   Users
	Publish Publisher
	Log     *slog.Logger
}

func (d Deps) publish(r *http.Request, ev events.Event) {
	u, err := d.Users.Authenticate(r)
	if err != nil {
		d.Log.Warn("event dropped: cannot resolve user", "type", ev.Type, "err", err)
		return
	}
	ev.UserID = u.ID
	// The client request may be finished; the event must still be stored.
	if err := d.Publish(context.WithoutCancel(r.Context()), ev); err != nil {
		d.Log.Error("publish event", "type", ev.Type, "err", err)
		return
	}
	d.Log.Debug("event", "type", ev.Type, "user", ev.UserID, "assets", ev.AssetIDs, "data", ev.Data)
}
