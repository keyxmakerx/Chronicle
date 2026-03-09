package extensions

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

// ExtensionMigration represents a single numbered migration for an extension.
type ExtensionMigration struct {
	Version int    // Sequential version number (1, 2, 3...).
	UpSQL   string // SQL to apply (create tables, add columns, etc.).
	DownSQL string // SQL to reverse (drop tables, remove columns, etc.).
}

// MigrationRunner manages per-extension schema migrations. It runs numbered
// SQL files from an extension's migrations/ directory, tracks applied versions
// in extension_schema_versions, and validates all SQL through the security
// validator to prevent modifications to core tables.
type MigrationRunner struct {
	db *sql.DB
}

// NewMigrationRunner creates a migration runner backed by the given database.
func NewMigrationRunner(db *sql.DB) *MigrationRunner {
	return &MigrationRunner{db: db}
}

// migrationFilePattern matches files like "001_create_tables.up.sql".
var migrationFilePattern = regexp.MustCompile(`^(\d+)_.*\.(up|down)\.sql$`)

// ParseMigrations reads numbered SQL migration files from a directory.
// Files must follow the pattern NNN_description.up.sql / NNN_description.down.sql.
// Returns migrations sorted by version number.
func ParseMigrations(dir string) ([]ExtensionMigration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading migration directory: %w", err)
	}

	// Group files by version number.
	type migPair struct {
		up   string
		down string
	}
	pairs := make(map[int]*migPair)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if len(matches) != 3 {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		direction := matches[2]

		if _, ok := pairs[version]; !ok {
			pairs[version] = &migPair{}
		}

		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading migration file %s: %w", entry.Name(), err)
		}

		switch direction {
		case "up":
			pairs[version].up = string(content)
		case "down":
			pairs[version].down = string(content)
		}
	}

	// Sort versions and build result.
	var versions []int
	for v := range pairs {
		versions = append(versions, v)
	}
	sort.Ints(versions)

	var migrations []ExtensionMigration
	for _, v := range versions {
		p := pairs[v]
		if p.up == "" {
			return nil, fmt.Errorf("migration version %d has no .up.sql file", v)
		}
		migrations = append(migrations, ExtensionMigration{
			Version: v,
			UpSQL:   p.up,
			DownSQL: p.down,
		})
	}

	return migrations, nil
}

// RunUp applies unapplied migrations in order. Each migration's SQL is validated
// against the extension slug before execution to prevent core table modifications.
func (r *MigrationRunner) RunUp(ctx context.Context, extensionID, slug string, migrations []ExtensionMigration) error {
	applied, err := r.GetAppliedVersions(ctx, extensionID)
	if err != nil {
		return fmt.Errorf("checking applied versions: %w", err)
	}
	appliedSet := make(map[int]bool, len(applied))
	for _, v := range applied {
		appliedSet[v] = true
	}

	for _, m := range migrations {
		if appliedSet[m.Version] {
			continue
		}

		// Validate SQL before execution.
		if err := ValidateExtensionSQL(slug, m.UpSQL); err != nil {
			return fmt.Errorf("migration %d validation failed: %w", m.Version, err)
		}

		// Execute migration statements.
		for _, stmt := range splitStatements(m.UpSQL) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := r.db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migration %d failed: %w", m.Version, err)
			}
		}

		// Record the applied version.
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO extension_schema_versions (extension_id, version) VALUES (?, ?)`,
			extensionID, m.Version,
		)
		if err != nil {
			return fmt.Errorf("recording migration %d: %w", m.Version, err)
		}

		slog.Info("applied extension migration",
			slog.String("extension_id", extensionID),
			slog.String("slug", slug),
			slog.Int("version", m.Version),
		)
	}

	return nil
}

// RunDown reverses all applied migrations in descending version order.
// Each down migration's SQL is validated before execution.
func (r *MigrationRunner) RunDown(ctx context.Context, extensionID, slug string, migrations []ExtensionMigration) error {
	applied, err := r.GetAppliedVersions(ctx, extensionID)
	if err != nil {
		return fmt.Errorf("checking applied versions: %w", err)
	}
	appliedSet := make(map[int]bool, len(applied))
	for _, v := range applied {
		appliedSet[v] = true
	}

	// Build a version-indexed map of migrations for lookup.
	migMap := make(map[int]ExtensionMigration, len(migrations))
	for _, m := range migrations {
		migMap[m.Version] = m
	}

	// Sort applied versions descending for reverse order execution.
	sort.Sort(sort.Reverse(sort.IntSlice(applied)))

	for _, version := range applied {
		m, ok := migMap[version]
		if !ok || m.DownSQL == "" {
			slog.Warn("no down migration found, skipping",
				slog.String("extension_id", extensionID),
				slog.Int("version", version),
			)
			continue
		}

		// Validate down SQL.
		if err := ValidateExtensionSQL(slug, m.DownSQL); err != nil {
			return fmt.Errorf("down migration %d validation failed: %w", version, err)
		}

		// Execute down migration.
		for _, stmt := range splitStatements(m.DownSQL) {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := r.db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("down migration %d failed: %w", version, err)
			}
		}

		// Remove the tracking row.
		_, err := r.db.ExecContext(ctx,
			`DELETE FROM extension_schema_versions WHERE extension_id = ? AND version = ?`,
			extensionID, version,
		)
		if err != nil {
			return fmt.Errorf("removing migration %d record: %w", version, err)
		}

		slog.Info("reversed extension migration",
			slog.String("extension_id", extensionID),
			slog.String("slug", slug),
			slog.Int("version", version),
		)
	}

	return nil
}

// DropExtensionTables drops all tables prefixed with ext_<slug>_ as a safety
// net during uninstall. This catches any tables that weren't covered by down
// migrations.
func (r *MigrationRunner) DropExtensionTables(ctx context.Context, slug string) error {
	prefix := "ext_" + slug + "_"

	rows, err := r.db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return fmt.Errorf("listing tables: %w", err)
	}
	defer rows.Close()

	var toDrop []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("scanning table name: %w", err)
		}
		if strings.HasPrefix(tableName, prefix) {
			toDrop = append(toDrop, tableName)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating tables: %w", err)
	}

	for _, table := range toDrop {
		// Table name is from SHOW TABLES, safe to interpolate.
		if _, err := r.db.ExecContext(ctx, "DROP TABLE IF EXISTS `"+table+"`"); err != nil {
			slog.Warn("failed to drop extension table",
				slog.String("table", table),
				slog.Any("error", err),
			)
		} else {
			slog.Info("dropped extension table",
				slog.String("slug", slug),
				slog.String("table", table),
			)
		}
	}

	return nil
}

// GetAppliedVersions returns all version numbers applied for an extension,
// sorted ascending.
func (r *MigrationRunner) GetAppliedVersions(ctx context.Context, extensionID string) ([]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT version FROM extension_schema_versions WHERE extension_id = ? ORDER BY version`,
		extensionID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying applied versions: %w", err)
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scanning version: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}
