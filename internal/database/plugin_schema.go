// Package database provides connection setup for MariaDB and Redis.
// This file implements per-plugin schema migration with graceful degradation.
// Each built-in plugin owns its own numbered SQL migration files. Failures
// disable the plugin instead of crashing the app.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// PluginSchema describes a built-in plugin's migration configuration.
// Each plugin registers one of these at startup.
type PluginSchema struct {
	// Slug is the unique plugin identifier (e.g., "calendar", "maps").
	Slug string

	// MigrationsFS is an embedded filesystem containing the plugin's
	// numbered SQL migration files. Using embed.FS ensures migrations
	// are available regardless of the binary's working directory.
	MigrationsFS fs.FS
}

// PluginMigrationResult holds the outcome of running a single plugin's
// schema migrations.
type PluginMigrationResult struct {
	Slug          string
	Healthy       bool
	Error         error
	Version       int
	LatestVersion int
}

// pluginMigration represents a single numbered migration for a plugin.
type pluginMigration struct {
	Version int
	UpSQL   string
	DownSQL string
}

// pluginMigrationFileRe matches files like "001_create_tables.up.sql".
var pluginMigrationFileRe = regexp.MustCompile(`^(\d+)_.*\.(up|down)\.sql$`)

// RunPluginMigrations runs schema migrations for all registered plugins.
// Each plugin runs independently — a failure disables that plugin but
// does not affect others or prevent the app from starting.
func RunPluginMigrations(db *sql.DB, plugins []PluginSchema) []PluginMigrationResult {
	results := make([]PluginMigrationResult, 0, len(plugins))

	// Ensure the tracking table exists. This is idempotent.
	if err := ensurePluginSchemaTable(db); err != nil {
		slog.Error("cannot create plugin_schema_versions table",
			slog.Any("error", err),
		)
		// If we can't even create the tracking table, mark all plugins unhealthy.
		for _, p := range plugins {
			results = append(results, PluginMigrationResult{
				Slug:    p.Slug,
				Healthy: false,
				Error:   fmt.Errorf("plugin schema tracking unavailable: %w", err),
			})
		}
		return results
	}

	for _, p := range plugins {
		result := runSinglePluginMigrations(db, p)
		results = append(results, result)

		if result.Healthy {
			slog.Info("plugin schema ready",
				slog.String("plugin", p.Slug),
				slog.Int("version", result.Version),
			)
		} else {
			slog.Error("plugin schema failed — feature will be disabled",
				slog.String("plugin", p.Slug),
				slog.Any("error", result.Error),
			)
		}
	}

	return results
}

// MarkPluginMigrationApplied records `version` as already applied for `slug`
// WITHOUT running its SQL, so the migration runner skips straight past it.
//
// This exists for exactly one situation: a migration whose SQL can never
// succeed against the database in front of it, because it encodes an upgrade
// step from a predecessor state this database was never in. `foundry_vtt`'s
// migration 001 is the archetype — it RENAMEs a table the deleted
// `foundry_modules` plugin used to create, so on a fresh install its first
// statement fails and (since the runner returns on the first failure) every
// later migration for that plugin is unreachable forever.
//
// Marking a migration applied is a DATA fix to the tracking table, not a
// schema change, which is why it lives in a Go reconciler rather than in a
// migration (see CLAUDE.md → "Migration Safety Rules"). The caller owns the
// judgement that the migration is genuinely inapplicable; a later idempotent
// migration must still establish the schema the skipped one would have
// produced, or the plugin ends up "healthy" with a missing table.
//
// Idempotent: INSERT IGNORE, and the tracking table is ensured first because
// reconcilers run BEFORE RunPluginMigrations creates it.
func MarkPluginMigrationApplied(ctx context.Context, db *sql.DB, slug string, version int) error {
	if db == nil {
		return fmt.Errorf("MarkPluginMigrationApplied(%s, %d): nil db handle", slug, version)
	}
	if err := ensurePluginSchemaTable(db); err != nil {
		return fmt.Errorf("ensuring plugin_schema_versions: %w", err)
	}
	_, err := db.ExecContext(ctx,
		`INSERT IGNORE INTO plugin_schema_versions (plugin_slug, version) VALUES (?, ?)`,
		slug, version,
	)
	if err != nil {
		return fmt.Errorf("recording %s migration %d as applied: %w", slug, version, err)
	}
	return nil
}

