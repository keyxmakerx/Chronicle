// Package database provides connection setup for MariaDB and Redis.
//
// plugin_migration_safety.go is the damage-control layer around
// execPluginMigration. It exists because of a specific, reproduced failure
// mode (C-SWEEP-R4 / data/plugin-migration-no-transaction):
//
//	A plugin migration with more than one statement is applied statement by
//	statement on a plain *sql.DB, and its version row is written only after
//	the LAST one succeeds. So a migration that fails on its second statement
//	leaves the first statement's effect in the database and NO record that
//	anything happened. The next boot re-runs the migration from statement one
//	— which now fails with "table already exists" / "duplicate column name",
//	because most plugin ALTERs are not idempotent. The plugin is degraded
//	forever and every subsequent boot repeats the same failure.
//
// WHAT THIS IS NOT. It is not a transaction and does not pretend to be one.
// MariaDB does not support transactional DDL: every CREATE / ALTER / DROP /
// RENAME commits implicitly and cannot be rolled back. Nothing in this file
// undoes a statement that already ran, and the error messages say so.
//
// WHAT IT ACTUALLY BUYS, precisely:
//
//  1. A PRE-FLIGHT APPLICABILITY CHECK. Before the first statement of a
//     migration executes, every statement is checked against the schema
//     catalogue as it will be at that point in the migration. If any statement
//     cannot possibly succeed, the migration aborts having executed NOTHING.
//     That converts the common "half-applied, unrecoverable" outcome into
//     "nothing applied, actionable error" — which is the closest thing to
//     atomicity available without transactional DDL. It is a table-granularity
//     check: it catches the failures that produce unrecoverable states
//     (missing/duplicate/renamed tables), not every possible SQL error.
//
//  2. PARTIAL-PROGRESS RECORDING. When a statement fails anyway — a pre-flight
//     at table granularity cannot foresee an FK violation, a duplicate column,
//     a full disk — the number of statements that DID succeed is recorded
//     first, keyed by a hash of the migration's SQL. The next boot resumes
//     after them instead of replaying them, so a non-idempotent ALTER that
//     already applied is never attempted twice. The crash-loop becomes forward
//     progress or, at worst, the same honest failure at the same statement.
//
// The SQL hash is what makes resuming safe: a recorded offset is only honoured
// if the migration's text is byte-identical to the text that produced it.
// Migrations are append-only and immutable (ADR-044/045) so this should never
// diverge, but a resume that skipped the wrong statements would be far worse
// than the crash-loop it replaces, so it is checked rather than assumed.
package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// --- Statement classification -------------------------------------------
//
// The pre-flight understands the four table-level DDL verbs whose failure
// leaves a migration unrecoverable. Anything it does not recognise is passed
// through untouched: the check may only ever ADD a reason to refuse, never
// skip a statement or soften an existing failure.

var (
	// `IF NOT EXISTS` / `IF EXISTS` are optional and captured so the check can
	// tell a self-guarding statement (always applicable) from a bare one.
	reCreateTable = regexp.MustCompile(`(?is)^CREATE\s+(?:TEMPORARY\s+)?TABLE\s+(IF\s+NOT\s+EXISTS\s+)?` + identPattern)
	reDropTable   = regexp.MustCompile(`(?is)^DROP\s+(?:TEMPORARY\s+)?TABLE\s+(IF\s+EXISTS\s+)?` + identPattern)
	reRenameTable = regexp.MustCompile(`(?is)^RENAME\s+TABLE\s+` + identPattern + `\s+TO\s+` + identPattern)
	reAlterTable  = regexp.MustCompile(`(?is)^ALTER\s+(?:ONLINE\s+|IGNORE\s+)*TABLE\s+(IF\s+EXISTS\s+)?` + identPattern)
	reCreateIndex = regexp.MustCompile(`(?is)^CREATE\s+(?:UNIQUE\s+|FULLTEXT\s+|SPATIAL\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?\S+\s+ON\s+` + identPattern)
)

// identPattern matches a table identifier, optionally backquoted and
// optionally schema-qualified, capturing only the table name. Schema
// qualification is captured-and-discarded rather than rejected because a
// migration that names its own schema is legal, if unusual.
const identPattern = "(?:`?\\w+`?\\.)?`?(\\w+)`?"

