//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"
)

type tagDTO struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

func (c *client) tagAsset(assetID string, values ...string) {
	c.t.Helper()
	var tags []tagDTO
	if s := c.json(http.MethodPut, "/api/tags", map[string]any{"tags": values}, &tags); s != http.StatusOK {
		c.t.Fatalf("upsert tags: %d", s)
	}
	ids := make([]string, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}
	if s := c.json(http.MethodPut, "/api/tags/assets", map[string]any{"tagIds": ids, "assetIds": []string{assetID}}, nil); s != http.StatusOK {
		c.t.Fatalf("tag asset: %d", s)
	}
}

func (c *client) assetTags(assetID string) []string {
	c.t.Helper()
	var a struct{ Tags []tagDTO }
	c.json(http.MethodGet, "/api/assets/"+assetID, nil, &a)
	var out []string
	for _, t := range a.Tags {
		out = append(out, t.Value)
	}
	sort.Strings(out)
	return out
}

func TestTagMove(t *testing.T) {
	c := adminClient(t)
	p := fmt.Sprintf("move%d", time.Now().UnixNano())
	a1 := c.upload("a1.jpg", randomJPEG(t)).ID
	a2 := c.upload("a2.jpg", randomJPEG(t)).ID
	c.tagAsset(a1, p+"/XX")
	c.tagAsset(a2, p+"/XX/child", p+"/keep")

	var plan struct {
		Assets  int `json:"assets"`
		Changes []struct{ From, To string }
	}
	req := map[string]any{"from": p + "/XX", "to": p + "/YY/XX", "dryRun": true}
	if s := c.json(http.MethodPost, "/_flows/api/tags/move", req, &plan); s != http.StatusOK || plan.Assets != 2 || len(plan.Changes) != 2 {
		t.Fatalf("plan: %d %+v", s, plan)
	}
	if got := c.assetTags(a1); len(got) != 1 || got[0] != p+"/XX" {
		t.Fatalf("dry run changed tags: %v", got)
	}

	req["dryRun"] = false
	if s := c.json(http.MethodPost, "/_flows/api/tags/move", req, &plan); s != http.StatusOK {
		t.Fatalf("apply: %d", s)
	}
	if got := c.assetTags(a1); fmt.Sprint(got) != fmt.Sprint([]string{p + "/YY/XX"}) {
		t.Errorf("a1: %v", got)
	}
	if got := c.assetTags(a2); fmt.Sprint(got) != fmt.Sprint([]string{p + "/YY/XX/child", p + "/keep"}) {
		t.Errorf("a2: %v", got)
	}
	var all []tagDTO
	c.json(http.MethodGet, "/api/tags", nil, &all)
	for _, tag := range all {
		if tag.Value == p+"/XX" || tag.Value == p+"/XX/child" {
			t.Errorf("source tag still exists: %s", tag.Value)
		}
	}

	if s := c.json(http.MethodPost, "/_flows/api/tags/move", map[string]any{"from": p + "/YY", "to": p + "/YY/Z"}, nil); s != http.StatusBadRequest {
		t.Errorf("move into itself: %d", s)
	}
}
