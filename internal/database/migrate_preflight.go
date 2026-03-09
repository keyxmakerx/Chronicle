// Package database provides connection setup for MariaDB and Redis.
// This file implements pre-flight validation of pending SQL migrations
// against the live database schema to catch common MariaDB pitfalls
// (Error 1553, unsafe ENUM removals, etc.) before execution.
package database

import (
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

// preflightCheck represents a single validation rule that inspects a pending
// migration's SQL against the live INFORMATION_SCHEMA. Each check returns a
// non-nil error if the migration would fail at runtime.
type preflightCheck func(db *sql.DB, dbName string, sqlContent string, filename string) error

// preflightChecks is the ordered list of validations run against each pending
// migration before it is applied. Add new checks here as failure modes are
// discovered.
var preflightChecks = []preflightCheck{
	checkDropIndexBackedByFK,
	checkDropColumnBackedByFK,
}

// runPreflightValidation reads all pending .up.sql migration files from disk,
// determines which ones haven't been applied yet (by comparing to the current
// schema_migrations version), and runs each pending file through all preflight
// checks against the live database. Returns the first validation error found,
// or nil if all pending migrations pass.
func runPreflightValidation(db *sql.DB, migrationsPath string) error {
	// Determine current applied version from schema_migrations table.
	currentVersion, err := getCurrentVersion(db)
	if err != nil {
		// Table might not exist yet (fresh install). Skip validation.
		slog.Debug("preflight: cannot read schema_migrations, skipping validation",
			slog.String("error", err.Error()),
		)
		return nil
	}

	// Determine the database name for INFORMATION_SCHEMA queries.
	dbName, err := getDatabaseName(db)
	if err != nil {
		slog.Warn("preflight: cannot determine database name, skipping validation",
			slog.String("error", err.Error()),
		)
		return nil
	}

	// Read and sort pending migration files.
	pending, err := pendingMigrations(migrationsPath, currentVersion)
	if err != nil {
		slog.Warn("preflight: cannot read migration files, skipping validation",
			slog.String("error", err.Error()),
		)
		return nil
	}

	if len(pending) == 0 {
		return nil
	}

	slog.Info("preflight: validating pending migrations",
		slog.Int("count", len(pending)),
		slog.Uint64("current_version", uint64(currentVersion)),
	)

	for _, mf := range pending {
		for _, check := range preflightChecks {
			if err := check(db, dbName, mf.content, mf.filename); err != nil {
				return fmt.Errorf("preflight validation failed for %s: %w", mf.filename, err)
			}
		}
	}

	slog.Info("preflight: all pending migrations passed validation")
	return nil
}

// migrationFile holds a parsed pending migration's metadata and SQL content.
type migrationFile struct {
	version  uint
	filename string
	content  string
}

// migrationFileRe matches numbered migration filenames: 000063_description.up.sql
var migrationFileRe = regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)

// pendingMigrations reads the migrations directory and returns all .up.sql
// files with version numbers greater than currentVersion, sorted ascending.
func pendingMigrations(dir string, currentVersion uint) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading migrations dir: %w", err)
	}

	var pending []migrationFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		matches := migrationFileRe.FindStringSubmatch(e.Name())
		if matches == nil {
			continue
		}
		v, err := strconv.ParseUint(matches[1], 10, 64)
		if err != nil {
			continue
		}
		if uint(v) <= currentVersion {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}

		pending = append(pending, migrationFile{
			version:  uint(v),
			filename: e.Name(),
			content:  string(data),
		})
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].version < pending[j].version
	})
	return pending, nil
}

