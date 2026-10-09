package immich

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Auth sets credentials on an outgoing request.
type Auth func(*http.Request)

// APIKey authenticates with an Immich API key.
func APIKey(key string) Auth {
	return func(r *http.Request) { r.Header.Set("x-api-key", key) }
}

// Forward copies the credentials of an incoming client request.
func Forward(in *http.Request) Auth {
	h := in.Header.Clone()
	return func(r *http.Request) {
		for _, k := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Immich-User-Token", "X-Immich-Session-Token"} {
			if v := h.Values(k); len(v) > 0 {
				r.Header[k] = v
			}
		}
	}
}

// New returns a client for the Immich server at base (without /api).
func New(base *url.URL, auth Auth, hc *http.Client) (*ClientWithResponses, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	server := strings.TrimSuffix(base.String(), "/") + "/api"
	return NewClientWithResponses(server,
		WithHTTPClient(hc),
		WithRequestEditorFn(func(_ context.Context, r *http.Request) error {
			auth(r)
			return nil
		}),
	)
}

// Error is a non-2xx response from Immich.
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("immich: HTTP %d: %s", e.Status, e.Body)
}

// Check returns an *Error if res is not a 2xx response.
func Check(res interface {
	StatusCode() int
}, body []byte) error {
	if c := res.StatusCode(); c < 200 || c > 299 {
		if len(body) > 512 {
			body = body[:512]
		}
		return &Error{Status: c, Body: string(body)}
	}
	return nil
}
