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
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/bestiary"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
	"github.com/keyxmakerx/chronicle/internal/plugins/systemstate"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
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
	// Auto-apply pending migrations on startup so no manual migrate step is
	// needed after deployment. MigrateWithBackup backs up only when a
	// migration is actually pending, and tolerates a database AHEAD of this
	// build (rollback, or an applied-but-missing migration) by logging and
	// starting anyway instead of crash-looping. BACKUP_REQUIRED=1 makes a
	// backup failure on the pending path abort the boot. See ADR-035/036/037/044.
	backupRequired := strings.EqualFold(getEnvDefault("BACKUP_REQUIRED", ""), "1") ||
		strings.EqualFold(getEnvDefault("BACKUP_REQUIRED", ""), "true")
	backupCfg := database.HealthCheckConfig{
		BackupDir:      cfg.BackupDir,
		BackupRequired: backupRequired,
		MediaPath:      cfg.Upload.MediaPath,
		RedisURL:       cfg.Redis.URL,
		DBName:         cfg.Database.Name,
		DBHost:         cfg.Database.Host,
		DBUser:         cfg.Database.User,
		DBPassword:     cfg.Database.Password,
	}
	coreBackedUp, err := database.MigrateWithBackup(db, cfg.Database.DSN(), "db/migrations", backupCfg)
	if err != nil {
		fatalBoot("failed to run migrations", err)
	}

	// --- Startup Health Checks ---
	// Validates migration version, schema columns, DB health, and security;
	// refuses to start if any fatal check fails. Config is shared with the
	// admin Database > Health tab via app.StartupHealthCheckConfig so the two
	// surfaces never disagree.
	if err := database.RunStartupHealthChecks(db, app.StartupHealthCheckConfig(cfg)); err != nil {
		fatalBoot("startup health checks failed", err)
	}

	// --- Run Plugin Migrations ---
	// Each plugin runs its own schema migrations independently; failures
	// disable the plugin instead of crashing the app. Migrations are embedded
	// via embed.FS so they work in any environment.
	//
	// Run foundry_vtt's pre-migration check before the migration loop: it
	// refuses to start the server if the (deprecated) foundry_module_versions
	// table has rows, since a manual upload could otherwise be silently
	// dropped by the migration. Idempotent; a no-op once the migration applied.
	if err := foundry_vtt.PreMigrationCheck(context.Background(), db); err != nil {
		slog.Error("foundry_vtt pre-migration check failed", slog.Any("error", err))
		os.Exit(1)
	}
	// The check above only guards the UPGRADE path. On a fresh database,
	// migration 001's RENAME of foundry_module_campaign_tokens has no source
	// table (its plugin was deleted), so it fails on its first statement. The
	// reconciler records 001 as applied wherever it can never succeed, so the
	// runner reaches migration 002's idempotent DDL. A failure here is fatal:
	// it means the schema state cannot even be read, so the migration
	// decision would be a guess.
	if err := foundry_vtt.ReconcileConsolidationState(context.Background(), db); err != nil {
		slog.Error("foundry_vtt consolidation reconcile failed", slog.Any("error", err))
		os.Exit(1)
	}
	pluginHealth := database.NewPluginHealthRegistry()
	pluginSchemas := registeredPlugins()

	// Pre-PLUGIN-migration backup. MigrateWithBackup above guards only CORE
	// migrations, so a release with destructive plugin migrations and no core
	// ones would otherwise ship with no automatic backup. Detect pending
	// plugin migrations the same way the runner will, and back up under the
	// same BACKUP_REQUIRED semantics as the core gate. Skipped when the core
	// path already captured a snapshot this boot.
	pendingPlugins, pendErr := database.PendingPluginMigrations(db, pluginSchemas)
	if pendErr != nil {
		// If the tracking state cannot be read, the safe assumption is that
		// something is pending: fail toward taking a backup, never away.
		slog.Warn("could not determine pending plugin migrations; assuming some are pending",
			slog.Any("error", pendErr))
		pendingPlugins = []string{"(undetermined)"}
	}
	if len(pendingPlugins) > 0 && !coreBackedUp {
		slog.Info("pending plugin migration(s) detected — backing up before applying",
			slog.String("plugins", strings.Join(pendingPlugins, ", ")))
		if backupErr := database.PreMigrationBackup(db, backupCfg); backupErr != nil {
			if backupRequired {
				fatalBoot("pre-plugin-migration backup failed and BACKUP_REQUIRED=1; refusing to apply plugin migrations", backupErr)
			}
			slog.Warn("pre-plugin-migration backup failed (non-fatal in default mode); plugin migrations will still apply",
				slog.Any("error", backupErr))
		}
	}

	pluginResults := database.RunPluginMigrations(db, pluginSchemas)
	for _, r := range pluginResults {
		pluginHealth.Register(r.Slug, r.Healthy, r.Error, r.Version, r.LatestVersion)
	}
	if degraded := pluginHealth.DegradedPlugins(); len(degraded) > 0 {
		slog.Warn("some plugins are degraded — features disabled",
			slog.Any("plugins", degraded),
		)
	}

	// Repair any stored icon name that fails the shared icon check. Runs after
	// every migration so all icon tables exist, and before serving so no page
	// is built from an old bad value. Idempotent; a failure is logged and
	// boot continues, since every page also escapes icons.
	if n, err := database.ReconcileIconColumns(context.Background(), db, app.IconColumns()); err != nil {
		slog.Error("icon reconcile failed", slog.Any("error", err), slog.Int("rows_fixed", n))
	} else if n > 0 {
		slog.Info("icon reconcile: repaired stored icons", slog.Int("rows", n))
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
	application, err := app.New(cfg, db, rdb, pluginHealth, pluginSchemas)
	if err != nil {
		slog.Error("failed to create application", slog.Any("error", err))
		os.Exit(1)
	}

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

		// Signal long-running background jobs (e.g. the media content-hash
		// backfill, #711) to stop instead of continuing to work against a
		// closing DB connection.
		application.ShutdownCancel()

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

// getEnvDefault reads an environment variable or returns the fallback.
func getEnvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// fatalBoot logs an unrecoverable boot error and exits — but first sleeps a
// backoff (BOOT_FAIL_BACKOFF, default 45s) so a `restart: unless-stopped`
// container retries at ~1/min instead of hot-looping, which floods logs and
// disk. Use ONLY for errors that won't fix themselves on a fast retry (bad
// migration/schema state, failed health checks, misconfiguration). Transient
// dependency waits (e.g. DB not ready yet) keep exiting fast so the
// orchestrator can retry quickly.
func fatalBoot(msg string, err error) {
	slog.Error(msg, slog.Any("error", err))
	backoff := 45 * time.Second
	if raw := getEnvDefault("BOOT_FAIL_BACKOFF", ""); raw != "" {
		if d, perr := time.ParseDuration(raw); perr == nil {
			backoff = d
		} else {
			slog.Warn("invalid BOOT_FAIL_BACKOFF; using default",
				slog.String("value", raw), slog.Duration("default", backoff))
		}
	}
	if backoff > 0 {
		slog.Error("unrecoverable boot error — sleeping before exit to avoid a restart hot-loop",
			slog.Duration("backoff", backoff),
			slog.String("hint", "fix the underlying issue or roll back the image; see docs/deployment.md"),
		)
		time.Sleep(backoff)
	}
	os.Exit(1)
}

// registeredPlugins returns the built-in plugins with their embedded
// migration filesystems (via Go's embed package, so they're available
// regardless of working directory).
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
		// widgetbindings: the generic host↔widget-type↔instance binding table.
		// FK-free and polymorphic, so plugin order vs calendar/maps/timeline
		// doesn't matter.
		{Slug: "widgetbindings", MigrationsFS: mustSub(widgetbindings.MigrationsFS, database.PluginMigrationsSubdir)},
		// systemstate: per-page game-system state. Its foreign keys point at
		// core tables only, which have migrated before any plugin runs.
		{Slug: systemstate.PluginSlug, MigrationsFS: mustSub(systemstate.MigrationsFS, database.PluginMigrationsSubdir)},
		// rolltables: one per-campaign document; its only foreign key points at
		// the core campaigns table.
		{Slug: rolltables.PluginSlug, MigrationsFS: mustSub(rolltables.MigrationsFS, database.PluginMigrationsSubdir)},
		// armory: its own tables beyond the core ones (item shares). Foreign
		// keys point at core campaigns and entities only.
		{Slug: armory.AddonSlug, MigrationsFS: mustSub(armory.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "syncapi", MigrationsFS: mustSub(syncapi.MigrationsFS, database.PluginMigrationsSubdir)},
		{Slug: "packages", MigrationsFS: mustSub(packages.MigrationsFS, database.PluginMigrationsSubdir)},
		// foundry_vtt's migration 001 renames foundry_module_campaign_tokens
		// to foundry_vtt_campaign_tokens and drops the orphaned
		// foundry_module_versions table; foundry_vtt.PreMigrationCheck aborts
		// startup first if foundry_module_versions has rows, to avoid
		// destroying an in-flight manual upload.
		//
		// The Slug is taken from the plugin's own constant rather than a
		// literal because foundry_vtt.ReconcileConsolidationState writes a
		// plugin_schema_versions row under it: if the two ever drifted, the
		// reconciler's row would be invisible to the runner and the
		// fresh-install crash would come straight back.
		{Slug: foundry_vtt.PluginHealthKey, MigrationsFS: mustSub(foundry_vtt.MigrationsFS, database.PluginMigrationsSubdir)},
	}
}
