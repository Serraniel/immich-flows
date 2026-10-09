package hooks

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/serraniel/immich-flows/internal/auth"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/proxy"
)

type fakeUsers struct{ err error }

func (f fakeUsers) Authenticate(*http.Request) (*auth.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &auth.User{ID: "u1"}, nil
}

type recorder struct {
	mu     sync.Mutex
	events []events.Event
	next   int
	got    chan struct{}
}

func newRecorder() *recorder { return &recorder{got: make(chan struct{}, 10)} }

func (r *recorder) publish(_ context.Context, ev events.Event) error {
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
	r.got <- struct{}{}
	return nil
}

func (r *recorder) wait(t *testing.T) events.Event {
	t.Helper()
	select {
	case <-r.got:
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	return r.events[r.next-1]
}

func (r *recorder) none(t *testing.T) {
	t.Helper()
	select {
	case <-r.got:
		t.Fatalf("unexpected event: %+v", r.events)
	case <-time.After(200 * time.Millisecond):
	}
}

// upstream reads the whole body like Immich and replies with status and body.
func upstream(t *testing.T, status int, body string, received *[]byte) *url.URL {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if received != nil {
			*received = b
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u
}

func form(t *testing.T, file []byte, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("fileCreatedAt", "2026-01-01T00:00:00Z")
	p, _ := w.CreateFormFile("assetData", "client.jpg")
	_, _ = p.Write(file)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func post(t *testing.T, up *url.URL, rec *recorder, users Users, body io.Reader, ct string) int {
	t.Helper()
	d := Deps{Users: users, Publish: rec.publish, Log: slog.New(slog.DiscardHandler)}
	srv := httptest.NewServer(proxy.New(up, slog.New(slog.DiscardHandler), Upload(d)))
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/assets", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	return res.StatusCode
}

func TestUploadHashesOriginal(t *testing.T) {
	file := make([]byte, 5<<20)
	_, _ = rand.Read(file)
	var received []byte
	up := upstream(t, http.StatusCreated, `{"id":"a1","status":"created"}`, &received)
	body, ct := form(t, file, map[string]string{"filename": "SERVICE-A_1_x.jpg"})
	sent := body.Bytes()
	rec := newRecorder()

	if s := post(t, up, rec, fakeUsers{}, bytes.NewReader(sent), ct); s != http.StatusCreated {
		t.Fatalf("status %d", s)
	}
	if !bytes.Equal(received, sent) {
		t.Fatal("upstream did not receive the original body")
	}
	ev := rec.wait(t)
	o := ev.Data["original"].(Original)
	md, sh := md5.Sum(file), sha256.Sum256(file)
	if ev.Type != events.AssetUploaded || ev.UserID != "u1" || ev.AssetIDs[0] != "a1" || ev.Data["status"] != "created" {
		t.Errorf("event %+v", ev)
	}
	if o.MD5 != hex.EncodeToString(md[:]) || o.SHA256 != hex.EncodeToString(sh[:]) || o.Size != int64(len(file)) ||
		o.Filename != "SERVICE-A_1_x.jpg" || len(o.SHA1) != 40 || len(o.SHA512) != 128 {
		t.Errorf("original %+v", o)
	}
}

func TestUploadPartFilenameFallback(t *testing.T) {
	up := upstream(t, http.StatusOK, `{"id":"a1","status":"duplicate"}`, nil)
	body, ct := form(t, []byte("x"), nil)
	rec := newRecorder()
	post(t, up, rec, fakeUsers{}, body, ct)
	ev := rec.wait(t)
	if ev.Data["original"].(Original).Filename != "client.jpg" || ev.Data["status"] != "duplicate" {
		t.Errorf("%+v", ev)
	}
}

func TestUploadNoEvent(t *testing.T) {
	cases := map[string]struct {
		status int
		resp   string
		body   func() (io.Reader, string)
		users  Users
	}{
		"upstream error": {http.StatusBadRequest, `{"message":"bad"}`, func() (io.Reader, string) { return form(t, []byte("x"), nil) }, fakeUsers{}},
		"not multipart":  {http.StatusCreated, `{"id":"a1"}`, func() (io.Reader, string) { return strings.NewReader("{}"), "application/json" }, fakeUsers{}},
		"malformed": {http.StatusCreated, `{"id":"a1"}`, func() (io.Reader, string) {
			return strings.NewReader("garbage"), "multipart/form-data; boundary=xyz"
		}, fakeUsers{}},
		"no asset part": {http.StatusCreated, `{"id":"a1"}`, func() (io.Reader, string) {
			var b bytes.Buffer
			w := multipart.NewWriter(&b)
			_ = w.WriteField("x", "y")
			_ = w.Close()
			return &b, w.FormDataContentType()
		}, fakeUsers{}},
		"unknown user": {http.StatusCreated, `{"id":"a1"}`, func() (io.Reader, string) { return form(t, []byte("x"), nil) }, fakeUsers{err: errors.New("401")}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := newRecorder()
			body, ct := c.body()
			if s := post(t, upstream(t, c.status, c.resp, nil), rec, c.users, body, ct); s != c.status {
				t.Errorf("proxy changed status: %d", s)
			}
			rec.none(t)
		})
	}
}
