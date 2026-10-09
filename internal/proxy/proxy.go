// Package proxy forwards all traffic to the upstream and serves immich-flows' own routes.
package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Prefix for immich-flows' own routes. Must not collide with Immich routes.
const Prefix = "/_flows"

// New returns a handler that serves immich-flows routes under Prefix and proxies everything else to upstream.
func New(upstream *url.URL, log *slog.Logger) http.Handler {
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(upstream)
			r.SetXForwarded()
			r.Out.Host = r.In.Host
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("upstream request failed", "method", r.Method, "path", r.URL.Path, "err", err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+Prefix+"/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", rp)
	return mux
}
