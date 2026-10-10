package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
)

const maxAssets = 1000

// FlowToken returns the token that starts flowID as userID: "<userID>.<hmac>".
func FlowToken(key []byte, userID, flowID string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte("immich-flows trigger\x00" + userID + "\x00" + flowID))
	return userID + "." + hex.EncodeToString(m.Sum(nil))
}

// Triggerer starts a single flow.
type Triggerer interface {
	Trigger(ctx context.Context, q engine.Enqueuer, flowID string, ev events.Event) (int, error)
}

// FlowHandler serves POST <prefix>/flows/{id}/webhook for flows with trigger.on: webhook.
// The JSON body is trigger.data.body; a top-level "assetIds" list starts one run per asset.
type FlowHandler struct {
	Key    []byte
	Runner Triggerer
	Jobs   engine.Enqueuer
	Log    *slog.Logger
}

func (h *FlowHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flowID := r.PathValue("id")
	token := r.Header.Get(Header)
	userID, _, _ := strings.Cut(token, ".")
	if userID == "" || subtle.ConstantTimeCompare([]byte(token), []byte(FlowToken(h.Key, userID, flowID))) != 1 {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var body any
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
	}
	var ids []string
	if m, ok := body.(map[string]any); ok {
		if list, ok := m["assetIds"].([]any); ok {
			for _, v := range list {
				if s, ok := v.(string); ok && s != "" {
					ids = append(ids, s)
				}
			}
		}
	}
	if len(ids) > maxAssets {
		http.Error(w, "too many assetIds", http.StatusBadRequest)
		return
	}
	ev := events.Event{
		Type: events.Webhook, Time: time.Now().UTC(), UserID: userID, AssetIDs: ids,
		Data: map[string]any{"body": body},
	}
	n, err := h.Runner.Trigger(r.Context(), h.Jobs, flowID, ev)
	switch {
	case errors.Is(err, engine.ErrUnknownFlow):
		http.Error(w, "no enabled flow with trigger webhook and this id", http.StatusNotFound)
	case err != nil:
		h.Log.Error("webhook trigger", "flow", flowID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]int{"queued": n})
	}
}
