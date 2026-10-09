// Package hooks turns proxied Immich API calls into events.
package hooks

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/serraniel/immich-flows/internal/auth"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/immich"
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
	// Immich is the direct Immich URL, used to read state with the caller's credentials.
	Immich *url.URL
	// Wants reports whether an enabled flow listens for the event type. Hooks skip extra
	// requests to Immich (snapshots, listings) when nothing listens.
	Wants func(eventType string) bool
}

func (d Deps) wants(eventType string) bool { return d.Wants == nil || d.Wants(eventType) }

// client returns an Immich client acting with the credentials of the proxied request.
func (d Deps) client(r *http.Request) (*immich.ClientWithResponses, error) {
	return immich.New(d.Immich, immich.Forward(r), nil)
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
