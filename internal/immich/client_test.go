package immich

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNewAuthAndPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users/me" || r.Header.Get("x-api-key") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"0b6f2a1e-4c3a-4d8e-9a4b-1f2e3d4c5b6a","email":"a@b.c","name":"A","isAdmin":true}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL + "/")

	c, err := New(u, APIKey("secret"), nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.GetMyUserWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(res, res.Body); err != nil {
		t.Fatal(err)
	}
	if res.JSON200 == nil || !res.JSON200.IsAdmin || res.JSON200.Name != "A" {
		t.Errorf("unexpected user: %+v", res.JSON200)
	}
}

func TestCheckError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"no"}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	c, _ := New(u, APIKey("x"), nil)
	res, err := c.GetMyUserWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var e *Error
	if err := Check(res, res.Body); !errors.As(err, &e) || e.Status != http.StatusForbidden {
		t.Errorf("got %v", err)
	}
}

func TestForward(t *testing.T) {
	in := httptest.NewRequest(http.MethodGet, "/", nil)
	in.Header.Set("Cookie", "immich_access_token=t")
	in.Header.Set("X-Other", "drop")
	out := httptest.NewRequest(http.MethodGet, "/", nil)
	Forward(in)(out)
	if out.Header.Get("Cookie") != "immich_access_token=t" || out.Header.Get("X-Other") != "" {
		t.Errorf("headers: %v", out.Header)
	}
}
