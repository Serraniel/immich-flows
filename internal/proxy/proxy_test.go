package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProxyForwards(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Path", r.URL.Path)
		w.Header().Set("X-Host", r.Host)
		_, _ = w.Write(body)
	}))
	defer up.Close()
	u, _ := url.Parse(up.URL)

	srv := httptest.NewServer(New(u, slog.New(slog.DiscardHandler)))
	defer srv.Close()

	res, err := http.Post(srv.URL+"/api/assets", "text/plain", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)

	if string(body) != "payload" || res.Header.Get("X-Path") != "/api/assets" {
		t.Errorf("got body=%q path=%q", body, res.Header.Get("X-Path"))
	}
	if res.Header.Get("X-Host") != strings.TrimPrefix(srv.URL, "http://") {
		t.Errorf("host not preserved: %q", res.Header.Get("X-Host"))
	}
}

func TestHealthz(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	New(u, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, Prefix+"/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status %d", rec.Code)
	}
}

func TestUpstreamDown(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:1")
	rec := httptest.NewRecorder()
	New(u, slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/server/ping", nil))
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status %d", rec.Code)
	}
}
