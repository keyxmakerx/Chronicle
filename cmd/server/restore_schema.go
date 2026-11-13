package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/app"
	"github.com/keyxmakerx/chronicle/internal/config"
	"github.com/keyxmakerx/chronicle/internal/database"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

// postRestoreSchema returns the step the admin restore page runs after the
// script has loaded a backup: the same schema work a normal boot does, because
// the backup may predate the running image. It mirrors main's sequence
// (core migrations, startup checks, plugin migrations, repair passes) but
// returns an error instead of exiting, so a failure keeps the site closed and
// is shown to the admin.
func postRestoreSchema(cfg *config.Config, db *sql.DB, health *database.PluginHealthRegistry, plugins []database.PluginSchema) func(context.Context) error {
	return func(ctx context.Context) error {
		backupCfg, _ := bootBackupConfig(cfg)

		if _, err := database.MigrateWithBackup(db, cfg.Database.DSN(), "db/migrations", backupCfg); err != nil {
			return fmt.Errorf("core migrations: %w", err)
		}
		if err := database.RunStartupHealthChecks(db, app.StartupHealthCheckConfig(cfg)); err != nil {
			return fmt.Errorf("startup checks: %w", err)
		}
		if err := foundry_vtt.PreMigrationCheck(ctx, db); err != nil {
			return fmt.Errorf("foundry pre-migration check: %w", err)
		}
		if err := foundry_vtt.ReconcileConsolidationState(ctx, db); err != nil {
			return fmt.Errorf("foundry consolidation state: %w", err)
		}
		for _, r := range database.RunPluginMigrations(db, plugins) {
			health.Register(r.Slug, r.Healthy, r.Error, r.Version, r.LatestVersion)
		}
		if n, err := database.ReconcileIconColumns(ctx, db, app.IconColumns()); err != nil {
			slog.Error("restore: icon reconcile failed", slog.Any("error", err), slog.Int("rows_fixed", n))
		}
		if n, err := database.ReconcilePublicTextColumns(ctx, db, app.PublicTextColumns()); err != nil {
			slog.Error("restore: public text reconcile failed", slog.Any("error", err), slog.Int("rows_fixed", n))
		}
		return nil
	}
}
