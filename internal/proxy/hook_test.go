package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newUpstream(t *testing.T, h http.HandlerFunc) *url.URL {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u
}

func serve(t *testing.T, up *url.URL, hooks ...Hook) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(New(up, slog.New(slog.DiscardHandler), hooks...))
	t.Cleanup(srv.Close)
	return srv
}

type teeHash struct {
	io.ReadCloser
	h hash.Hash
}

func (t teeHash) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	t.h.Write(p[:n])
	return n, err
}

func TestHookPreAndPost(t *testing.T) {
	up := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") == "br" {
			t.Error("Accept-Encoding not stripped on hooked route")
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"1","status":"created"}`))
	})

	payload := bytes.Repeat([]byte("x"), 3<<20)
	want := sha256.Sum256(payload)
	done := make(chan *Exchange, 1)

	srv := serve(t, up, Hook{
		Route: "POST /api/assets",
		Pre: func(_ context.Context, ex *Exchange) error {
			h := sha256.New()
			ex.Request.Body = teeHash{ex.Request.Body, h}
			ex.Set("hash", h)
			return nil
		},
		Post: func(ex *Exchange) { done <- ex },
	})

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/assets", bytes.NewReader(payload))
	req.Header.Set("Accept-Encoding", "br")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()

	select {
	case ex := <-done:
		if ex.Status != http.StatusCreated || !ex.Complete || string(ex.Response) != `{"id":"1","status":"created"}` {
			t.Errorf("exchange: status=%d complete=%v body=%q", ex.Status, ex.Complete, ex.Response)
		}
		if got := ex.Get("hash").(hash.Hash).Sum(nil); !bytes.Equal(got, want[:]) {
			t.Error("hash of streamed body mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("post hook not called")
	}
}

func TestHookMethodMismatchIsNotHooked(t *testing.T) {
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	called := false
	srv := serve(t, up, Hook{Route: "POST /api/assets", Post: func(*Exchange) { called = true }})

	res, err := http.Get(srv.URL + "/api/assets")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if called || res.StatusCode != http.StatusOK {
		t.Errorf("called=%v status=%d", called, res.StatusCode)
	}
}

func TestHookPreFailsOpen(t *testing.T) {
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	srv := serve(t, up,
		Hook{Route: "DELETE /api/assets", Pre: func(context.Context, *Exchange) error { return errors.New("boom") }},
		Hook{Route: "DELETE /api/assets", Timeout: 10 * time.Millisecond, Pre: func(ctx context.Context, _ *Exchange) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	)
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/assets", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("status %d", res.StatusCode)
	}
}

func TestHookPathValueAndNonJSON(t *testing.T) {
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("plain"))
	})
	done := make(chan *Exchange, 1)
	srv := serve(t, up, Hook{Route: "PUT /api/tags/{id}/assets", Post: func(ex *Exchange) { done <- ex }})

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/tags/abc/assets", strings.NewReader("{}"))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()

	ex := <-done
	if ex.Request.PathValue("id") != "abc" || ex.Response != nil || string(body) != "plain" {
		t.Errorf("id=%q response=%q body=%q", ex.Request.PathValue("id"), ex.Response, body)
	}
}

// immich-upload-optimizer answers uploads with JSON labelled text/plain.
func TestHookJSONWithWrongContentType(t *testing.T) {
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"a1","status":"created"}`))
	})
	done := make(chan *Exchange, 1)
	srv := serve(t, up, Hook{Route: "POST /api/assets", Post: func(ex *Exchange) { done <- ex }})
	res, err := http.Post(srv.URL+"/api/assets", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(res.Body)
	_ = res.Body.Close()
	if ex := <-done; string(ex.Response) != `{"id":"a1","status":"created"}` {
		t.Errorf("response %q", ex.Response)
	}
}

func TestHookPostOnUpstreamError(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:1")
	done := make(chan *Exchange, 1)
	h := New(u, slog.New(slog.DiscardHandler), Hook{Route: "POST /api/assets", Post: func(ex *Exchange) { done <- ex }})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/assets", nil))
	if ex := <-done; ex.Status != http.StatusBadGateway || ex.Complete {
		t.Errorf("status=%d complete=%v", ex.Status, ex.Complete)
	}
}

func TestCaptureOverflow(t *testing.T) {
	big := `"` + strings.Repeat("a", maxCapture) + `"`
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(big))
	})
	done := make(chan *Exchange, 1)
	srv := serve(t, up, Hook{Route: "GET /api/big", Post: func(ex *Exchange) { done <- ex }})

	res, err := http.Get(srv.URL + "/api/big")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if ex := <-done; ex.Response != nil || len(body) != len(big) {
		t.Errorf("captured %d bytes, client got %d", len(ex.Response), len(body))
	}
}