// getCurrentVersion reads the current migration version from schema_migrations.
func getCurrentVersion(db *sql.DB) (uint, error) {
	var version uint
	var dirty bool
	err := db.QueryRow("SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// getDatabaseName returns the name of the current database.
func getDatabaseName(db *sql.DB) (string, error) {
	var name string
	err := db.QueryRow("SELECT DATABASE()").Scan(&name)
	return name, err
}

// --- Preflight checks ---

// dropIndexRe matches: ALTER TABLE <table> DROP INDEX <index>
// Also matches DROP KEY which is a MariaDB synonym.
var dropIndexRe = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+` + "`?" + `(\w+)` + "`?" + `\s+DROP\s+(?:INDEX|KEY)\s+` + "`?" + `(\w+)` + "`?")

// dropFKRe matches: ALTER TABLE <table> DROP FOREIGN KEY <constraint>
var dropFKRe = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+` + "`?" + `(\w+)` + "`?" + `\s+DROP\s+FOREIGN\s+KEY\s+` + "`?" + `(\w+)` + "`?")

// checkDropIndexBackedByFK queries INFORMATION_SCHEMA to see if a DROP INDEX
// targets an index that is the only index backing a foreign key constraint.
// MariaDB refuses to drop such indexes with Error 1553.
//
// The check verifies: for each DROP INDEX in the SQL, if a FK depends on that
// index AND the migration doesn't DROP that FK first, it fails validation.
func checkDropIndexBackedByFK(db *sql.DB, dbName string, sqlContent string, filename string) error {
	indexMatches := dropIndexRe.FindAllStringSubmatch(sqlContent, -1)
	if indexMatches == nil {
		return nil
	}

	// Build set of (table, fk_name) pairs that are dropped before any DROP INDEX.
	// We parse statement-by-statement to respect ordering.
	droppedFKs := make(map[string]map[string]bool)
	stmts := splitSQLStatements(sqlContent)
	for _, stmt := range stmts {
		// If we hit a DROP INDEX, stop collecting — we only care about
		// FK drops that come BEFORE the index drop.
		if dropIndexRe.MatchString(stmt) {
			break
		}
		for _, fkMatch := range dropFKRe.FindAllStringSubmatch(stmt, -1) {
			table := strings.ToLower(fkMatch[1])
			fkName := strings.ToLower(fkMatch[2])
			if droppedFKs[table] == nil {
				droppedFKs[table] = make(map[string]bool)
			}
			droppedFKs[table][fkName] = true
		}
	}

	for _, m := range indexMatches {
		table := m[1]
		index := m[2]

		// Query: which FK constraints on this table use this index?
		fks, err := fksDependingOnIndex(db, dbName, table, index)
		if err != nil {
			slog.Warn("preflight: cannot query FK dependencies, skipping check",
				slog.String("table", table),
				slog.String("index", index),
				slog.String("error", err.Error()),
			)
			continue
		}

		for _, fkName := range fks {
			tableKey := strings.ToLower(table)
			if droppedFKs[tableKey] != nil && droppedFKs[tableKey][strings.ToLower(fkName)] {
				continue // FK is dropped before the index drop — safe.
			}
			return fmt.Errorf(
				"DROP INDEX %s on table %s would fail: foreign key %q depends on this index. "+
					"Add ALTER TABLE %s DROP FOREIGN KEY %s before the DROP INDEX (MariaDB Error 1553)",
				index, table, fkName, table, fkName,
			)
		}
	}
	return nil
}

// fksDependingOnIndex returns FK constraint names that depend on the given index.
// In MariaDB/InnoDB, a FK requires an index on the referencing columns. If the
// index being dropped is the ONLY index covering those columns, the FK blocks it.
func fksDependingOnIndex(db *sql.DB, dbName string, table string, indexName string) ([]string, error) {
	// Find which columns the target index covers.
	rows, err := db.Query(`
		SELECT COLUMN_NAME
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?
		ORDER BY SEQ_IN_INDEX`,
		dbName, table, indexName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexCols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		indexCols = append(indexCols, strings.ToLower(col))
	}
	if len(indexCols) == 0 {
		return nil, nil // Index doesn't exist (yet) — nothing to validate.
	}

	// Find FK constraints on this table whose columns match the index columns.
	fkRows, err := db.Query(`
		SELECT CONSTRAINT_NAME, COLUMN_NAME, ORDINAL_POSITION
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		  AND REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY CONSTRAINT_NAME, ORDINAL_POSITION`,
		dbName, table,
	)
	if err != nil {
		return nil, err
	}
	defer fkRows.Close()

	// Group FK columns by constraint name.
	type fkInfo struct {
		name string
		cols []string
	}
	fkMap := make(map[string]*fkInfo)
	for fkRows.Next() {
		var name, col string
		var pos int
		if err := fkRows.Scan(&name, &col, &pos); err != nil {
			return nil, err
		}
		if fkMap[name] == nil {
			fkMap[name] = &fkInfo{name: name}
		}
		fkMap[name].cols = append(fkMap[name].cols, strings.ToLower(col))
	}

	// Check: does the index being dropped cover a FK's leading columns?
	// InnoDB requires that some index covers the FK columns as a prefix.
	var dependentFKs []string
	for _, fk := range fkMap {
		if isPrefix(fk.cols, indexCols) {
			// This FK depends on this index. Check if another index also covers it.
			hasAlternate, err := hasAlternateIndex(db, dbName, table, indexName, fk.cols)
			if err != nil {
				return nil, err
			}
			if !hasAlternate {
				dependentFKs = append(dependentFKs, fk.name)
			}
		}
	}

	return dependentFKs, nil
}

// hasAlternateIndex checks whether any index OTHER than excludeIndex covers
// the given columns as a prefix. If so, dropping excludeIndex is safe because
// the FK has another index to fall back on.
func hasAlternateIndex(db *sql.DB, dbName string, table string, excludeIndex string, fkCols []string) (bool, error) {
	rows, err := db.Query(`
		SELECT INDEX_NAME, COLUMN_NAME, SEQ_IN_INDEX
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME != ?
		ORDER BY INDEX_NAME, SEQ_IN_INDEX`,
		dbName, table, excludeIndex,
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	idxCols := make(map[string][]string)
	for rows.Next() {
		var idxName, col string
		var seq int
		if err := rows.Scan(&idxName, &col, &seq); err != nil {
			return false, err
		}
		idxCols[idxName] = append(idxCols[idxName], strings.ToLower(col))
	}

	for _, cols := range idxCols {
		if isPrefix(fkCols, cols) {
			return true, nil
		}
	}
	return false, nil
}

// isPrefix returns true if every element in needle appears in haystack at the
// same position (i.e., needle is a prefix of haystack).
func isPrefix(needle []string, haystack []string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i, v := range needle {
		if v != haystack[i] {
			return false
		}
	}
	return true
}

// dropColumnRe matches: ALTER TABLE <table> DROP COLUMN <column>
var dropColumnRe = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+` + "`?" + `(\w+)` + "`?" + `\s+DROP\s+(?:COLUMN\s+)` + "`?" + `(\w+)` + "`?")

// checkDropColumnBackedByFK verifies that dropping a column won't fail because
// a foreign key constraint references it. MariaDB requires the FK to be dropped
// first.
func checkDropColumnBackedByFK(db *sql.DB, dbName string, sqlContent string, filename string) error {
	colMatches := dropColumnRe.FindAllStringSubmatch(sqlContent, -1)
	if colMatches == nil {
		return nil
	}

	for _, m := range colMatches {
		table := m[1]
		column := m[2]

		var count int
		err := db.QueryRow(`
			SELECT COUNT(*)
			FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?
			  AND REFERENCED_TABLE_NAME IS NOT NULL`,
			dbName, table, column,
		).Scan(&count)
		if err != nil {
			slog.Warn("preflight: cannot check FK on column",
				slog.String("table", table),
				slog.String("column", column),
				slog.String("error", err.Error()),
			)
			continue
		}

		if count > 0 {
			// Check if the migration drops the FK first.
			droppedBefore := false
			stmts := splitSQLStatements(sqlContent)
			for _, stmt := range stmts {
				if dropColumnRe.MatchString(stmt) {
					break
				}
				if dropFKRe.MatchString(stmt) && strings.Contains(strings.ToLower(stmt), strings.ToLower(table)) {
					droppedBefore = true
					break
				}
			}
			if !droppedBefore {
				return fmt.Errorf(
					"DROP COLUMN %s on table %s would fail: a foreign key references this column. "+
						"Drop the FK constraint before dropping the column",
					column, table,
				)
			}
		}
	}
	return nil
}

// splitSQLStatements splits SQL into individual statements by semicolons,
// stripping single-line comments. Used for ordering analysis.
func splitSQLStatements(sql string) []string {
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
