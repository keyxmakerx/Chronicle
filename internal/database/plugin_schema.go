// Package database provides connection setup for MariaDB and Redis.
// This file implements per-plugin schema migration with graceful degradation.
// Each built-in plugin owns its own numbered SQL migration files. Failures
// disable the plugin instead of crashing the app.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

	// MigrationsDir is the absolute or relative path to the plugin's
	// migrations/ directory containing numbered SQL files.
	MigrationsDir string
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
	return err
}

// runSinglePluginMigrations parses and applies pending migrations for one plugin.
func runSinglePluginMigrations(db *sql.DB, plugin PluginSchema) PluginMigrationResult {
	ctx := context.Background()

	// Check if the migrations directory exists.
	if _, err := os.Stat(plugin.MigrationsDir); err != nil {
		if os.IsNotExist(err) {
			// No migrations directory = nothing to do, plugin is healthy.
			return PluginMigrationResult{Slug: plugin.Slug, Healthy: true, Version: 0}
		}
		return PluginMigrationResult{
			Slug:  plugin.Slug,
			Error: fmt.Errorf("checking migrations dir: %w", err),
		}
	}

	// Parse migration files from disk.
	migrations, err := parsePluginMigrations(plugin.MigrationsDir)
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

// parsePluginMigrations reads numbered SQL migration files from a directory.
// Returns migrations sorted by version number ascending.
func parsePluginMigrations(dir string) ([]pluginMigration, error) {
	entries, err := os.ReadDir(dir)
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

		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		switch direction {
		case "up":
			pairs[version].up = string(content)
		case "down":
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
func execPluginMigration(ctx context.Context, db *sql.DB, slug string, m pluginMigration) error {
	stmts := splitPluginStatements(m.UpSQL)
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration %d failed on statement: %w\nSQL: %.200s", m.Version, err, stmt)
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
// in a plugin's migrations directory. Returns 0 if no migrations exist.
func LatestMigrationVersion(migrationsDir string) (int, error) {
	if _, err := os.Stat(migrationsDir); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	entries, err := os.ReadDir(migrationsDir)
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

// RegisteredPlugins returns the list of built-in plugins that have schema
// migrations. The MigrationsDir paths are relative to the working directory.
func RegisteredPlugins() []PluginSchema {
	return []PluginSchema{
		{Slug: "calendar", MigrationsDir: "internal/plugins/calendar/migrations"},
		{Slug: "maps", MigrationsDir: "internal/plugins/maps/migrations"},
		{Slug: "sessions", MigrationsDir: "internal/plugins/sessions/migrations"},
		{Slug: "timeline", MigrationsDir: "internal/plugins/timeline/migrations"},
		{Slug: "syncapi", MigrationsDir: "internal/plugins/syncapi/migrations"},
	}
}
