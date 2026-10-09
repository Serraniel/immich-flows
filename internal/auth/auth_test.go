package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

const userJSON = `{"id":"0b6f2a1e-4c3a-4d8e-9a4b-1f2e3d4c5b6a","email":"a@b.c","name":"A","isAdmin":%s}`

func fakeImmich(t *testing.T, calls *atomic.Int32) *url.URL {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/users/me" {
			http.NotFound(w, r)
			return
		}
		admin := "false"
		switch {
		case r.Header.Get("x-api-key") == "admin-key":
			admin = "true"
		case r.Header.Get("x-api-key") == "user-key":
		case func() bool { c, err := r.Cookie("immich_access_token"); return err == nil && c.Value == "tok" }():
		case r.Header.Get("x-api-key") == "broken":
			w.WriteHeader(http.StatusInternalServerError)
			return
		default:
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, userJSON, admin)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u
}

func request(h http.Handler, set func(*http.Request)) int {
	r := httptest.NewRequest(http.MethodGet, "/_flows/api/x", nil)
	set(r)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

func TestMiddleware(t *testing.T) {
	var calls atomic.Int32
	a := New(fakeImmich(t, &calls), nil)
	ok := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()) == nil {
			t.Error("user missing in context")
		}
	})
	h := a.Middleware(ok)
	admin := a.Middleware(RequireAdmin(ok))

	cases := []struct {
		name string
		h    http.Handler
		set  func(*http.Request)
		want int
	}{
		{"none", h, func(*http.Request) {}, http.StatusUnauthorized},
		{"bad key", h, func(r *http.Request) { r.Header.Set("x-api-key", "nope") }, http.StatusUnauthorized},
		{"cookie", h, func(r *http.Request) { r.Header.Set("Cookie", "immich_access_token=tok") }, http.StatusOK},
		{"other cookie only", h, func(r *http.Request) { r.Header.Set("Cookie", "x=y") }, http.StatusUnauthorized},
		{"user on admin route", admin, func(r *http.Request) { r.Header.Set("x-api-key", "user-key") }, http.StatusForbidden},
		{"admin on admin route", admin, func(r *http.Request) { r.Header.Set("x-api-key", "admin-key") }, http.StatusOK},
		{"immich error", h, func(r *http.Request) { r.Header.Set("x-api-key", "broken") }, http.StatusBadGateway},
	}
	for _, c := range cases {
		if got := request(c.h, c.set); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestCache(t *testing.T) {
	var calls atomic.Int32
	a := New(fakeImmich(t, &calls), nil)
	now := time.Now()
	a.now = func() time.Time { return now }
	h := a.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	set := func(r *http.Request) { r.Header.Set("x-api-key", "user-key") }

	request(h, set)
	request(h, set)
	if calls.Load() != 1 {
		t.Errorf("calls %d, want 1 (cached)", calls.Load())
	}
	now = now.Add(ttl + time.Second)
	request(h, set)
	if calls.Load() != 2 {
		t.Errorf("calls %d, want 2 after expiry", calls.Load())
	}
	// Failures are not cached.
	bad := func(r *http.Request) { r.Header.Set("x-api-key", "nope") }
	request(h, bad)
	request(h, bad)
	if calls.Load() != 4 {
		t.Errorf("calls %d, want 4", calls.Load())
	}
}
