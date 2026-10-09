// Package auth authenticates requests to immich-flows with the caller's Immich credentials.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/serraniel/immich-flows/internal/immich"
)

// User is the authenticated Immich user.
type User struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"isAdmin"`
}

// ErrUnauthenticated means Immich rejected or no credentials were sent.
var ErrUnauthenticated = errors.New("unauthenticated")

const (
	ttl        = time.Minute
	maxEntries = 10000
)

// Authenticator validates credentials against Immich and caches the result briefly.
type Authenticator struct {
	immich *url.URL
	hc     *http.Client
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]entry
}

type entry struct {
	user    *User
	expires time.Time
}

// New returns an Authenticator for the Immich server at immichURL.
func New(immichURL *url.URL, hc *http.Client) *Authenticator {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Authenticator{immich: immichURL, hc: hc, now: time.Now, cache: map[string]entry{}}
}

// credentialKey identifies the credentials of r, or "" if there are none.
func credentialKey(r *http.Request) string {
	h := sha256.New()
	found := false
	for _, k := range []string{"Authorization", "X-Api-Key", "X-Immich-User-Token", "X-Immich-Session-Token"} {
		if v := r.Header.Get(k); v != "" {
			found = true
			h.Write([]byte(k + "=" + v + "\n"))
		}
	}
	if c, err := r.Cookie("immich_access_token"); err == nil && c.Value != "" {
		found = true
		h.Write([]byte("cookie=" + c.Value + "\n"))
	}
	if !found {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Authenticate returns the Immich user for the credentials on r.
func (a *Authenticator) Authenticate(r *http.Request) (*User, error) {
	key := credentialKey(r)
	if key == "" {
		return nil, ErrUnauthenticated
	}

	a.mu.Lock()
	e, ok := a.cache[key]
	a.mu.Unlock()
	if ok && a.now().Before(e.expires) {
		return e.user, nil
	}

	c, err := immich.New(a.immich, immich.Forward(r), a.hc)
	if err != nil {
		return nil, err
	}
	res, err := c.GetMyUserWithResponse(r.Context())
	if err != nil {
		return nil, err
	}
	if res.StatusCode() == http.StatusUnauthorized || res.StatusCode() == http.StatusForbidden {
		return nil, ErrUnauthenticated
	}
	if err := immich.Check(res, res.Body); err != nil {
		return nil, err
	}
	u := &User{ID: res.JSON200.Id.String(), Email: string(res.JSON200.Email), Name: res.JSON200.Name, IsAdmin: res.JSON200.IsAdmin}

	a.mu.Lock()
	if len(a.cache) >= maxEntries {
		clear(a.cache)
	}
	a.cache[key] = entry{user: u, expires: a.now().Add(ttl)}
	a.mu.Unlock()
	return u, nil
}

type userKey struct{}

// FromContext returns the user set by Middleware.
func FromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userKey{}).(*User)
	return u
}

// WithUser returns ctx carrying u.
func WithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// Middleware rejects unauthenticated requests with 401 and stores the user in the context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := a.Authenticate(r)
		switch {
		case errors.Is(err, ErrUnauthenticated):
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		case err != nil:
			http.Error(w, "immich unavailable", http.StatusBadGateway)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

// RequireAdmin rejects non-admin users with 403. Use behind Middleware.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := FromContext(r.Context()); u == nil || !u.IsAdmin {
			http.Error(w, "admin only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
