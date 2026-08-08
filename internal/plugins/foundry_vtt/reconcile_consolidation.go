package foundry_vtt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// consolidationMigrationVersion is the version number of
// 001_consolidate_foundry_modules — the upgrade-path migration this reconciler
// decides is applicable or not.
const consolidationMigrationVersion = 1

// predecessorTokenTable is the table `foundry_modules` (deleted in C-FMC-5c)
// created and migration 001 renames. Its presence is the ONLY evidence that
// this database was ever in the pre-consolidation state.
const predecessorTokenTable = "foundry_module_campaign_tokens"

// migrationSlug is the key this plugin's rows carry in
// plugin_schema_versions. It is PluginHealthKey ("foundry_vtt", underscore),
// NOT PluginSlug ("foundry-vtt", hyphen): cmd/server/main.go registers the
// schema under the underscore form, and a row written under the hyphen form
// would be invisible to the runner — the reconciler would appear to work and
// migration 001 would still be attempted and still fail. TestRegisteredPlugins_
// FoundryVTTSlugMatchesReconciler pins the two together.
const migrationSlug = PluginHealthKey

// ReconcileConsolidationState makes the foundry_vtt migration chain reachable
// on every database, not just the ones that came through `foundry_modules`.
//
// THE BUG IT CLOSES (C-SWEEP-R4 / data/fvtt-fresh-db-rename). Migration 001 is
// a consolidation migration: its first statement is
//
//	RENAME TABLE foundry_module_campaign_tokens TO foundry_vtt_campaign_tokens;
//
// On a brand-new self-hosted install that source table has never existed — the
// plugin that created it was deleted before the install ever ran — so the
// statement fails with Error 1146, the plugin is marked DEGRADED, and the
// Foundry integration is permanently dead. It cannot self-heal: the runner
// returns on the first failed migration, so no later migration for this plugin
// is ever reached. Nothing in the suite noticed because there was no fresh-DB
// replay anywhere in CI (see cmd/server/freshdb_migration_test.go).
//
// It is NOT closed by PreMigrationCheck, which sits immediately next to this
// call: that check only refuses when foundry_module_versions exists AND has
// rows. On a fresh database the table does not exist, so the check returns nil
// and the doomed migration runs anyway.
//
// THE RULE. Migration 001 is applicable if and only if its source table is
// present. When it is absent the RENAME can never succeed, so 001 is recorded
// as applied without running, and migration 002 — idempotent DDL — establishes
// the post-consolidation shape instead. Three histories converge on one schema:
//
//	fresh install            source absent  → 001 skipped, 002 creates the table
//	completed upgrade        source absent  → 001 already recorded, 002 no-ops
//	upgrade crashed mid-001  source absent  → 001 skipped, 002 finishes the job
//	pre-consolidation DB     source PRESENT → untouched; 001 runs for real
//
// The last row is what keeps this safe. The reconciler never skips a migration
// that has real work to do: as long as the predecessor table exists, 001 runs
// exactly as before and this function does nothing at all.
//
// Called from cmd/server/main.go alongside PreMigrationCheck, before
// database.RunPluginMigrations. Idempotent and cheap — one information_schema
// lookup, and at most one INSERT IGNORE.
func ReconcileConsolidationState(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("foundry_vtt.ReconcileConsolidationState: nil db handle")
	}
	return reconcileConsolidationState(ctx, sqlDBReconciler{db})
}

// consolidationReconciler is the narrow contract the internal reconcile uses.
// Same shape and same reason as preMigrationChecker in premigration.go: two
// methods, one per query the decision needs, so a test can drive every branch
// (including both failure paths) with canned values and no database.
type consolidationReconciler interface {
	// predecessorTableExists reports whether foundry_module_campaign_tokens —
	// migration 001's RENAME source — is present in the current schema.
	predecessorTableExists(ctx context.Context) (bool, error)
	// markConsolidationApplied records migration 001 as applied without
	// running it. Only called when predecessorTableExists returned false.
	markConsolidationApplied(ctx context.Context) error
}

// sqlDBReconciler is the production implementation backed by a *sql.DB.
type sqlDBReconciler struct{ db *sql.DB }

func (r sqlDBReconciler) predecessorTableExists(ctx context.Context) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		 WHERE table_schema = DATABASE()
		   AND table_name = ?
	`, predecessorTokenTable).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r sqlDBReconciler) markConsolidationApplied(ctx context.Context) error {
	return database.MarkPluginMigrationApplied(
		ctx, r.db, migrationSlug, consolidationMigrationVersion)
}

// reconcileConsolidationState is the testable core. See
// ReconcileConsolidationState for the rule it implements and why.
func reconcileConsolidationState(ctx context.Context, r consolidationReconciler) error {
	exists, err := r.predecessorTableExists(ctx)
	if err != nil {
		return fmt.Errorf(
			"foundry_vtt.ReconcileConsolidationState: query information_schema.tables: %w. "+
				"The reconciler needs schema metadata to know whether migration 001's RENAME "+
				"has a source table to rename. DB connectivity issue or missing privileges. "+
				"Verify the chronicle DB user has SELECT on information_schema.tables and retry",
			err)
	}

	if exists {
		// The pre-consolidation state is really here. 001 has work to do —
		// a real table to rename, with real token rows in it. Leave the
		// runner alone; skipping here would strand those rows under the old
		// name and leave the repository querying a table that never appears.
		return nil
	}

	if err := r.markConsolidationApplied(ctx); err != nil {
		return fmt.Errorf(
			"foundry_vtt.ReconcileConsolidationState: %w. "+
				"Without this record the foundry_vtt migration chain stops at 001, whose "+
				"RENAME cannot succeed on this database, and the Foundry integration stays "+
				"disabled. Verify the chronicle DB user can write plugin_schema_versions",
			err)
	}

	slog.Info("foundry_vtt: consolidation migration 001 is inapplicable on this database "+
		"(no predecessor token table); recorded as applied so migration 002 can establish "+
		"the post-consolidation schema",
		slog.String("plugin", migrationSlug),
		slog.Int("version", consolidationMigrationVersion),
	)
	return nil
}
