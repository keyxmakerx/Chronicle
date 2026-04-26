// Package main is the entry point for the Chronicle server. It loads
// configuration, establishes database connections, wires together all
// plugins/systems/widgets, and starts the HTTP server.
package main

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/keyxmakerx/chronicle/internal/app"
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/bestiary"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

func main() {
	// --- Load Configuration ---
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(1)
	}

	// Configure structured logging based on environment.
	setupLogging(cfg)

	slog.Info("starting Chronicle",
		slog.String("env", cfg.Env),
		slog.Int("port", cfg.Port),
	)

	// --- Connect to MariaDB ---
	db, err := database.NewMariaDB(cfg.Database)
	if err != nil {
		slog.Error("failed to connect to MariaDB", slog.Any("error", err))
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("connected to MariaDB")

	// --- Run Database Migrations ---
	// Auto-apply pending migrations on every startup. Already-applied
	// migrations are skipped. This eliminates the need to run migrate
	// manually after deployment.

	// Pre-migration backup: capture DB + media + Redis before applying any
	// schema changes. When BACKUP_REQUIRED=1 the boot aborts on any
	// capture failure; otherwise (legacy default) the failure is logged
	// and migrations proceed. See ADR-035/ADR-036/ADR-037 in
	// .ai/decisions.md for the policy.
	backupRequired := strings.EqualFold(getEnvDefault("BACKUP_REQUIRED", ""), "1") ||
		strings.EqualFold(getEnvDefault("BACKUP_REQUIRED", ""), "true")
	preMigrateErr := database.PreMigrationBackup(db, database.HealthCheckConfig{
		BackupDir:      cfg.BackupDir,
		BackupRequired: backupRequired,
		MediaPath:      cfg.Upload.MediaPath,
		RedisURL:       cfg.Redis.URL,
		DBName:         cfg.Database.Name,
		DBHost:         cfg.Database.Host,
		DBUser:         cfg.Database.User,
		DBPassword:     cfg.Database.Password,
	})
	if preMigrateErr != nil {
		if backupRequired {
			slog.Error("pre-migration backup failed and BACKUP_REQUIRED=1; refusing to apply migrations",
				slog.Any("error", preMigrateErr),
			)
			os.Exit(1)
		}
		slog.Warn("pre-migration backup failed (non-fatal in default mode); migrations will still apply",
			slog.Any("error", preMigrateErr),
		)
	}

	if err := database.RunMigrations(db, cfg.Database.DSN(), "db/migrations"); err != nil {
		slog.Error("failed to run migrations", slog.Any("error", err))
		os.Exit(1)
	}

	// --- Startup Health Checks ---
	// Validates migration version, schema columns, DB health, and security.
	// Server refuses to start if any fatal check fails.
	if err := database.RunStartupHealthChecks(db, database.HealthCheckConfig{
		ExpectedMigrationVersion: 23,
		CriticalColumns: map[string][]string{
			"campaigns":        {"id", "name", "slug", "archived_at", "join_code", "settings", "sidebar_config"},
			"entities":         {"id", "campaign_id", "name", "slug", "entry", "entry_html", "fields_data", "visibility", "owner_user_id"},
			"users":            {"id", "email", "display_name", "password_hash"},
			"campaign_members": {"campaign_id", "user_id", "role"},
		},
		Env:        cfg.Env,
		BaseURL:    cfg.BaseURL,
		DBTLSMode:  cfg.Database.TLSMode,
		DBPassword: cfg.Database.Password,
		DBHost:     cfg.Database.Host,
		DBUser:     cfg.Database.User,
		DBName:     cfg.Database.Name,
		SmokeTests: []database.SmokeTest{
			campaigns.ScanSmokeTest(),
		},
	}); err != nil {
		slog.Error("startup health checks failed", slog.Any("error", err))
		os.Exit(1)
	}

	// --- Run Plugin Migrations ---
	// Each plugin runs its own schema migrations independently. Failures
	// disable the plugin instead of crashing the app. Migrations are
	// embedded in the binary via embed.FS so they work in any environment.
	pluginHealth := database.NewPluginHealthRegistry()
	pluginSchemas := registeredPlugins()
	pluginResults := database.RunPluginMigrations(db, pluginSchemas)
	for _, r := range pluginResults {
		pluginHealth.Register(r.Slug, r.Healthy, r.Error, r.Version, r.LatestVersion)
	}
	if degraded := pluginHealth.DegradedPlugins(); len(degraded) > 0 {
		slog.Warn("some plugins are degraded — features disabled",
			slog.Any("plugins", degraded),
		)
	}

	// --- Connect to Redis ---
	rdb, err := database.NewRedis(cfg.Redis)
	if err != nil {
		slog.Error("failed to connect to Redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() { _ = rdb.Close() }()
	slog.Info("connected to Redis")

	// --- Initialize Game Systems ---
	// Discover and load system manifests + data from internal/systems/.
	// Systems register their factories via init() (blank imports above).
	if err := systems.Init("internal/systems"); err != nil {
		slog.Warn("system initialization failed", slog.Any("error", err))
	}
	// Also scan package-manager-installed systems.
	systems.ScanPackageDir(filepath.Join(cfg.Upload.MediaPath, "packages", "systems"))

	// --- Create Application ---
	application := app.New(cfg, db, rdb, pluginHealth, pluginSchemas)

	// Register all routes (public, plugin, system, widget, API).
	application.RegisterRoutes()

	// --- Graceful Shutdown ---
	// Listen for interrupt/term signals to drain connections cleanly.
	// This is required for Docker/Cosmos restarts to be seamless.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		slog.Info("shutting down server...")

		// Give in-flight requests 10 seconds to complete.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Unload all WASM plugins before stopping the server.
		if application.WASMPluginManager != nil {
			application.WASMPluginManager.UnloadAll(ctx)
		}

		if err := application.Echo.Shutdown(ctx); err != nil {
			slog.Error("server forced shutdown", slog.Any("error", err))
		}
	}()

	// --- Start Server ---
	if err := application.Start(); err != nil {
		// Echo returns http.ErrServerClosed on graceful shutdown, which is expected.
		slog.Info("server stopped", slog.Any("reason", err))
	}
}

// setupLogging configures the global slog logger based on the environment.
// Development uses text format for readability. Production uses JSON for
// structured log aggregation.
func setupLogging(cfg *config.Config) {
	var handler slog.Handler

	if cfg.IsDevelopment() {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}

	slog.SetDefault(slog.New(handler))
}

// registeredPlugins returns the list of built-in plugins with their embedded
// migration filesystems. Each plugin embeds its own migrations/*.sql files
// getEnvDefault reads an environment variable or returns the fallback.
func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// via Go's embed package, ensuring they're available in the compiled binary
// regardless of working directory.
func registeredPlugins() []database.PluginSchema {
	mustSub := func(fsys fs.FS, dir string) fs.FS {
		sub, err := fs.Sub(fsys, dir)
		if err != nil {
			panic("embedded migrations sub-dir: " + err.Error())
		}
		return sub
	}
	return []database.PluginSchema{
		{Slug: "bestiary", MigrationsFS: mustSub(bestiary.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "calendar", MigrationsFS: mustSub(calendar.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "maps", MigrationsFS: mustSub(maps.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "sessions", MigrationsFS: mustSub(sessions.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "timeline", MigrationsFS: mustSub(timeline.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "syncapi", MigrationsFS: mustSub(syncapi.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "packages", MigrationsFS: mustSub(packages.MigrationsFS, database.PluginMigrationsSubdir)},
	}
}
