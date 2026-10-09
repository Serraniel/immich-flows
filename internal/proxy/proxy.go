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

type proxy struct {
	rp  *httputil.ReverseProxy
	log *slog.Logger
}

// New returns a handler that serves immich-flows routes under Prefix, runs hooks on matching
// routes and proxies everything to upstream.
func New(upstream *url.URL, log *slog.Logger, hooks ...Hook) http.Handler {
	p := &proxy{log: log}
	p.rp = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(upstream)
			r.SetXForwarded()
			r.Out.Host = r.In.Host
			if ex := exchangeFrom(r.In.Context()); ex != nil && len(ex.post) > 0 {
				// Hooks need to read the response. Without a client value the transport negotiates gzip and decompresses transparently.
				r.Out.Header.Del("Accept-Encoding")
			}
		},
		ModifyResponse: modifyResponse,
		FlushInterval:  -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Error("upstream request failed", "method", r.Method, "path", r.URL.Path, "err", err)
			if ex := exchangeFrom(r.Context()); ex != nil {
				ex.Status = http.StatusBadGateway
			}
			w.WriteHeader(http.StatusBadGateway)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+Prefix+"/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	byRoute := map[string][]Hook{}
	var order []string
	for _, h := range hooks {
		if _, ok := byRoute[h.Route]; !ok {
			order = append(order, h.Route)
		}
		byRoute[h.Route] = append(byRoute[h.Route], h)
	}
	for _, route := range order {
		mux.Handle(route, p.hooked(byRoute[route]))
	}

	mux.Handle("/", p.rp)
	return mux
}
