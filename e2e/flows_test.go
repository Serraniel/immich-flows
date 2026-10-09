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
}
