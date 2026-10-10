package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	defaultPreTimeout = 5 * time.Second
	maxCapture        = 1 << 20
)

// Hook intercepts requests matching Route, an http.ServeMux pattern such as "POST /api/assets".
type Hook struct {
	Route string
	// Pre runs before the request is forwarded. It may wrap Request.Body to observe it while streaming.
	// Errors and timeouts are logged; the request is forwarded anyway.
	Pre     func(ctx context.Context, ex *Exchange) error
	Timeout time.Duration // for Pre; default 5s
	// Post runs once the response has been sent to the client.
	Post func(ex *Exchange)
}

// Exchange is one proxied request/response seen by hooks.
type Exchange struct {
	Request *http.Request
	// Set before Post.
	Status   int
	Response []byte // response body if it is valid JSON (any Content-Type) and at most 1 MiB
	Complete bool   // response body fully read from upstream

	vals map[any]any
	once sync.Once
	post []func(*Exchange)
}

// Set stores a value for later hooks of the same exchange.
func (e *Exchange) Set(key, val any) {
	if e.vals == nil {
		e.vals = map[any]any{}
	}
	e.vals[key] = val
}

// Get returns a value stored with Set.
func (e *Exchange) Get(key any) any { return e.vals[key] }

func (e *Exchange) finish() {
	e.once.Do(func() {
		for _, fn := range e.post {
			fn(e)
		}
	})
}

type exchangeKey struct{}

func exchangeFrom(ctx context.Context) *Exchange {
	ex, _ := ctx.Value(exchangeKey{}).(*Exchange)
	return ex
}

func (p *proxy) hooked(hooks []Hook) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ex := &Exchange{Request: r}
		for _, h := range hooks {
			if h.Post != nil {
				ex.post = append(ex.post, h.Post)
			}
		}
		for _, h := range hooks {
			if h.Pre == nil {
				continue
			}
			timeout := h.Timeout
			if timeout == 0 {
				timeout = defaultPreTimeout
			}
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			if err := h.Pre(ctx, ex); err != nil {
				p.log.Warn("pre hook failed", "route", h.Route, "err", err)
			}
			cancel()
		}
		ex.Request = ex.Request.WithContext(context.WithValue(ex.Request.Context(), exchangeKey{}, ex))
		p.rp.ServeHTTP(w, ex.Request)
		// Covers responses whose body was never wrapped (e.g. HEAD, upstream error).
		ex.finish()
	})
}

func modifyResponse(res *http.Response) error {
	ex := exchangeFrom(res.Request.Context())
	if ex == nil || len(ex.post) == 0 {
		return nil
	}
	ex.Status = res.StatusCode
	// Content-Type is not trusted: proxies in the chain (e.g. immich-upload-optimizer) answer
	// with text/plain. Hooked routes are API calls with small bodies.
	res.Body = &capture{ReadCloser: res.Body, ex: ex}
	return nil
}

// capture copies up to maxCapture bytes of the response while it streams to the client.
type capture struct {
	io.ReadCloser
	ex       *Exchange
	buf      []byte
	overflow bool
}

func (c *capture) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	if !c.overflow {
		if len(c.buf)+n > maxCapture {
			c.overflow, c.buf = true, nil
		} else {
			c.buf = append(c.buf, p[:n]...)
		}
	}
	if err == io.EOF {
		c.ex.Complete = true
	}
	return n, err
}

func (c *capture) Close() error {
	err := c.ReadCloser.Close()
	if !c.overflow && json.Valid(c.buf) {
		c.ex.Response = c.buf
	}
	c.ex.finish()
	return err
}
