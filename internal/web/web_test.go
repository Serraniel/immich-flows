package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	files := fstest.MapFS{
		"200.html":                 {Data: []byte("spa")},
		"_app/immutable/app.js":    {Data: []byte("js")},
		"robots.txt":               {Data: []byte("robots")},
		"_app/immutable/dir/x.css": {Data: []byte("css")},
	}
	h := handler("/_flows", files)

	cases := []struct {
		path, body, cache string
	}{
		{"/_flows/", "spa", "no-cache"},
		{"/_flows", "spa", "no-cache"},
		{"/_flows/flows/abc", "spa", "no-cache"},
		{"/_flows/robots.txt", "robots", "no-cache"},
		{"/_flows/_app/immutable/app.js", "js", "public, max-age=31536000, immutable"},
		{"/_flows/_app/immutable/dir", "spa", "no-cache"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != c.body || rec.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s: %d %q cache=%q", c.path, rec.Code, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
	}
}

func TestHandlerRejectsTraversal(t *testing.T) {
	rec := httptest.NewRecorder()
	handler("/_flows", fstest.MapFS{"200.html": {Data: []byte("spa")}}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_flows/../etc/passwd", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d", rec.Code)
	}
}

func TestHandlerNotBuilt(t *testing.T) {
	rec := httptest.NewRecorder()
	handler("/_flows", fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_flows/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d", rec.Code)
	}
}