// tableCatalogue is the set of table names the pre-flight believes exist. It
// starts as the real catalogue and is advanced statement by statement, so a
// migration that creates a table and then alters it validates correctly — a
// check against only the pre-migration catalogue would reject that, and
// falsely aborting a good migration is the one outcome worse than the bug.
type tableCatalogue map[string]bool

// checkStatementApplicable validates one statement against the simulated
// catalogue and advances it. A non-nil error means the statement cannot
// succeed and the migration must not start.
func checkStatementApplicable(cat tableCatalogue, stmt string) error {
	s := strings.TrimSpace(stmt)

	if m := reCreateTable.FindStringSubmatch(s); m != nil {
		guarded, name := m[1] != "", strings.ToLower(m[2])
		if !guarded && cat[name] {
			return fmt.Errorf("CREATE TABLE %s: the table already exists "+
				"(use CREATE TABLE IF NOT EXISTS — new plugin DDL must be idempotent)", name)
		}
		cat[name] = true
		return nil
	}

	if m := reDropTable.FindStringSubmatch(s); m != nil {
		guarded, name := m[1] != "", strings.ToLower(m[2])
		if !guarded && !cat[name] {
			return fmt.Errorf("DROP TABLE %s: the table does not exist "+
				"(use DROP TABLE IF EXISTS — new plugin DDL must be idempotent)", name)
		}
		delete(cat, name)
		return nil
	}

	if m := reRenameTable.FindStringSubmatch(s); m != nil {
		from, to := strings.ToLower(m[1]), strings.ToLower(m[2])
		if !cat[from] {
			return fmt.Errorf("RENAME TABLE %s TO %s: the source table does not exist. "+
				"This is an upgrade-path statement running against a database that was "+
				"never in the state it upgrades FROM", from, to)
		}
		if cat[to] {
			return fmt.Errorf("RENAME TABLE %s TO %s: the destination table already exists", from, to)
		}
		delete(cat, from)
		cat[to] = true
		return nil
	}

	if m := reAlterTable.FindStringSubmatch(s); m != nil {
		guarded, name := m[1] != "", strings.ToLower(m[2])
		if !guarded && !cat[name] {
			return fmt.Errorf("ALTER TABLE %s: the table does not exist", name)
		}
		return nil
	}

	if m := reCreateIndex.FindStringSubmatch(s); m != nil {
		if name := strings.ToLower(m[1]); !cat[name] {
			return fmt.Errorf("CREATE INDEX ... ON %s: the table does not exist", name)
		}
		return nil
	}

	// Unrecognised statement (INSERT, UPDATE, SET, a DDL form not modelled
	// here). Not classifiable at table granularity, so it is left alone and
	// allowed to run. The pre-flight narrows the failure surface; it does not
	// claim to cover it.
	return nil
}

// preflightPluginMigration reports whether every statement of a migration can
// apply, given the schema as it stands now. Returns nil if the migration may
// proceed.
//
// FAIL-OPEN on a metadata read failure, deliberately: this check is an extra
// safety net layered over behaviour that already worked, and refusing to run
// any plugin migration because information_schema hiccuped would be a new
// outage mode strictly worse than the one being fixed. The failure is logged
// so it is never silent.
func preflightPluginMigration(ctx context.Context, db *sql.DB, slug string, version int, stmts []string) error {
	cat, err := loadTableCatalogue(ctx, db)
	if err != nil {
		slog.Warn("plugin migration pre-flight skipped — could not read the schema catalogue",
			slog.String("plugin", slug),
			slog.Int("version", version),
			slog.Any("error", err),
		)
		return nil
	}

	for i, stmt := range stmts {
		if err := checkStatementApplicable(cat, stmt); err != nil {
			return fmt.Errorf(
				"migration %d cannot be applied: statement %d of %d %w.\n"+
					"NOTHING was executed — the pre-flight check refused before the first "+
					"statement ran, so this database is exactly as it was.\n"+
					"SQL: %.200s",
				version, i+1, len(stmts), err, stmt)
		}
	}
	return nil
}

