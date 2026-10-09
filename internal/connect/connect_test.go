package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/store"
)

type fakeImmich struct {
	mu      sync.Mutex
	n       int
	keys    map[string]string // id -> secret
	perms   []string
	deleted []string
}

func (f *fakeImmich) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/api-keys":
		if r.Header.Get("Cookie") != "immich_access_token=session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct{ Permissions []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.perms = body.Permissions
		f.n++
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.n)
		secret := fmt.Sprintf("secret-%d", f.n)
		f.keys[id] = secret
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"secret":%q,"apiKey":{"id":%q,"name":"immich-flows","permissions":[]},"id":%q,"name":"x","permissions":[]}`, secret, id, id)
	case r.Method == http.MethodDelete && len(r.URL.Path) > len("/api/api-keys/"):
		id := r.URL.Path[len("/api/api-keys/"):]
		f.deleted = append(f.deleted, id)
		delete(f.keys, id)
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/api/users/me":
		for _, s := range f.keys {
			if r.Header.Get("x-api-key") == s {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"0b6f2a1e-4c3a-4d8e-9a4b-1f2e3d4c5b6a","email":"a@b.c","name":"A","isAdmin":false}`))
				return
			}
		}
		w.WriteHeader(http.StatusUnauthorized)
	default:
		http.NotFound(w, r)
	}
}

func setup(t *testing.T) (*Manager, *fakeImmich) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &fakeImmich{keys: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	key, err := LoadKey("", dir)
	if err != nil {
		t.Fatal(err)
	}
	return New(db, key, u, nil), f
}

func caller() immich.Auth {
	return func(r *http.Request) { r.Header.Set("Cookie", "immich_access_token=session") }
}

func TestConnectClientDisconnect(t *testing.T) {
	ctx := context.Background()
	m, f := setup(t)

	if _, err := m.Client(ctx, "u1"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("want ErrNotConnected, got %v", err)
	}
	if err := m.Connect(ctx, "u1", caller()); err != nil {
		t.Fatal(err)
	}
	if len(f.perms) != len(Permissions) {
		t.Errorf("permissions sent: %v", f.perms)
	}
	if s, _ := m.Status(ctx, "u1"); !s.Connected || s.CreatedAt == nil {
		t.Errorf("status %+v", s)
	}

	c, err := m.Client(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.GetMyUserWithResponse(ctx)
	if err != nil || res.StatusCode() != http.StatusOK {
		t.Fatalf("client not authenticated with stored key: %v %d", err, res.StatusCode())
	}

	// Reconnect replaces and deletes the old key.
	if err := m.Connect(ctx, "u1", caller()); err != nil {
		t.Fatal(err)
	}
	if len(f.deleted) != 1 || len(f.keys) != 1 {
		t.Errorf("deleted=%v keys=%v", f.deleted, f.keys)
	}

	if err := m.Disconnect(ctx, "u1", caller()); err != nil {
		t.Fatal(err)
	}
	if s, _ := m.Status(ctx, "u1"); s.Connected || len(f.keys) != 0 {
		t.Errorf("status %+v keys %v", s, f.keys)
	}
}

func TestConnectRejectedByImmich(t *testing.T) {
	m, _ := setup(t)
	err := m.Connect(context.Background(), "u1", func(*http.Request) {})
	var ie *immich.Error
	if !errors.As(err, &ie) || ie.Status != http.StatusUnauthorized {
		t.Errorf("got %v", err)
	}
}

func TestStoredKeyIsEncrypted(t *testing.T) {
	ctx := context.Background()
	m, _ := setup(t)
	if err := m.Connect(ctx, "u1", caller()); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	_ = m.db.QueryRow(`SELECT api_key FROM connections`).Scan(&raw)
	if string(raw) == "secret-1" || len(raw) < 20 {
		t.Errorf("stored key not encrypted: %q", raw)
	}
	m.key, _ = LoadKey("other secret", "")
	if _, err := m.Client(ctx, "u1"); err == nil {
		t.Error("decrypt with wrong key must fail")
	}
}

func TestLoadKeyPersistsGeneratedKey(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadKey("", dir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := LoadKey("", dir)
	if string(a) != string(b) || len(a) != 32 {
		t.Error("generated key not stable")
	}
	st, _ := os.Stat(filepath.Join(dir, "secret.key"))
	if st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
	c, _ := LoadKey("explicit", dir)
	if string(c) == string(a) {
		t.Error("explicit secret ignored")
	}
}
