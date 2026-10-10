package webhook

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
)

type triggerer struct {
	flow string
	evs  []events.Event
}

func (t *triggerer) Trigger(_ context.Context, _ engine.Enqueuer, flowID string, ev events.Event) (int, error) {
	if flowID != t.flow {
		return 0, engine.ErrUnknownFlow
	}
	t.evs = append(t.evs, ev)
	return max(1, len(ev.AssetIDs)), nil
}

func TestFlowHandler(t *testing.T) {
	tr := &triggerer{flow: "f1"}
	h := &FlowHandler{Key: []byte("key"), Runner: tr, Log: slog.New(slog.DiscardHandler)}
	mux := http.NewServeMux()
	mux.Handle("POST /hooks/{id}", h)
	do := func(flow, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/hooks/"+flow, strings.NewReader(body))
		req.Header.Set(Header, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	tok := FlowToken(h.Key, "u1", "f1")
	if !strings.HasPrefix(tok, "u1.") {
		t.Fatalf("token %q", tok)
	}

	for name, c := range map[string]struct{ flow, token string }{
		"no token":       {"f1", ""},
		"other flow":     {"f1", FlowToken(h.Key, "u1", "f2")},
		"user swapped":   {"f1", "u2" + strings.TrimPrefix(tok, "u1")},
		"wrong key":      {"f1", FlowToken([]byte("other"), "u1", "f1")},
		"token for flow": {"f2", tok},
	} {
		if rec := do(c.flow, c.token, `{}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}

	if rec := do("f1", tok, `{"assetIds":["a","b"],"x":1}`); rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"queued":2`) {
		t.Fatalf("trigger: %d %s", rec.Code, rec.Body)
	}
	ev := tr.evs[0]
	if ev.Type != events.Webhook || ev.UserID != "u1" || len(ev.AssetIDs) != 2 || ev.Data["body"].(map[string]any)["x"] != float64(1) {
		t.Errorf("event %+v", ev)
	}
	if rec := do("f1", tok, ``); rec.Code != http.StatusAccepted || tr.evs[1].Data["body"] != nil {
		t.Errorf("empty body: %d", rec.Code)
	}
	if rec := do("f1", tok, `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: %d", rec.Code)
	}
	f2 := FlowToken(h.Key, "u1", "f2")
	if rec := do("f2", f2, `{}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown flow: %d", rec.Code)
	}
}
