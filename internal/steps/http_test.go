package steps

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/queue"
)

func TestHostAllowed(t *testing.T) {
	allow := []string{"api.example.com", "*.svc.local"}
	for host, want := range map[string]bool{
		"api.example.com": true, "API.example.com": true, "x.svc.local": true,
		"svc.local": false, "evil.com": false, "api.example.com.evil.com": false,
	} {
		if got := hostAllowed(allow, host); got != want {
			t.Errorf("%s: %v", host, got)
		}
	}
	if !hostAllowed([]string{"*"}, "anything") || hostAllowed(nil, "anything") {
		t.Error("wildcard / empty")
	}
}

func TestHTTPStep(t *testing.T) {
	var got struct {
		method, ct, auth string
		body             map[string]any
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fail":
			w.WriteHeader(http.StatusNotFound)
			return
		case "/busy":
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		case "/redirect":
			http.Redirect(w, r, "http://evil.invalid/", http.StatusFound)
			return
		}
		got.method, got.ct, got.auth = r.Method, r.Header.Get("Content-Type"), r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"n":2}`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	deps := engine.Deps{HTTPAllow: []string{u.Hostname()}}

	flow := func(path, extra string) string {
		return `id: f
trigger: {on: manual}
steps:
  - id: call
    use: http
    with:
      url: '` + srv.URL + path + `'
` + extra + `
  - use: capture
`
	}

	r := runFlow(t, flow("/hook", `      method: POST
      headers: {Authorization: 'Bearer {{ asset.originalFileName }}'}
      body: {name: '{{ asset.originalFileName }}', tags: [a, b]}`), map[string]any{"originalFileName": "x.jpg"}, deps)
	if r.err != nil {
		t.Fatal(r.err)
	}
	out := r.outputs["call"]
	if out["status"] != float64(200) || out["json"].(map[string]any)["n"] != float64(2) {
		t.Errorf("output %+v", out)
	}
	if got.method != "POST" || got.ct != "application/json" || got.auth != "Bearer x.jpg" ||
		got.body["name"] != "x.jpg" || len(got.body["tags"].([]any)) != 2 {
		t.Errorf("request %+v", got)
	}

	if r := runFlow(t, flow("/fail", ""), nil, deps); r.err == nil || !queue.IsPermanent(r.err) {
		t.Errorf("404: %v", r.err)
	}
	if r := runFlow(t, flow("/busy", ""), nil, deps); r.err == nil || queue.IsPermanent(r.err) {
		t.Errorf("503 should be retried: %v", r.err)
	}
	if r := runFlow(t, flow("/fail", "      failOnError: false"), nil, deps); r.err != nil || r.outputs["call"]["status"] != float64(404) {
		t.Errorf("failOnError false: %v %+v", r.err, r.outputs)
	}
	if r := runFlow(t, flow("/redirect", ""), nil, deps); r.err == nil || !strings.Contains(r.err.Error(), "not allowed") {
		t.Errorf("redirect: %v", r.err)
	}
	if r := runFlow(t, flow("/hook", ""), nil, engine.Deps{}); r.err == nil || !strings.Contains(r.err.Error(), "IMMICH_FLOWS_HTTP_ALLOW") {
		t.Errorf("not allowed: %v", r.err)
	}
}
