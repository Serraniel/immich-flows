package trashwatch

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/serraniel/immich-flows/internal/connect"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/store"
)

const (
	a1 = "00000000-0000-4000-8000-000000000001"
	a2 = "00000000-0000-4000-8000-000000000002"
	a3 = "00000000-0000-4000-8000-000000000003"
)

// fakeImmich keeps assets by ID with a trashed flag; missing assets answer 400 like Immich.
type fakeImmich struct {
	mu     sync.Mutex
	assets map[string]bool // id -> trashed
}

func (f *fakeImmich) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/search/metadata":
		items := []map[string]any{}
		for id, trashed := range f.assets {
			if trashed {
				items = append(items, map[string]any{"id": id, "originalFileName": id[len(id)-1:] + ".jpg", "isTrashed": true})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"assets": map[string]any{"items": items, "nextPage": nil}})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/assets/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/assets/")
		trashed, ok := f.assets[id]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Not found or no asset.read access"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "isTrashed": trashed})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeImmich) set(id string, trashed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.assets[id] = trashed
}

func (f *fakeImmich) remove(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.assets, id)
}

func setup(t *testing.T, wants bool) (*Watcher, *fakeImmich, *[]events.Event) {
	t.Helper()
	fake := &fakeImmich{assets: map[string]bool{}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var published []events.Event
	w := New(Deps{
		DB: db,
		Users: func(context.Context) ([]string, error) {
			return []string{"u1", "gone-user"}, nil
		},
		Clients: func(_ context.Context, user string) (*immich.ClientWithResponses, error) {
			if user == "gone-user" {
				return nil, connect.ErrNotConnected
			}
			return immich.New(u, immich.APIKey("k"), nil)
		},
		Publish: func(_ context.Context, ev events.Event) error {
			published = append(published, ev)
			return nil
		},
		Wants: func(string) bool { return wants },
		Log:   slog.New(slog.DiscardHandler),
	})
	return w, fake, &published
}

func TestSync(t *testing.T) {
	ctx := context.Background()
	w, fake, published := setup(t, true)
	fake.set(a1, true)
	fake.set(a2, true)
	fake.set(a3, false)
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 0 {
		t.Fatalf("first pass published %v", *published)
	}

	fake.remove(a1)     // purged by Immich
	fake.set(a2, false) // restored outside the proxy
	fake.set(a3, true)  // newly trashed
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 1 {
		t.Fatalf("published %d events", len(*published))
	}
	ev := (*published)[0]
	if ev.Type != events.AssetDeleted || ev.UserID != "u1" || len(ev.AssetIDs) != 1 || ev.AssetIDs[0] != a1 {
		t.Errorf("event %+v", ev)
	}
	snap := ev.Data["assets"].(map[string]any)[a1].(map[string]any)
	if snap["originalFileName"] != "1.jpg" || ev.Data["source"] != "reconcile" {
		t.Errorf("snapshot %+v", ev.Data)
	}

	// Reported by the proxy: no second event.
	if err := w.Forget(ctx, "u1", []string{a3}); err != nil {
		t.Fatal(err)
	}
	fake.remove(a3)
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 1 {
		t.Errorf("events after forget: %+v", (*published)[1:])
	}
}

func TestSyncWithoutFlowsClears(t *testing.T) {
	ctx := context.Background()
	w, fake, published := setup(t, true)
	fake.set(a1, true)
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	w.d.Wants = func(string) bool { return false }
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	fake.remove(a1)
	w.d.Wants = func(string) bool { return true }
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 0 {
		t.Errorf("stale delete emitted: %+v", *published)
	}
}

func TestPublishFailureRetries(t *testing.T) {
	ctx := context.Background()
	w, fake, published := setup(t, true)
	fake.set(a1, true)
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	fake.remove(a1)
	ok := w.d.Publish
	w.d.Publish = func(context.Context, events.Event) error { return errors.New("db busy") }
	if err := w.Sync(ctx); err == nil {
		t.Fatal("expected error")
	}
	w.d.Publish = ok
	if err := w.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if len(*published) != 1 || (*published)[0].AssetIDs[0] != a1 {
		t.Errorf("not retried: %+v", *published)
	}
}
