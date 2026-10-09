//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestAPIAuth(t *testing.T) {
	anon := &client{t: t}
	if s := anon.json(http.MethodGet, "/_flows/api/me", nil, nil); s != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", s)
	}

	c := adminClient(t)
	var me struct {
		Email   string `json:"email"`
		IsAdmin bool   `json:"isAdmin"`
	}
	if s := c.json(http.MethodGet, "/_flows/api/me", nil, &me); s != http.StatusOK || !me.IsAdmin || me.Email != "admin@e2e.test" {
		t.Errorf("me: %d %+v", s, me)
	}
}
