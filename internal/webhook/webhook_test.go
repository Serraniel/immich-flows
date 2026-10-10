package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/queue"
	"github.com/serraniel/immich-flows/internal/store"
)

type jobs struct{ payloads []upload }

func (j *jobs) Enqueue(_ context.Context, kind string, p any) (int64, error) {
	if kind != Job {
		panic(kind)
	}
	j.payloads = append(j.payloads, p.(upload))
	return int64(len(j.payloads)), nil
}

const body = `{"type":"AssetV1","trigger":"%s","data":{"asset":{"id":"a1","ownerId":"u1","originalFileName":"x.jpg"}}}`

func post(h http.Handler, token, trigger string) int {
	req := httptest.NewRequest(http.MethodPost, "/_flows/api/webhook", strings.NewReader(strings.Replace(body, "%s", trigger, 1)))
	req.Header.Set(Header, token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestHandler(t *testing.T) {
	j := &jobs{}
	wants := true
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := &Handler{Key: []byte("key"), Jobs: j, Wants: func(string) bool { return wants }, Log: slog.New(slog.DiscardHandler), Now: func() time.Time { return now }}

	token := Token(h.Key, "u1")
	if c := post(h, "wrong", "AssetCreate"); c != http.StatusUnauthorized {
		t.Errorf("wrong token: %d", c)
	}
	if c := post(h, Token(h.Key, "u2"), "AssetCreate"); c != http.StatusUnauthorized {
		t.Errorf("other user's token: %d", c)
	}
	if c := post(h, token, "AssetTagged"); c != http.StatusAccepted || len(j.payloads) != 0 {
		t.Errorf("tagged: %d %v", c, j.payloads)
	}
	if c := post(h, token, "AssetCreate"); c != http.StatusAccepted || len(j.payloads) != 1 {
		t.Fatalf("create: %d %v", c, j.payloads)
	}
	if got := j.payloads[0]; got != (upload{UserID: "u1", AssetID: "a1", Trigger: "AssetCreate", ReceivedAt: now}) {
		t.Errorf("payload %+v", got)
	}
	wants = false
	if c := post(h, token, "AssetCreate"); c != http.StatusAccepted || len(j.payloads) != 1 {
		t.Errorf("no flows: %d", c)
	}
	if Token([]byte("key"), "u1") == Token([]byte("other"), "u1") || len(token) != 64 {
		t.Error("token derivation")
	}
}

func TestProcess(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	received := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := received
	seen := &Seen{DB: db, Now: func() time.Time { return now }}
	var published []events.Event
	handle := process(seen, func(_ context.Context, ev events.Event) error {
		published = append(published, ev)
		return nil
	})
	job := func(id string) queue.Job {
		b, _ := json.Marshal(upload{UserID: "u1", AssetID: id, Trigger: "AssetCreate", ReceivedAt: received})
		return queue.Job{Kind: Job, Payload: b, Attempt: 1}
	}

	if err := handle(ctx, job("a1")); err == nil || !strings.Contains(err.Error(), "snoozed") {
		t.Fatalf("expected snooze, got %v", err)
	}

	// The proxy reported a2 while the webhook job waited.
	if err := seen.Mark(ctx, "a2"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(Delay)
	for _, id := range []string{"a1", "a2", "a1"} {
		if err := handle(ctx, job(id)); err != nil {
			t.Fatal(err)
		}
	}
	if len(published) != 1 || published[0].AssetIDs[0] != "a1" || published[0].Type != events.AssetUploaded ||
		published[0].UserID != "u1" || published[0].Data["source"] != "webhook" {
		t.Errorf("published %+v", published)
	}
}
