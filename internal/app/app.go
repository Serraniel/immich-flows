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
	"github.com/serraniel/immich-flows/internal/runs"
	"github.com/serraniel/immich-flows/internal/steps"
	"github.com/serraniel/immich-flows/internal/store"
	"github.com/serraniel/immich-flows/internal/trashwatch"
	"github.com/serraniel/immich-flows/internal/web"
	"github.com/serraniel/immich-flows/internal/webhook"
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
		HTTPAllow: cfg.HTTPAllow,
	}, log)
	eng.Register(jobs)

	publish := func(ctx context.Context, ev events.Event) error { return engine.Publish(ctx, jobs, ev) }
	trash := trashwatch.New(trashwatch.Deps{
		DB: db, Users: conn.Users, Clients: conn.Client, Publish: publish, Wants: eng.Wants, Log: log,
	})
	go trash.Run(ctx, cfg.TrashInterval)

	seen := &webhook.Seen{DB: db}
	webhook.Register(jobs, seen, publish)
	flowHook := &webhook.FlowHandler{Key: key, Runner: eng, Jobs: jobs, Log: log}
	hook := &webhook.Handler{Key: key, Jobs: jobs, Wants: eng.Wants, Log: log}

	hookDeps := hooks.Deps{
		Users: users,
		Publish: func(ctx context.Context, ev events.Event) error {
			if ev.Type == events.AssetUploaded {
				// Reported here first; webhook duplicates from Immich workflows are dropped.
				if err := seen.Mark(ctx, ev.AssetIDs...); err != nil {
					log.Warn("webhook: mark upload", "err", err)
				}
			}
			if ev.Type == events.AssetDeleted || ev.Type == events.AssetRestored {
				// Reported by the proxy; the trash reconciler must not report it again.
				if err := trash.Forget(ctx, ev.UserID, ev.AssetIDs); err != nil {
					log.Warn("trash reconcile: forget", "err", err)
				}
			}
			return publish(ctx, ev)
		},
		Log:    log,
		Immich: cfg.Immich,
		Wants:  eng.Wants,
	}
	hookList := []proxy.Hook{hooks.Upload(hookDeps), hooks.Duplicates(hookDeps)}
	hookList = append(hookList, hooks.Trash(hookDeps)...)
	hookList = append(hookList, hooks.Tags(hookDeps)...)
	px := proxy.New(cfg.Upstream, log, hookList...)

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
		Handler:           routes(px, hook, flowHook, api.New(proxy.Prefix+"/api", api.Deps{Immich: cfg.Immich, Auth: users, Conn: conn, Flows: flowDir, Schema: reg, Runner: eng, Jobs: jobs, Runs: runs.New(db), WebhookKey: key, Log: log})),
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

// userClients returns Immich clients acting as a connected user. Runs for unconnected users are skipped.
func userClients(conn *connect.Manager) engine.ClientFunc {
	return func(ctx context.Context, userID string) (*immich.ClientWithResponses, error) {
		c, err := conn.Client(ctx, userID)
		if errors.Is(err, connect.ErrNotConnected) {
			return nil, queue.Permanent(fmt.Errorf("%w: %w", engine.ErrSkip, err))
		}
		return c, err
	}
}

func routes(px, hook, flowHook, apiHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	// Authenticated by token, not by Immich session.
	mux.Handle("POST "+proxy.Prefix+"/api/webhook", hook)
	mux.Handle("POST "+proxy.Prefix+"/api/flows/{id}/webhook", flowHook)
	mux.Handle(proxy.Prefix+"/", web.Handler(proxy.Prefix))
	mux.Handle(proxy.Prefix+"/api/", apiHandler)
	mux.Handle("GET "+proxy.Prefix+"/healthz", px)
	mux.Handle("/", px)
	return mux
}
