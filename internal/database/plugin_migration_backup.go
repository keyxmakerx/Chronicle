package database

import (
	"context"
	"database/sql"
	"fmt"
)

// plugin_migration_backup.go closes the backup gap the CALV5 wipe exposed.
//
// MigrateWithBackup backs up ONLY when a CORE migration is pending. Plugin
// migrations run afterwards (RunPluginMigrations) with no backup hook of any
// kind — so a release that ships destructive PLUGIN migrations and zero core
// migrations used to reach production with no automatic pre-wipe backup at
// all, while the wipe's own down files told the operator that recovery is
// "restoring the database backup taken before the wipe". The CALV5 branch
// (calendar 019 + sessions 006 + timeline 002) is exactly that shape.
//
// PendingPluginMigrations is the detection half; main.go pairs it with
// PreMigrationBackup under the same BACKUP_REQUIRED semantics as the core
// gate, skipping the extra backup when the core path already took one this
// boot.

// PendingPluginMigrations reports which of the given plugins have at least one
// migration file whose version is not recorded as applied in
// plugin_schema_versions. It creates the tracking table if it does not exist
// (idempotent, same DDL the runner uses), so it is safe to call on a fresh
// database — where it reports every plugin with migrations as pending, which
// is the truth.
//
// It deliberately mirrors runSinglePluginMigrations' accounting (parse the
// embedded FS, diff against applied versions) rather than keeping its own: a
// second notion of "pending" that could disagree with the runner's would make
// the backup gate lie in exactly the situations it exists for.
func PendingPluginMigrations(db *sql.DB, plugins []PluginSchema) ([]string, error) {
	if err := ensurePluginSchemaTable(db); err != nil {
		return nil, fmt.Errorf("ensuring plugin_schema_versions: %w", err)
	}
	ctx := context.Background()

	var pending []string
	for _, p := range plugins {
		if p.MigrationsFS == nil {
			continue
		}
		migrations, err := parsePluginMigrations(p.MigrationsFS)
		if err != nil {
			return nil, fmt.Errorf("plugin %s: parsing migrations: %w", p.Slug, err)
		}
		if len(migrations) == 0 {
			continue
		}
		applied, err := getPluginAppliedVersions(ctx, db, p.Slug)
		if err != nil {
			return nil, fmt.Errorf("plugin %s: reading applied versions: %w", p.Slug, err)
		}
		appliedSet := make(map[int]bool, len(applied))
		for _, v := range applied {
			appliedSet[v] = true
		}
		for _, m := range migrations {
			if !appliedSet[m.Version] {
				pending = append(pending, p.Slug)
				break
			}
		}
	}
	return pending, nil
}
