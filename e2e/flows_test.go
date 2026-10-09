//go:build e2e

package e2e

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// connect links the admin to immich-flows so flows run for their uploads.
func (c *client) connect() {
	c.t.Helper()
	if s := c.json(http.MethodPost, "/_flows/api/connection", nil, nil); s != http.StatusOK {
		c.t.Fatalf("connect: %d", s)
	}
}

// waitTags polls until the asset has exactly want (sorted), or fails.
func (c *client) waitTags(assetID string, want []string) {
	c.t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var got []string
	for time.Now().Before(deadline) {
		got = c.assetTags(assetID)
		if fmt.Sprint(got) == fmt.Sprint(want) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	c.t.Fatalf("asset tags %v, want %v", got, want)
}

func TestFlowTagsUploadFromFilename(t *testing.T) {
	c := adminClient(t)
	c.connect()
	up := c.upload(fmt.Sprintf("SERVICE-A_%d_cat_dog.jpg", time.Now().UnixNano()), randomJPEG(t))
	want := []string{"e2e-service-a/cat", "e2e-service-a/dog"}
	c.waitTags(up.ID, want)
	// Immich's own jobs (sidecar write, re-extraction) must not undo the tags.
	c.waitIdle()
	if got := c.assetTags(up.ID); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("tags after Immich jobs: %v", got)
	}

	other := c.upload("HASH.jpg", randomJPEG(t))
	c.waitIdle()
	if got := c.assetTags(other.ID); len(got) != 0 {
		t.Errorf("flow ran for non-matching file: %v", got)
	}
}

func TestFlowHashTags(t *testing.T) {
	c := adminClient(t)
	c.connect()
	data := randomJPEG(t)
	up := c.upload("HASHED_x.jpg", data)
	md, s1, s256 := md5.Sum(data), sha1.Sum(data), sha256.Sum256(data)
	// No converter in the e2e stack: original and stored bytes are equal.
	c.waitTags(up.ID, []string{
		"e2e-hashes/MD5/ORIGINAL/" + hex.EncodeToString(md[:]),
		"e2e-hashes/MD5/STORED/" + hex.EncodeToString(md[:]),
		"e2e-hashes/SHA1/STORED/" + hex.EncodeToString(s1[:]),
		"e2e-hashes/SHA256/ORIGINAL/" + hex.EncodeToString(s256[:]),
	})

	// metadata.set runs after tag.assign in the same flow.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var meta []struct {
			Key   string            `json:"key"`
			Value map[string]string `json:"value"`
		}
		c.json(http.MethodGet, "/api/assets/"+up.ID+"/metadata", nil, &meta)
		for _, m := range meta {
			if m.Key == "immich-flows.hashes" && m.Value["originalMd5"] == hex.EncodeToString(md[:]) &&
				m.Value["storedSha1"] == hex.EncodeToString(s1[:]) {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("metadata: %+v", meta)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestFlowTrashAndRestore(t *testing.T) {
	c := adminClient(t)
	c.connect()
	up := c.upload("TRASH_x.jpg", randomJPEG(t))
	c.waitIdle()

	if s := c.json(http.MethodDelete, "/api/assets", map[string]any{"ids": []string{up.ID}}, nil); s != http.StatusNoContent {
		t.Fatalf("trash: %d", s)
	}
	c.waitTags(up.ID, []string{"e2e-lifecycle/trashed"})

	if s := c.json(http.MethodPost, "/api/trash/restore/assets", map[string]any{"ids": []string{up.ID}}, nil); s != http.StatusOK {
		t.Fatalf("restore: %d", s)
	}
	c.waitTags(up.ID, []string{"e2e-lifecycle/restored", "e2e-lifecycle/trashed"})
}

func TestFlowDuplicatesResolved(t *testing.T) {
	c := adminClient(t)
	c.connect()
	keep := c.upload("dup-keep.jpg", randomJPEG(t)).ID
	trash := c.upload("dup-trash.jpg", randomJPEG(t)).ID
	c.waitIdle()
	c.tagAsset(keep, "e2e-dup/a")
	c.tagAsset(trash, "e2e-dup/b")

	// Duplicate detection needs ML; mark the pair as duplicates directly.
	group := fmt.Sprintf("%08x-0000-4000-8000-000000000000", time.Now().Unix()&0xffffffff)
	if s := c.json(http.MethodPut, "/api/assets", map[string]any{"ids": []string{keep, trash}, "duplicateId": group}, nil); s != http.StatusNoContent {
		t.Fatalf("mark duplicates: %d", s)
	}
	c.waitIdle()

	var res []struct {
		Success bool `json:"success"`
	}
	body := map[string]any{"groups": []map[string]any{{"duplicateId": group, "keepAssetIds": []string{keep}, "trashAssetIds": []string{trash}}}}
	if s := c.json(http.MethodPost, "/api/duplicates/resolve", body, &res); s != http.StatusOK || len(res) != 1 || !res[0].Success {
		t.Fatalf("resolve: %d %+v", s, res)
	}
	// Immich merges the trashed asset's tags itself; the flow adds its own tag.
	c.waitTags(keep, []string{"e2e-dup/a", "e2e-dup/b", "e2e-dup/resolved-1"})
}

func TestFlowOnTagAssigned(t *testing.T) {
	c := adminClient(t)
	c.connect()
	id := c.upload("tagged.jpg", randomJPEG(t)).ID
	c.waitIdle()
	c.tagAsset(id, "e2e-trigger/sunset")
	c.waitTags(id, []string{"e2e-trigger/sunset", "e2e-triggered/sunset"})
	// The flow's own tag.assign goes directly to Immich: no second event, no loop.
	c.waitIdle()
	if got := c.assetTags(id); len(got) != 2 {
		t.Errorf("tags: %v", got)
	}
}

func TestManualRunBackfill(t *testing.T) {
	c := adminClient(t)
	c.json(http.MethodDelete, "/_flows/api/connection", nil, nil)
	id := c.upload(fmt.Sprintf("SERVICE-A_%d_old_photo.jpg", time.Now().UnixNano()), randomJPEG(t)).ID
	c.waitIdle()
	if got := c.assetTags(id); len(got) != 0 {
		t.Fatalf("flow ran without connection: %v", got)
	}

	c.connect()
	var res struct {
		Queued int `json:"queued"`
	}
	if s := c.json(http.MethodPost, "/_flows/api/flows/e2e-service-tags/run", map[string]any{"assetIds": []string{id}}, &res); s != http.StatusAccepted || res.Queued != 1 {
		t.Fatalf("run: %d %+v", s, res)
	}
	c.waitTags(id, []string{"e2e-service-a/old", "e2e-service-a/photo"})

	if s := c.json(http.MethodPost, "/_flows/api/flows/missing/run", map[string]any{"all": true}, nil); s != http.StatusNotFound {
		t.Errorf("unknown flow: %d", s)
	}
	if s := c.json(http.MethodPost, "/_flows/api/flows/e2e-service-tags/run", map[string]any{"assetIds": []string{"x"}}, nil); s != http.StatusBadRequest {
		t.Errorf("invalid id: %d", s)
	}
}
