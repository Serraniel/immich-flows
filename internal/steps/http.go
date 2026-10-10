package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/queue"
)

func init() { register(httpStep()) }

const (
	httpMaxBody    = 1 << 20
	httpTimeout    = 30 * time.Second
	httpMaxTimeout = 120 * time.Second
)

var httpMethods = []any{"GET", "POST", "PUT", "PATCH", "DELETE"}

// hostAllowed matches host against patterns: exact, "*.example.com" (subdomains) or "*".
func hostAllowed(patterns []string, host string) bool {
	host = strings.ToLower(host)
	for _, p := range patterns {
		switch {
		case p == "*", p == host:
			return true
		case strings.HasPrefix(p, "*.") && strings.HasSuffix(host, p[1:]):
			return true
		}
	}
	return false
}

func httpStep() engine.StepType {
	return engine.StepType{
		Name:        "http",
		Description: "Call an HTTP endpoint. Hosts must be listed in IMMICH_FLOWS_HTTP_ALLOW.",
		Schema: schema(map[string]any{
			"url":     prop("string", "http(s) URL"),
			"method":  map[string]any{"type": "string", "enum": httpMethods, "default": "GET"},
			"headers": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
			"body":    map[string]any{"description": "String, or any other value sent as JSON"},
			"timeout": map[string]any{"type": "integer", "description": "Seconds, default 30, max 120"},
			"failOnError": map[string]any{
				"type": "boolean", "default": true,
				"description": "Fail on status >= 400 (5xx and 429 are retried). If false, the status is in the output",
			},
		}, "url"),
		Validate: func(with map[string]any) error { return requireKeys(with, "url") },
		Run: func(ctx context.Context, sc *engine.StepContext, with map[string]any) (map[string]any, error) {
			u, err := url.Parse(str(with, "url", ""))
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return nil, queue.Permanent(fmt.Errorf("invalid url %q", str(with, "url", "")))
			}
			allow := sc.Deps.HTTPAllow
			if !hostAllowed(allow, u.Hostname()) {
				return nil, queue.Permanent(fmt.Errorf("host %q not allowed; add it to IMMICH_FLOWS_HTTP_ALLOW", u.Hostname()))
			}
			method := strings.ToUpper(str(with, "method", "GET"))
			failOnError, err := boolean(with, "failOnError", true)
			if err != nil {
				return nil, queue.Permanent(err)
			}
			timeout := httpTimeout
			if v, ok := with["timeout"].(float64); ok && v > 0 {
				timeout = min(time.Duration(v*float64(time.Second)), httpMaxTimeout)
			}

			var body io.Reader
			contentType := ""
			switch b := with["body"].(type) {
			case nil:
			case string:
				body = strings.NewReader(b)
			default:
				j, err := json.Marshal(b)
				if err != nil {
					return nil, queue.Permanent(fmt.Errorf("body: %w", err))
				}
				body, contentType = bytes.NewReader(j), "application/json"
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
			if err != nil {
				return nil, queue.Permanent(err)
			}
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			if hs, ok := with["headers"].(map[string]any); ok {
				for k, v := range hs {
					req.Header.Set(k, fmt.Sprint(v))
				}
			}

			c := http.Client{Transport: http.DefaultTransport}
			if sc.Deps.HTTP != nil {
				c = *sc.Deps.HTTP
			}
			c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				if !hostAllowed(allow, r.URL.Hostname()) {
					return fmt.Errorf("redirect to host %q not allowed", r.URL.Hostname())
				}
				return nil
			}
			res, err := c.Do(req) // #nosec G704 -- host checked against the admin's allowlist
			if err != nil {
				return nil, err
			}
			defer func() { _ = res.Body.Close() }()
			data, err := io.ReadAll(io.LimitReader(res.Body, httpMaxBody+1))
			if err != nil {
				return nil, err
			}
			if len(data) > httpMaxBody {
				return nil, queue.Permanent(fmt.Errorf("response larger than %d bytes", httpMaxBody))
			}

			out := map[string]any{"status": res.StatusCode, "body": string(data)}
			headers := map[string]any{}
			for k := range res.Header {
				headers[strings.ToLower(k)] = res.Header.Get(k)
			}
			out["headers"] = headers
			if strings.Contains(res.Header.Get("Content-Type"), "json") {
				var v any
				if json.Unmarshal(data, &v) == nil {
					out["json"] = v
				}
			}
			if failOnError && res.StatusCode >= 400 {
				err := fmt.Errorf("%s %s: status %d: %.200s", method, u.Redacted(), res.StatusCode, data)
				if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests {
					return nil, err
				}
				return nil, queue.Permanent(err)
			}
			return out, nil
		},
	}
}
