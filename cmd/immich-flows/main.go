// Command immich-flows is a workflow-enabled reverse proxy for Immich.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/serraniel/immich-flows/internal/config"
	"github.com/serraniel/immich-flows/internal/proxy"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           proxy.New(cfg.Upstream, log),
		ReadHeaderTimeout: 30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("starting", "version", version, "listen", cfg.Listen, "upstream", cfg.Upstream.String())
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck is used by the Docker HEALTHCHECK; the runtime image has no shell or curl.
func healthcheck() int {
	addr := os.Getenv("IMMICH_FLOWS_LISTEN")
	if addr == "" {
		addr = ":2283"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	c := http.Client{Timeout: 5 * time.Second}
	res, err := c.Get("http://127.0.0.1:" + port + proxy.Prefix + "/healthz") // #nosec G704 -- loopback only
	if err != nil {
		return 1
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
