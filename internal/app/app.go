// Package app wires the service together.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/serraniel/immich-flows/internal/api"
	"github.com/serraniel/immich-flows/internal/auth"
	"github.com/serraniel/immich-flows/internal/config"
	"github.com/serraniel/immich-flows/internal/connect"
	"github.com/serraniel/immich-flows/internal/engine"
	"github.com/serraniel/immich-flows/internal/events"
	"github.com/serraniel/immich-flows/internal/flows"
	"github.com/serraniel/immich-flows/internal/hooks"
	"github.com/serraniel/immich-flows/internal/immich"
	"github.com/serraniel/immich-flows/internal/proxy"
	"github.com/serraniel/immich-flows/internal/queue"
	"github.com/serraniel/immich-flows/internal/steps"
	"github.com/serraniel/immich-flows/internal/store"
	"github.com/serraniel/immich-flows/internal/web"
)

// Run serves until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Config, log *slog.Logger, version string) error {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	db, err := store.Open(ctx, filepath.Join(cfg.DataDir, "immich-flows.db"))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	key, err := connect.LoadKey(cfg.Secret, cfg.DataDir)
	if err != nil {
		return fmt.Errorf("load secret: %w", err)
	}
	conn := connect.New(db, key, cfg.Immich, nil)
	users := auth.New(cfg.Immich, nil)

	reg := engine.NewRegistry()
	steps.Register(reg)
	flowDir := flows.NewDir(cfg.FlowsDir, reg, log)
	if err := flowDir.Load(); err != nil {
		return fmt.Errorf("load flows: %w", err)
	}
	go flowDir.Watch(ctx, 2*time.Second)

	jobs := queue.New(db, log, queue.Options{Workers: cfg.Workers})
	clients := userClients(conn)
	eng := engine.New(reg, db, flowDir, engine.Deps{
		Immich:    clients,
		LoadAsset: engine.ImmichAssets(clients),
	}, log)
	eng.Register(jobs)

	hookDeps := hooks.Deps{
		Users:   users,
		Publish: func(ctx context.Context, ev events.Event) error { return engine.Publish(ctx, jobs, ev) },
		Log:     log,
		Immich:  cfg.Immich,
		Wants:   eng.Wants,
	}
	px := proxy.New(cfg.Upstream, log, append([]proxy.Hook{hooks.Upload(hookDeps), hooks.Duplicates(hookDeps)}, hooks.Trash(hookDeps)...)...)

	jobsCtx, stopJobs := context.WithCancel(context.WithoutCancel(ctx))
	jobsDone := make(chan error, 1)
	go func() { jobsDone <- jobs.Run(jobsCtx) }()
	defer func() {
		stopJobs()
		if err := <-jobsDone; err != nil {
			log.Error("job queue", "err", err)
		}
	}()

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           routes(px, api.New(proxy.Prefix+"/api", cfg.Immich, users, conn, flowDir, log)),
		ReadHeaderTimeout: 30 * time.Second,
	}
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

// userClients returns Immich clients acting as a connected user. Unconnected users fail permanently.
func userClients(conn *connect.Manager) engine.ClientFunc {
	return func(ctx context.Context, userID string) (*immich.ClientWithResponses, error) {
		c, err := conn.Client(ctx, userID)
		if errors.Is(err, connect.ErrNotConnected) {
			return nil, queue.Permanent(err)
		}
		return c, err
	}
}

func routes(px, apiHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(proxy.Prefix+"/", web.Handler(proxy.Prefix))
	mux.Handle(proxy.Prefix+"/api/", apiHandler)
	mux.Handle("GET "+proxy.Prefix+"/healthz", px)
	mux.Handle("/", px)
	return mux
}
