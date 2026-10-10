//go:build e2e

package e2e

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
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
	// Tags set before Immich's metadata extraction can be overwritten by it.
	c.waitIdle()

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

// Immich's web UI uses socket.io over WebSocket; upgrades must pass through the proxy.
func TestWebSocketUpgrade(t *testing.T) {
	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	_, err = fmt.Fprintf(conn, "GET /api/socket.io/?EIO=4&transport=websocket HTTP/1.1\r\nHost: %s\r\n"+
		"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d", res.StatusCode)
	}
}
