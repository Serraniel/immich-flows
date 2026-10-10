// Package webhook receives events from Immich workflows, for assets that never pass the proxy
// (external libraries, server-side imports). Immich's core plugin step "Trigger Webhook" posts
// {type, trigger, data: {asset}} to the endpoint with a token header.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/queue"
)

const (
	// Header carries the token; configure it as the step's header name.
	Header = "X-Immich-Flows-Token"
	// Job processes a received upload after Delay.
	Job = "webhook.upload"
	// Delay lets the proxy report the same upload first; its event carries the original hashes.
	Delay = 30 * time.Second

	seenTTL = 7 * 24 * time.Hour
)

// Triggers reported as asset.uploaded. AssetTagged carries no tag details and is ignored.
var uploadTriggers = map[string]bool{"AssetCreate": true, "AssetMetadataExtraction": true}

// Token derives a user's webhook token from the instance key. Events are accepted only for
// assets owned by that user.
func Token(key []byte, userID string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("immich-flows webhook\x00" + userID))
	return hex.EncodeToString(m.Sum(nil))
}

type payload struct {
	Trigger string `json:"trigger"`
	Data    struct {
		Asset struct {
			ID      string `json:"id"`
			OwnerID string `json:"ownerId"`
		} `json:"asset"`
	} `json:"data"`
}

type upload struct {
	UserID     string    `json:"userId"`
	AssetID    string    `json:"assetId"`
	Trigger    string    `json:"trigger"`
	ReceivedAt time.Time `json:"receivedAt"`
}

// Handler serves the webhook endpoint.
type Handler struct {
	Key  []byte // instance key, see Token
	Jobs engine.Enqueuer
	// Wants reports whether an enabled flow listens for asset.uploaded.
	Wants func(eventType string) bool
	Log   *slog.Logger
	Now   func() time.Time
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var p payload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&p); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	a := p.Data.Asset
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(Header)), []byte(Token(h.Key, a.OwnerID))) != 1 {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if !uploadTriggers[p.Trigger] || a.ID == "" || a.OwnerID == "" || !h.Wants(events.AssetUploaded) {
		h.Log.Debug("webhook ignored", "trigger", p.Trigger, "asset", a.ID)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	if _, err := h.Jobs.Enqueue(r.Context(), Job, upload{UserID: a.OwnerID, AssetID: a.ID, Trigger: p.Trigger, ReceivedAt: now().UTC()}); err != nil {
		h.Log.Error("webhook enqueue", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// Seen records uploads that were reported, so webhook duplicates are dropped.
type Seen struct {
	DB  *sql.DB
	Now func() time.Time
}

func (s *Seen) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Mark records assets as reported.
func (s *Seen) Mark(ctx context.Context, assetIDs ...string) error {
	for _, id := range assetIDs {
		if _, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO seen_uploads (asset_id, seen_at) VALUES (?, ?)`, id, s.now().UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

// Register handles webhook upload jobs: after Delay, uploads not reported yet are published
// as asset.uploaded with trigger.data.source "webhook".
func Register(q *queue.Queue, seen *Seen, publish func(context.Context, events.Event) error) {
	q.Handle(Job, process(seen, publish))
}

func process(seen *Seen, publish func(context.Context, events.Event) error) queue.Handler {
	return func(ctx context.Context, j queue.Job) error {
		var u upload
		if err := json.Unmarshal(j.Payload, &u); err != nil {
			return queue.Permanent(err)
		}
		if wait := u.ReceivedAt.Add(Delay).Sub(seen.now()); wait > 0 {
			return queue.Snooze(wait)
		}
		if _, err := seen.DB.ExecContext(ctx, `DELETE FROM seen_uploads WHERE seen_at < ?`, seen.now().Add(-seenTTL).UnixMilli()); err != nil {
			return err
		}
		var n int
		if err := seen.DB.QueryRowContext(ctx, `SELECT count(*) FROM seen_uploads WHERE asset_id = ?`, u.AssetID).Scan(&n); err != nil || n > 0 {
			return err
		}
		if err := publish(ctx, events.Event{
			Type: events.AssetUploaded, Time: u.ReceivedAt, UserID: u.UserID, AssetIDs: []string{u.AssetID},
			Data: map[string]any{"source": "webhook", "immichTrigger": u.Trigger},
		}); err != nil {
			return err
		}
		return seen.Mark(ctx, u.AssetID)
	}
}