// ensurePluginSchemaTable creates the plugin_schema_versions table if it
// doesn't exist. Separate from extension_schema_versions to keep built-in
// plugin state distinct from user-installed extensions.
func ensurePluginSchemaTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_schema_versions (
			plugin_slug VARCHAR(100) NOT NULL,
			version     INT          NOT NULL,
			applied_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (plugin_slug, version)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
	`)
	if err != nil {
		return err
	}
	// The resume-offset table is ensured from the same place, so there is
	// exactly one call site that has to be reached before either is used and
	// they cannot drift apart. See plugin_migration_safety.go.
	return ensurePluginMigrationProgressTable(db)
}

// runSinglePluginMigrations parses and applies pending migrations for one plugin.
func runSinglePluginMigrations(db *sql.DB, plugin PluginSchema) PluginMigrationResult {
	ctx := context.Background()

	if plugin.MigrationsFS == nil {
		// No embedded migrations = nothing to do, plugin is healthy.
		return PluginMigrationResult{Slug: plugin.Slug, Healthy: true, Version: 0}
	}

	// Parse migration files from embedded filesystem.
	migrations, err := parsePluginMigrations(plugin.MigrationsFS)
	if err != nil {
		return PluginMigrationResult{
			Slug:  plugin.Slug,
			Error: fmt.Errorf("parsing migrations: %w", err),
		}
	}

	if len(migrations) == 0 {
		return PluginMigrationResult{Slug: plugin.Slug, Healthy: true, Version: 0, LatestVersion: 0}
	}

	// Compute the highest available migration version from files.
	latestVersion := migrations[len(migrations)-1].Version

	// Get already-applied versions.
	applied, err := getPluginAppliedVersions(ctx, db, plugin.Slug)
	if err != nil {
		return PluginMigrationResult{
			Slug:          plugin.Slug,
			LatestVersion: latestVersion,
			Error:         fmt.Errorf("reading applied versions: %w", err),
		}
	}
	appliedSet := make(map[int]bool, len(applied))
	for _, v := range applied {
		appliedSet[v] = true
	}

	// Determine highest applied version for the result.
	highestVersion := 0
	if len(applied) > 0 {
		highestVersion = applied[len(applied)-1]
	}

	// Apply pending migrations in order.
	for _, m := range migrations {
		if appliedSet[m.Version] {
			if m.Version > highestVersion {
				highestVersion = m.Version
			}
			continue
		}

		// Execute each statement individually. Built-in plugins are trusted
		// code, so we skip the SQL validator (unlike user extensions).
		if err := execPluginMigration(ctx, db, plugin.Slug, m); err != nil {
			return PluginMigrationResult{
				Slug:          plugin.Slug,
				Error:         err,
				Version:       highestVersion,
				LatestVersion: latestVersion,
			}
		}

		highestVersion = m.Version
		slog.Info("applied plugin migration",
			slog.String("plugin", plugin.Slug),
			slog.Int("version", m.Version),
		)
	}

	return PluginMigrationResult{
		Slug:          plugin.Slug,
		Healthy:       true,
		Version:       highestVersion,
		LatestVersion: latestVersion,
	}
}

// parsePluginMigrations reads numbered SQL migration files from an embedded filesystem.
// Returns migrations sorted by version number ascending.
func parsePluginMigrations(migrationsFS fs.FS) ([]pluginMigration, error) {
	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return nil, fmt.Errorf("reading migrations dir: %w", err)
	}

	type migPair struct {
		up   string
		down string
	}
	pairs := make(map[int]*migPair)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := pluginMigrationFileRe.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		direction := matches[2]

		if pairs[version] == nil {
			pairs[version] = &migPair{}
		}

		content, err := fs.ReadFile(migrationsFS, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		switch direction {
		case "up":
			// Guard against two .up.sql files with the same version number:
			// the old behaviour silently overwrote, so whichever file was
			// read last won and the other's SQL was never executed. Error
			// loudly instead so the collision is caught at startup (plugin
			// degrades) rather than silently losing a migration.
			if pairs[version].up != "" {
				return nil, fmt.Errorf("duplicate plugin migration version %d: two .up.sql files found", version)
			}
			pairs[version].up = string(content)
		case "down":
			// Same guard for .down.sql: two files with the same version is
			// always a numbering mistake that must be fixed in source.
			if pairs[version].down != "" {
				return nil, fmt.Errorf("duplicate plugin migration version %d: two .down.sql files found", version)
			}
			pairs[version].down = string(content)
		}
	}

	var versions []int
	for v := range pairs {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	var migrations []pluginMigration
	for _, v := range versions {
		p := pairs[v]
		if p.up == "" {
			return nil, fmt.Errorf("migration version %d has no .up.sql file", v)
		}
		migrations = append(migrations, pluginMigration{
			Version: v,
			UpSQL:   p.up,
			DownSQL: p.down,
		})
	}

	return migrations, nil
}

// execPluginMigration executes a single migration's SQL and records it
// in plugin_schema_versions. Statements are split by semicolons and
// executed individually.
// The migration is NOT atomic and cannot be — MariaDB commits every DDL
// statement implicitly. Two mechanisms make a partial failure survivable
// instead of terminal (C-SWEEP-R4 / data/plugin-migration-no-transaction); see
// plugin_migration_safety.go for exactly what they do and do not promise:
//
//   - a pre-flight applicability check, so a migration that cannot complete
//     aborts having executed nothing at all, and
//   - a recorded resume offset, so a statement that DID apply before the
//     failure is not replayed on the next boot into "table already exists".
func execPluginMigration(ctx context.Context, db *sql.DB, slug string, m pluginMigration) error {
	stmts := splitPluginStatements(m.UpSQL)
	sum := migrationSQLSum(m.UpSQL)

	// Statements before this index applied during an earlier failed attempt.
	// Re-running them is what turned a one-statement failure into a permanent
	// crash-loop, because most plugin ALTERs are not idempotent.
	start := resumeOffset(ctx, db, slug, m.Version, sum, len(stmts))

	if err := preflightPluginMigration(ctx, db, slug, m.Version, stmts[start:]); err != nil {
		return err
	}

	for i := start; i < len(stmts); i++ {
		if _, err := db.ExecContext(ctx, stmts[i]); err != nil {
			// Record what really did apply BEFORE returning, so the next boot
			// resumes here rather than replaying. Best-effort by design: the
			// migration error below is the one the operator must see.
			recordProgress(ctx, db, slug, m.Version, i, sum)
			return fmt.Errorf(
				"migration %d failed on statement %d of %d: %w\n"+
					"The %d statement(s) before it DID apply and have NOT been rolled back — "+
					"MariaDB commits DDL implicitly, so no rollback is possible. That progress "+
					"is recorded: the next start resumes at statement %d instead of replaying "+
					"the ones that succeeded.\nSQL: %.200s",
				m.Version, i+1, len(stmts), err, i, i+1, stmts[i])
		}
	}

	// Record the applied version.
	_, err := db.ExecContext(ctx,
		`INSERT INTO plugin_schema_versions (plugin_slug, version) VALUES (?, ?)`,
		slug, m.Version,
	)
	if err != nil {
		return fmt.Errorf("recording migration %d: %w", m.Version, err)
	}

	// The resume offset has served its purpose and must not outlive it.
	clearProgress(ctx, db, slug, m.Version)

	return nil
}

// getPluginAppliedVersions returns version numbers already applied for a plugin.
func getPluginAppliedVersions(ctx context.Context, db *sql.DB, slug string) ([]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT version FROM plugin_schema_versions WHERE plugin_slug = ? ORDER BY version`,
		slug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// splitPluginStatements splits SQL content by semicolons, stripping
// single-line comments. Returns non-empty trimmed statements.
func splitPluginStatements(sql string) []string {
	lines := strings.Split(sql, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	joined := strings.Join(cleaned, "\n")

	parts := strings.Split(joined, ";")
	var stmts []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}

// LatestMigrationVersion returns the highest migration version number found
// in a plugin's embedded migrations filesystem. Returns 0 if nil or empty.
func LatestMigrationVersion(migrationsFS fs.FS) (int, error) {
	if migrationsFS == nil {
		return 0, nil
	}

	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return 0, err
	}

	highest := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := pluginMigrationFileRe.FindStringSubmatch(entry.Name())
		if len(matches) < 2 {
			continue
		}
		v, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		if v > highest {
			highest = v
		}
	}

	return highest, nil
}

// PluginMigrationsSubdir is the subdirectory within each plugin's embed.FS
// that contains migration SQL files (from //go:embed migrations/*.sql).
const PluginMigrationsSubdir = "migrations"
