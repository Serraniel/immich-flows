//go:build e2e

package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"
	"time"
)

// Plain HTTP helpers on purpose: tests exercise the wire protocol, not the generated client.

var baseURL = func() string {
	if u := os.Getenv("E2E_URL"); u != "" {
		return u
	}
	return "http://localhost:22830"
}()

type client struct {
	t      *testing.T
	apiKey string
}

func (c *client) do(method, path string, body io.Reader, contentType string, out any) int {
	c.t.Helper()
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		c.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(res.Body)
	if out != nil && res.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %q: %v", method, path, data, err)
		}
	}
	if res.StatusCode >= 300 {
		c.t.Logf("%s %s: %d %s", method, path, res.StatusCode, data)
	}
	return res.StatusCode
}

func (c *client) json(method, path string, in, out any) int {
	c.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	return c.do(method, path, body, "application/json", out)
}

func waitReady() error {
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		res, err := http.Get(baseURL + "/api/server/ping")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("immich not reachable through immich-flows at %s", baseURL)
}

// adminClient signs up the admin (first run only), logs in and creates an API key.
func adminClient(t *testing.T) *client {
	t.Helper()
	c := &client{t: t}
	creds := map[string]string{"email": "admin@e2e.test", "password": "e2e-password", "name": "Admin"}
	c.json(http.MethodPost, "/api/auth/admin-sign-up", creds, nil)

	var login struct{ AccessToken string }
	if s := c.json(http.MethodPost, "/api/auth/login", creds, &login); s != http.StatusCreated {
		t.Fatalf("login: %d", s)
	}

	req, _ := http.NewRequest(http.MethodPost, baseURL+"/api/api-keys",
		bytes.NewReader([]byte(fmt.Sprintf(`{"name":"e2e-%d","permissions":["all"]}`, time.Now().UnixNano()))))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+login.AccessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var key struct{ Secret string }
	if err := json.NewDecoder(res.Body).Decode(&key); err != nil || key.Secret == "" {
		t.Fatalf("create api key: %d %v", res.StatusCode, err)
	}
	c.apiKey = key.Secret
	return c
}

// randomJPEG returns a unique small JPEG so uploads are never duplicates.
func randomJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	if _, err := rand.Read(img.Pix); err != nil {
		t.Fatal(err)
	}
	img.Set(0, 0, color.RGBA{A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type uploaded struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (c *client) upload(name string, data []byte) uploaded {
	c.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	now := time.Now().UTC().Format(time.RFC3339)
	_ = w.WriteField("fileCreatedAt", now)
	_ = w.WriteField("fileModifiedAt", now)
	_ = w.WriteField("filename", name)
	part, _ := w.CreateFormFile("assetData", name)
	_, _ = part.Write(data)
	_ = w.Close()

	var out uploaded
	if s := c.do(http.MethodPost, "/api/assets", &body, w.FormDataContentType(), &out); s != http.StatusCreated && s != http.StatusOK {
		c.t.Fatalf("upload: %d", s)
	}
	return out
}
