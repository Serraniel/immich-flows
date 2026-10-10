//go:build e2e && perf

package e2e

// Load and memory comparison: immich-flows vs. direct Immich. Not run in CI.
//
//	cd e2e && docker compose up -d --build --wait
//	go test -tags e2e,perf -run TestPerf -v -timeout 30m ./e2e/
//
// Results: docs/performance.md.

import (
	"crypto/rand"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	perfContainer = "immich-flows-e2e-immich-flows-1"
	uploadSize    = 50 << 20
	uploadConc    = 8
	uploadsPerRun = 16
	apiRequests   = 2000
	apiConc       = 16
	rounds        = 3
)

// streamUpload uploads size random bytes without holding them in memory.
func streamUpload(base, key string, size int64) error {
	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	go func() {
		now := time.Now().UTC().Format(time.RFC3339)
		_ = w.WriteField("fileCreatedAt", now)
		_ = w.WriteField("fileModifiedAt", now)
		name := fmt.Sprintf("perf-%d.jpg", time.Now().UnixNano())
		part, err := w.CreateFormFile("assetData", name)
		if err == nil {
			_, err = io.CopyN(part, rand.Reader, size)
		}
		if err == nil {
			err = w.Close()
		}
		pw.CloseWithError(err)
	}()
	req, err := http.NewRequest(http.MethodPost, base+"/api/assets", pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("x-api-key", key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != http.StatusCreated {
		return fmt.Errorf("upload: %d", res.StatusCode)
	}
	return nil
}

// parallel runs n calls of f with conc workers and returns the wall time and per-call latencies.
func parallel(n, conc int, f func() error) (time.Duration, []time.Duration, error) {
	var next atomic.Int64
	lat := make([]time.Duration, n)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	start := time.Now()
	for range conc {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				t := time.Now()
				err := f()
				lat[i] = time.Since(t)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	return time.Since(start), lat, firstErr
}

func pct(lat []time.Duration, p float64) time.Duration {
	s := slices.Clone(lat)
	slices.Sort(s)
	return s[int(float64(len(s)-1)*p)]
}

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	return s[len(s)/2]
}

// memSampler records the container's peak memory while running.
type memSampler struct {
	stop chan struct{}
	done chan struct{}
	peak atomic.Int64
}

func sampleMem(t *testing.T) *memSampler {
	m := &memSampler{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(m.done)
		for {
			out, err := exec.Command("docker", "stats", "--no-stream", "--format", "{{.MemUsage}}", perfContainer).Output()
			if err == nil {
				if b := parseBytes(strings.TrimSpace(strings.Split(string(out), "/")[0])); b > m.peak.Load() {
					m.peak.Store(b)
				}
			} else {
				t.Logf("docker stats: %v", err)
			}
			select {
			case <-m.stop:
				return
			default:
			}
		}
	}()
	return m
}

func (m *memSampler) Stop() int64 { close(m.stop); <-m.done; return m.peak.Load() }

func parseBytes(s string) int64 {
	units := []struct {
		suffix string
		mult   float64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"B", 1}}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			f, _ := strconv.ParseFloat(strings.TrimSuffix(s, u.suffix), 64)
			return int64(f * u.mult)
		}
	}
	return 0
}

func TestPerf(t *testing.T) {
	c := adminClient(t)
	c.connect() // flows run for this user, like in production
	targets := []struct{ name, base string }{{"direct", immichURL}, {"proxy", baseURL}}

	idle := sampleMem(t)
	time.Sleep(3 * time.Second)
	t.Logf("immich-flows memory idle: %.0f MiB", float64(idle.Stop())/(1<<20))

	t.Run("api", func(t *testing.T) {
		for _, path := range []string{"/api/server/ping", "/api/users/me"} {
			p50s, p95s, rps := map[string][]float64{}, map[string][]float64{}, map[string][]float64{}
			for range rounds {
				for _, tg := range targets {
					wall, lat, err := parallel(apiRequests, apiConc, func() error {
						req, _ := http.NewRequest(http.MethodGet, tg.base+path, nil)
						req.Header.Set("x-api-key", c.apiKey)
						res, err := http.DefaultClient.Do(req)
						if err != nil {
							return err
						}
						_, _ = io.Copy(io.Discard, res.Body)
						_ = res.Body.Close()
						if res.StatusCode != http.StatusOK {
							return fmt.Errorf("%s: %d", path, res.StatusCode)
						}
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
					p50s[tg.name] = append(p50s[tg.name], float64(pct(lat, 0.5).Microseconds())/1000)
					p95s[tg.name] = append(p95s[tg.name], float64(pct(lat, 0.95).Microseconds())/1000)
					rps[tg.name] = append(rps[tg.name], float64(apiRequests)/wall.Seconds())
				}
			}
			for _, tg := range targets {
				t.Logf("GET %-18s %-6s p50 %6.2f ms  p95 %6.2f ms  %6.0f req/s", path, tg.name,
					median(p50s[tg.name]), median(p95s[tg.name]), median(rps[tg.name]))
			}
		}
	})

	t.Run("upload", func(t *testing.T) {
		mbps := map[string][]float64{}
		var peak int64
		for range rounds {
			for _, tg := range targets {
				var mem *memSampler
				if tg.name == "proxy" {
					mem = sampleMem(t)
				}
				wall, _, err := parallel(uploadsPerRun, uploadConc, func() error {
					return streamUpload(tg.base, c.apiKey, uploadSize)
				})
				if mem != nil {
					peak = max(peak, mem.Stop())
				}
				if err != nil {
					t.Fatal(err)
				}
				mbps[tg.name] = append(mbps[tg.name], float64(uploadsPerRun*uploadSize)/(1<<20)/wall.Seconds())
			}
		}
		for _, tg := range targets {
			t.Logf("upload %d x %d MiB, %d parallel, %-6s %6.0f MiB/s", uploadsPerRun, uploadSize>>20, uploadConc, tg.name, median(mbps[tg.name]))
		}
		t.Logf("immich-flows peak memory during uploads: %.0f MiB", float64(peak)/(1<<20))
	})
}
