package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/serraniel/immich-flows/internal/auth"
)

func TestMe(t *testing.T) {
	immich := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"0b6f2a1e-4c3a-4d8e-9a4b-1f2e3d4c5b6a","email":"a@b.c","name":"A","isAdmin":true}`))
	}))
	defer immich.Close()
	u, _ := url.Parse(immich.URL)
	h := New("/_flows/api", u, auth.New(u, nil), nil, nil, slog.New(slog.DiscardHandler))

	req := httptest.NewRequest(http.MethodGet, "/_flows/api/me", nil)
	req.Header.Set("x-api-key", "k")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var got auth.User
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || !got.IsAdmin || got.Name != "A" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_flows/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/_flows/api/nope", nil)
	req.Header.Set("x-api-key", "k")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown route: %d", rec.Code)
	}
}
