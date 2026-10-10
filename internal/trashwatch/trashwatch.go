// Package trashwatch detects assets that Immich deleted on its own, e.g. by trash auto-purge,
// and emits asset.deleted. Immich's sync API rejects API keys, so the trash of each connected
// user is reconciled periodically: assets that leave the trash and no longer exist were deleted.
package trashwatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/serraniel/immich-flows/internal/connect"
	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/immich"
)

const (
	pageSize = 1000
	maxTrash = 100000 // per user and pass
	maxCheck = 500    // assets checked per user and pass
)

// Deps are the watcher's dependencies.
type Deps struct {
	DB      *sql.DB
	Users   func(ctx context.Context) ([]string, error)
	Clients engine.ClientFunc
	Publish func(ctx context.Context, ev events.Event) error
	// Wants reports whether an enabled flow listens for asset.deleted. Nothing is tracked otherwise.
	Wants func(eventType string) bool
	Log   *slog.Logger
	Now   func() time.Time
}

// Watcher reconciles the trash of connected users.
type Watcher struct{ d Deps }

// New returns a Watcher.
func New(d Deps) *Watcher {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Watcher{d: d}
}

// Run reconciles every interval until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := w.Sync(ctx); err != nil && ctx.Err() == nil {
			w.d.Log.Error("trash reconcile", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Sync runs one pass for all connected users.
func (w *Watcher) Sync(ctx context.Context) error {
	if !w.d.Wants(events.AssetDeleted) {
		// Stale entries would emit late deletes once a flow is enabled again.
		_, err := w.d.DB.ExecContext(ctx, `DELETE FROM trash`)
		return err
	}
	users, err := w.d.Users(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, u := range users {
		if err := w.syncUser(ctx, u); err != nil {
			errs = append(errs, fmt.Errorf("user %s: %w", u, err))
		}
	}
	return errors.Join(errs...)
}

// Forget stops tracking assets whose deletion or restore the proxy already reported.
func (w *Watcher) Forget(ctx context.Context, userID string, assetIDs []string) error {
	for _, id := range assetIDs {
		if _, err := w.d.DB.ExecContext(ctx, `DELETE FROM trash WHERE user_id = ? AND asset_id = ?`, userID, id); err != nil {
			return err
		}
	}
	return nil
}

func (w *Watcher) syncUser(ctx context.Context, userID string) error {
	c, err := w.d.Clients(ctx, userID)
	if errors.Is(err, connect.ErrNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}
	current, complete, err := trashed(ctx, c)
	if err != nil {
		return err
	}
	if err := w.track(ctx, userID, current); err != nil {
		return err
	}

	gone, err := w.untracked(ctx, userID, current)
	if err != nil {
		return err
	}
	if !complete {
		// The listing was cut off; absent assets may still be in the trash.
		return nil
	}
	var deleted []string
	for _, id := range gone {
		state, err := check(ctx, c, id)
		if err != nil {
			w.d.Log.Warn("trash reconcile: check asset", "asset", id, "err", err)
			continue
		}
		switch state {
		case stateGone:
			deleted = append(deleted, id)
		case stateRestored:
			if err := w.Forget(ctx, userID, []string{id}); err != nil {
				return err
			}
		}
	}
	return w.emit(ctx, userID, deleted)
}

// track stores assets newly seen in the trash.
func (w *Watcher) track(ctx context.Context, userID string, current map[string]json.RawMessage) error {
	tx, err := w.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := w.d.Now().UnixMilli()
	for id, snap := range current {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO trash (user_id, asset_id, snapshot, seen_at) VALUES (?, ?, ?, ?)`,
			userID, id, []byte(snap), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// untracked returns tracked assets that are no longer in the trash, at most maxCheck.
func (w *Watcher) untracked(ctx context.Context, userID string, current map[string]json.RawMessage) ([]string, error) {
	rows, err := w.d.DB.QueryContext(ctx, `SELECT asset_id FROM trash WHERE user_id = ? ORDER BY seen_at`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() && len(out) < maxCheck {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if _, ok := current[id]; !ok {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

// emit publishes asset.deleted for assets still tracked, with their snapshots, and stops
// tracking them. Assets forgotten concurrently (reported by the proxy) are skipped.
func (w *Watcher) emit(ctx context.Context, userID string, ids []string) error {
	snaps := map[string]any{}
	var emitted []string
	for _, id := range ids {
		var raw []byte
		err := w.d.DB.QueryRowContext(ctx,
			`DELETE FROM trash WHERE user_id = ? AND asset_id = ? RETURNING snapshot`, userID, id).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			snaps[id] = m
		}
		emitted = append(emitted, id)
	}
	if len(emitted) == 0 {
		return nil
	}
	ev := events.Event{
		Type: events.AssetDeleted, Time: w.d.Now().UTC(), UserID: userID, AssetIDs: emitted,
		Data: map[string]any{"assets": snaps, "source": "reconcile"},
	}
	if err := w.d.Publish(ctx, ev); err != nil {
		// Track again so the next pass retries.
		for id, s := range snaps {
			b, _ := json.Marshal(s)
			_, _ = w.d.DB.ExecContext(ctx,
				`INSERT OR IGNORE INTO trash (user_id, asset_id, snapshot, seen_at) VALUES (?, ?, ?, ?)`,
				userID, id, b, w.d.Now().UnixMilli())
		}
		return err
	}
	w.d.Log.Info("assets deleted by Immich", "user", userID, "count", len(emitted))
	return nil
}

// trashed lists the user's trashed assets as raw JSON by ID. complete is false if the
// listing was cut off at maxTrash.
func trashed(ctx context.Context, c *immich.ClientWithResponses) (map[string]json.RawMessage, bool, error) {
	out := map[string]json.RawMessage{}
	withDeleted, withExif, size := true, true, pageSize
	epoch := time.Unix(0, 0).UTC()
	for page := 1; ; page++ {
		if len(out) >= maxTrash {
			return out, false, nil
		}
		p := page
		res, err := c.SearchAssetsWithResponse(ctx, nil, immich.MetadataSearchDto{
			WithDeleted: &withDeleted, WithExif: &withExif, TrashedAfter: &epoch, Page: &p, Size: &size,
		})
		if err != nil {
			return nil, false, err
		}
		if err := immich.Check(res, res.Body); err != nil {
			return nil, false, err
		}
		var body struct {
			Assets struct {
				Items    []json.RawMessage `json:"items"`
				NextPage *string           `json:"nextPage"`
			} `json:"assets"`
		}
		if err := json.Unmarshal(res.Body, &body); err != nil {
			return nil, false, err
		}
		for _, item := range body.Assets.Items {
			var a struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(item, &a) == nil && a.ID != "" {
				out[a.ID] = item
			}
		}
		if body.Assets.NextPage == nil {
			return out, true, nil
		}
	}
}

type state int

const (
	stateTrashed state = iota
	stateRestored
	stateGone
)

// check reports what happened to an asset that left the trash listing.
func check(ctx context.Context, c *immich.ClientWithResponses, id string) (state, error) {
	var uid openapi_types.UUID
	if err := uid.UnmarshalText([]byte(id)); err != nil {
		return stateGone, nil
	}
	res, err := c.GetAssetInfoWithResponse(ctx, uid, nil)
	if err != nil {
		return 0, err
	}
	switch res.StatusCode() {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusBadRequest:
		// Immich answers 400 for assets that do not exist or are not accessible.
		return stateGone, nil
	default:
		return 0, immich.Check(res, res.Body)
	}
	var a struct {
		IsTrashed bool `json:"isTrashed"`
	}
	if err := json.Unmarshal(res.Body, &a); err != nil {
		return 0, err
	}
	if a.IsTrashed {
		return stateTrashed, nil
	}
	return stateRestored, nil
}