// loadTableCatalogue reads the current schema's table names, lower-cased so
// the simulation matches MariaDB's default case-insensitive table handling on
// the platforms Chronicle ships on.
func loadTableCatalogue(ctx context.Context, db *sql.DB) (tableCatalogue, error) {
	if db == nil {
		// Not reachable from the runner, which always holds a live handle.
		// Guarded so the fail-open path is exercisable without a database —
		// a panic here would turn the safety net into a new crash.
		return nil, fmt.Errorf("loadTableCatalogue: nil db handle")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT LOWER(table_name) FROM information_schema.tables
		 WHERE table_schema = DATABASE()
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cat := make(tableCatalogue)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cat[name] = true
	}
	return cat, rows.Err()
}

// --- Partial-progress recording -----------------------------------------

// ensurePluginMigrationProgressTable creates the resume-offset table.
// Idempotent; called from ensurePluginSchemaTable so there is exactly one
// place that has to be reached before either table is used.
func ensurePluginMigrationProgressTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_migration_progress (
			plugin_slug        VARCHAR(100) NOT NULL,
			version            INT          NOT NULL,
			statements_applied INT          NOT NULL DEFAULT 0,
			sql_sha256         CHAR(64)     NOT NULL,
			updated_at         TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
			                                ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (plugin_slug, version)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	return err
}

// migrationSQLSum is the identity a resume offset is bound to. Two different
// migration texts must never share one, or a resume would skip the wrong
// statements.
func migrationSQLSum(upSQL string) string {
	sum := sha256.Sum256([]byte(upSQL))
	return hex.EncodeToString(sum[:])
}

// resumeOffset returns how many leading statements of this migration are
// already known to have been applied by an earlier, failed attempt.
//
// Returns 0 — start from the beginning — whenever the answer is not certain:
// no recorded progress, a recorded hash that does not match the migration text
// in this binary, or any read failure. Replaying from zero is the pre-existing
// behaviour, so an uncertain answer degrades to exactly what happened before
// this mechanism existed and never to something new.
func resumeOffset(ctx context.Context, db *sql.DB, slug string, version int, sum string, total int) int {
	var applied int
	var storedSum string
	err := db.QueryRowContext(ctx, `
		SELECT statements_applied, sql_sha256 FROM plugin_migration_progress
		 WHERE plugin_slug = ? AND version = ?
	`, slug, version).Scan(&applied, &storedSum)
	switch {
	case err == sql.ErrNoRows:
		return 0
	case err != nil:
		slog.Warn("could not read plugin migration progress; replaying from the first statement",
			slog.String("plugin", slug), slog.Int("version", version), slog.Any("error", err))
		return 0
	}

	if storedSum != sum {
		slog.Warn("recorded plugin migration progress does not match this binary's migration "+
			"SQL; ignoring it and replaying from the first statement",
			slog.String("plugin", slug), slog.Int("version", version))
		return 0
	}
	if applied <= 0 || applied >= total {
		return 0
	}

	slog.Info("resuming a plugin migration that failed part-way through",
		slog.String("plugin", slug),
		slog.Int("version", version),
		slog.Int("statements_already_applied", applied),
		slog.Int("statements_total", total),
	)
	return applied
}

// recordProgress stores how many statements of a failing migration did apply,
// so the next boot resumes after them. Best-effort: if this write fails the
// caller's original migration error is what matters, and the next boot simply
// replays from the start exactly as it did before this mechanism existed.
func recordProgress(ctx context.Context, db *sql.DB, slug string, version, applied int, sum string) {
	if applied <= 0 {
		return
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO plugin_migration_progress (plugin_slug, version, statements_applied, sql_sha256)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE statements_applied = VALUES(statements_applied),
		                        sql_sha256         = VALUES(sql_sha256)
	`, slug, version, applied, sum)
	if err != nil {
		slog.Error("could not record partial plugin migration progress; the next boot will "+
			"replay this migration from its first statement",
			slog.String("plugin", slug), slog.Int("version", version), slog.Any("error", err))
	}
}

// clearProgress drops the resume offset once a migration completes, so a
// later re-run of the same version (there should not be one, but the row must
// not outlive its meaning) cannot skip anything.
func clearProgress(ctx context.Context, db *sql.DB, slug string, version int) {
	if _, err := db.ExecContext(ctx,
		`DELETE FROM plugin_migration_progress WHERE plugin_slug = ? AND version = ?`,
		slug, version,
	); err != nil {
		slog.Warn("could not clear plugin migration progress row",
			slog.String("plugin", slug), slog.Int("version", version), slog.Any("error", err))
	}
}
