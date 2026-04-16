package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	_ "github.com/usepackyard/packyard/internal/provider/github"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/database"
	"github.com/usepackyard/packyard/internal/frontend"
	"github.com/usepackyard/packyard/internal/jobs"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/server"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

var version = "dev"

func main() {
	cfg := config.Load()
	setupLogger(cfg.Log.Level)

	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting packyard", "version", version, "port", cfg.Port, "db_driver", cfg.DB.Driver, "storage", cfg.Storage.Type, "mode", cfg.Mode)

	// Database.
	db, err := database.Open(cfg.DB)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	stores := store.NewStores(db)

	if err := seedDefaults(stores, cfg); err != nil {
		slog.Error("failed to seed defaults", "error", err)
		os.Exit(1)
	}

	// Storage.
	strg, err := storage.New(cfg)
	if err != nil {
		slog.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}

	// Composer metadata cache.
	cache := composer.NewCache(stores.Packages, stores.Orgs, cfg.BaseURL, cfg.Mode)
	if err := cache.RebuildAll(context.Background()); err != nil {
		slog.Error("failed to build initial metadata cache", "error", err)
		os.Exit(1)
	}

	// Frontend filesystem (embedded SPA).
	frontendFS, err := frontend.FS()
	if err != nil {
		slog.Error("failed to load embedded frontend", "error", err)
		os.Exit(1)
	}

	// HTTP server.
	mux := server.NewMux(cfg, stores, strg, cache, frontendFS)
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      server.Wrap(cfg, mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start session cleanup goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cleanupSessions(ctx, stores.Sessions)
	if cfg.DownloadRetentionDays > 0 {
		go cleanupDownloads(ctx, stores.Downloads, cfg.DownloadRetentionDays)
	}

	// Recover any sync jobs that were left in "running" by an ungraceful
	// shutdown before spawning the worker pool — otherwise those rows
	// would stay stuck until the sweeper's first tick.
	if n, err := stores.Jobs.RecoverStuck(ctx, 0); err != nil {
		slog.Error("recover stuck sync jobs at boot", "error", err)
	} else if n > 0 {
		slog.Info("recovered stuck sync jobs at boot", "count", n)
	}

	// Worker pool runs sync jobs in the background; sweeper re-queues
	// any job that goes silent mid-run (worker crash, lost connection).
	syncPool := jobs.NewPool(stores, strg, cache, cfg)
	syncPool.Start(ctx)

	if cfg.JobRetentionDays > 0 {
		go cleanupJobs(ctx, stores.Jobs, cfg.JobRetentionDays)
	}

	// Graceful shutdown.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	cancel()
	slog.Info("server stopped")
}

// seedDefaults provisions a super-admin user from PACKYARD_ADMIN_EMAIL /
// PACKYARD_ADMIN_PASSWORD on first run (empty users table) and a "default"
// organization with the admin as owner, so the dashboard is usable
// immediately after install in both single and multi mode. In multi mode the
// super-admin can then provision additional organizations through the admin
// API or dashboard. Idempotent: no-ops once any user exists.
func seedDefaults(stores *store.Stores, cfg *config.Config) error {
	ctx := context.Background()

	count, err := stores.Users.Count(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := auth.HashPassword(cfg.Admin.Password, cfg.BcryptCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	user := &model.User{
		Email:        cfg.Admin.Email,
		Password:     hash,
		Name:         "Admin",
		IsActive:     true,
		IsSuperAdmin: true,
	}
	if err := stores.Users.Create(ctx, user); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	slog.Info("created super-admin user", "email", cfg.Admin.Email)

	// Provision a default org the admin owns. In multi mode this gives the
	// super-admin an organization to land in on first login; they can
	// create additional tenants through the admin API or dashboard.
	org := &model.Organization{Slug: "default", Name: "Default"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		return fmt.Errorf("create default org: %w", err)
	}
	slog.Info("created default organization")

	member := &model.OrgMember{
		OrgID:  org.ID,
		UserID: user.ID,
		Role:   "owner",
	}
	if err := stores.Orgs.AddMember(ctx, member); err != nil {
		return fmt.Errorf("add admin to default org: %w", err)
	}

	return nil
}

func cleanupSessions(ctx context.Context, sessions store.SessionStore) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := sessions.DeleteExpired(ctx); err != nil {
				slog.Error("failed to cleanup expired sessions", "error", err)
			}
		}
	}
}

// cleanupDownloads trims the download_events table so it doesn't grow
// unbounded. Runs once a day. retentionDays > 0 is a precondition — the
// caller is expected to skip starting this goroutine when retention is
// disabled.
func cleanupDownloads(ctx context.Context, downloads store.DownloadStore, retentionDays int) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Run once at startup so a long-running server doesn't wait 24h to
	// reflect a freshly lowered retention window.
	prune(ctx, downloads, retentionDays)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune(ctx, downloads, retentionDays)
		}
	}
}

func prune(ctx context.Context, downloads store.DownloadStore, retentionDays int) {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	n, err := downloads.PruneOlderThan(ctx, cutoff)
	if err != nil {
		slog.Error("prune download events", "error", err)
		return
	}
	if n > 0 {
		slog.Info("pruned download events", "rows", n, "cutoff", cutoff.Format(time.RFC3339))
	}
}

// cleanupJobs prunes terminal sync_jobs rows older than retentionDays on
// a daily tick. Mirrors cleanupDownloads' shape. The JobStore itself
// guarantees active (queued/running) rows are never touched.
func cleanupJobs(ctx context.Context, jobs store.JobStore, retentionDays int) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	pruneJobs(ctx, jobs, retentionDays)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneJobs(ctx, jobs, retentionDays)
		}
	}
}

func pruneJobs(ctx context.Context, jobs store.JobStore, retentionDays int) {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	n, err := jobs.PruneOlderThan(ctx, cutoff)
	if err != nil {
		slog.Error("prune sync jobs", "error", err)
		return
	}
	if n > 0 {
		slog.Info("pruned sync jobs", "rows", n, "cutoff", cutoff.Format(time.RFC3339))
	}
}

func setupLogger(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
}
