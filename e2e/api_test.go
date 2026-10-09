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

func TestConnection(t *testing.T) {
	c := adminClient(t)
	var st struct {
		Connected bool `json:"connected"`
	}
	if s := c.json(http.MethodPost, "/_flows/api/connection", nil, &st); s != http.StatusOK || !st.Connected {
		t.Fatalf("connect: %d %+v", s, st)
	}

	var keys []struct {
		Name string `json:"name"`
	}
	c.json(http.MethodGet, "/api/api-keys", nil, &keys)
	found := false
	for _, k := range keys {
		found = found || k.Name == "immich-flows"
	}
	if !found {
		t.Errorf("immich-flows key not created in Immich: %+v", keys)
	}

	if s := c.json(http.MethodDelete, "/_flows/api/connection", nil, nil); s != http.StatusNoContent {
		t.Fatalf("disconnect: %d", s)
	}
	c.json(http.MethodGet, "/_flows/api/connection", nil, &st)
	if st.Connected {
		t.Error("still connected")
	}
}
