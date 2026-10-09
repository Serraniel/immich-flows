//go:build e2e

package e2e

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := waitReady(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestHealthz(t *testing.T) {
	res, err := http.Get(baseURL + "/_flows/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestUploadThroughProxy(t *testing.T) {
	c := adminClient(t)
	data := randomJPEG(t)

	up := c.upload("SERVICE-A_1_cat_dog.jpg", data)
	if up.Status != "created" {
		t.Fatalf("status %q", up.Status)
	}

	var asset struct {
		OriginalFileName string `json:"originalFileName"`
		Checksum         string `json:"checksum"`
	}
	if s := c.json(http.MethodGet, "/api/assets/"+up.ID, nil, &asset); s != http.StatusOK {
		t.Fatalf("get asset: %d", s)
	}
	sum := sha1.Sum(data)
	if asset.OriginalFileName != "SERVICE-A_1_cat_dog.jpg" || asset.Checksum != base64.StdEncoding.EncodeToString(sum[:]) {
		t.Errorf("asset: %+v", asset)
	}

	if again := c.upload("again.jpg", data); again.Status != "duplicate" || again.ID != up.ID {
		t.Errorf("re-upload: %+v", again)
	}
}

func TestTagsThroughProxy(t *testing.T) {
	c := adminClient(t)
	up := c.upload("tag.jpg", randomJPEG(t))

	var tags []struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	}
	if s := c.json(http.MethodPut, "/api/tags", map[string]any{"tags": []string{"e2e/a/b"}}, &tags); s != http.StatusOK {
		t.Fatalf("upsert: %d", s)
	}
	leaf := tags[len(tags)-1]
	if leaf.Value != "e2e/a/b" {
		t.Fatalf("tags: %+v", tags)
	}
	if s := c.json(http.MethodPut, "/api/tags/assets", map[string]any{"tagIds": []string{leaf.ID}, "assetIds": []string{up.ID}}, nil); s != http.StatusOK {
		t.Fatalf("bulk tag: %d", s)
	}

	var asset struct {
		Tags []struct {
			Value string `json:"value"`
		} `json:"tags"`
	}
	c.json(http.MethodGet, "/api/assets/"+up.ID, nil, &asset)
	if len(asset.Tags) != 1 || asset.Tags[0].Value != "e2e/a/b" {
		t.Errorf("asset tags: %+v", asset.Tags)
	}
}
