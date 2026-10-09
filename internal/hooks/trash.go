package hooks

import (
	"context"
	"net/http"
	"time"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/proxy"
)

type trashKey struct{}

type trashState struct {
	ids       []string
	snapshots map[string]any
}

// Trash returns hooks for trashing, deleting and restoring assets.
// Deleted assets are snapshotted before deletion so flows can still read them (max 500).
func Trash(d Deps) []proxy.Hook {
	return []proxy.Hook{
		{
			Route: "DELETE /api/assets",
			Pre: func(ctx context.Context, ex *proxy.Exchange) error {
				var body struct {
					IDs   []string `json:"ids"`
					Force bool     `json:"force"`
				}
				if err := readJSON(ex.Request, &body); err != nil {
					return err
				}
				st := &trashState{ids: body.IDs}
				typ := events.AssetTrashed
				if body.Force {
					typ = events.AssetDeleted
					if d.wants(typ) {
						if err := d.snapshotInto(ctx, ex.Request, st); err != nil {
							return err
						}
					}
				}
				ex.Set(trashKey{}, st)
				ex.Set(typeKey{}, typ)
				return nil
			},
			Post: d.publishTrash(http.StatusNoContent),
		},
		d.listTrash("POST /api/trash/empty", events.AssetDeleted, true),
		d.listTrash("POST /api/trash/restore", events.AssetRestored, false),
		{
			Route: "POST /api/trash/restore/assets",
			Pre: func(_ context.Context, ex *proxy.Exchange) error {
				var body struct {
					IDs []string `json:"ids"`
				}
				if err := readJSON(ex.Request, &body); err != nil {
					return err
				}
				ex.Set(trashKey{}, &trashState{ids: body.IDs})
				ex.Set(typeKey{}, events.AssetRestored)
				return nil
			},
			Post: d.publishTrash(http.StatusOK),
		},
	}
}

type typeKey struct{}

// listTrash lists the caller's trash before Immich empties or restores it.
func (d Deps) listTrash(route, typ string, snap bool) proxy.Hook {
	return proxy.Hook{
		Route:   route,
		Timeout: 30 * time.Second,
		Pre: func(ctx context.Context, ex *proxy.Exchange) error {
			if !d.wants(typ) {
				return nil
			}
			c, err := d.client(ex.Request)
			if err != nil {
				return err
			}
			ids, err := trashedAssets(ctx, c)
			if err != nil {
				return err
			}
			st := &trashState{ids: ids}
			if snap {
				if err := d.snapshotInto(ctx, ex.Request, st); err != nil {
					return err
				}
			}
			ex.Set(trashKey{}, st)
			ex.Set(typeKey{}, typ)
			return nil
		},
		Post: d.publishTrash(http.StatusOK),
	}
}

func (d Deps) snapshotInto(ctx context.Context, r *http.Request, st *trashState) error {
	c, err := d.client(r)
	if err != nil {
		return err
	}
	st.snapshots = snapshot(ctx, c, st.ids)
	return nil
}

func (d Deps) publishTrash(okStatus int) func(*proxy.Exchange) {
	return func(ex *proxy.Exchange) {
		st, _ := ex.Get(trashKey{}).(*trashState)
		typ, _ := ex.Get(typeKey{}).(string)
		if st == nil || len(st.ids) == 0 || ex.Status != okStatus || !d.wants(typ) {
			return
		}
		ev := events.Event{Type: typ, Time: time.Now().UTC(), AssetIDs: st.ids}
		if len(st.snapshots) > 0 {
			ev.Data = map[string]any{"assets": st.snapshots}
		}
		d.publish(ex.Request, ev)
	}
}
