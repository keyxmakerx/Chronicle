// Package database provides connection setup for MariaDB and Redis.
// This file validates migration SQL files to catch schema mismatches early.
package database

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// validAddonCategories must match the ENUM values on addons.category.
// Update this set when adding new ENUM values via ALTER TABLE.
// Current ENUM: ENUM('system', 'widget', 'integration', 'plugin')
var validAddonCategories = map[string]bool{
	"system":      true,
	"module":      true, // Legacy: referenced in old migrations.
	"widget":      true,
	"integration": true,
	"plugin":      true,
}

// validAddonStatuses must match the ENUM values on addons.status.
// Current ENUM: ENUM('active', 'planned', 'deprecated')
var validAddonStatuses = map[string]bool{
	"active":     true,
	"planned":    true,
	"deprecated": true,
}

// migrationsDir returns the absolute path to db/migrations/ from the project root.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	// thisFile is internal/database/migrate_test.go, project root is two dirs up.
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	dir := filepath.Join(projectRoot, "db", "migrations")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("migrations directory not found at %s: %v", dir, err)
	}
	return dir
}

// TestMigrations_AddonCategoryValues scans all .up.sql migration files for
// INSERT or UPDATE statements that reference the addons table and validates
// that any category values used are valid ENUM members. This prevents the
// "Data truncated for column 'category'" crash (Error 1265) that occurs
// when an invalid ENUM value is used.
func TestMigrations_AddonCategoryValues(t *testing.T) {
	dir := migrationsDir(t)
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("globbing migration files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no migration files found")
	}

	// Match category = 'value' or category, ... 'value' patterns.
	categoryPattern := regexp.MustCompile(`category\s*[=,]\s*'([^']+)'`)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		content := string(data)

		// Only check files that reference the addons table.
		if !strings.Contains(content, "addons") {
			continue
		}

		// Skip ALTER TABLE statements (they define the ENUM, not use it).
		// We only care about INSERT/UPDATE statements.
		lines := strings.Split(content, "\n")
		inAlter := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(strings.ToUpper(line))
			if strings.HasPrefix(trimmed, "ALTER TABLE") {
				inAlter = true
			}
			if inAlter {
				if strings.Contains(line, ";") {
					inAlter = false
				}
				continue
			}

			matches := categoryPattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				value := match[1]
				if !validAddonCategories[value] {
					t.Errorf("%s: invalid addon category %q; valid values: module, widget, integration, plugin",
						filepath.Base(f), value)
				}
			}
		}
	}
}

// TestMigrations_AddonStatusValues validates status ENUM values in migration files.
func TestMigrations_AddonStatusValues(t *testing.T) {
	dir := migrationsDir(t)
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("globbing migration files: %v", err)
	}

	statusPattern := regexp.MustCompile(`status\s*[=,]\s*'([^']+)'`)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		content := string(data)

		if !strings.Contains(content, "addons") {
			continue
		}

		// Skip ALTER TABLE and CREATE TABLE (ENUM definitions).
		lines := strings.Split(content, "\n")
		inDDL := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(strings.ToUpper(line))
			if strings.HasPrefix(trimmed, "ALTER TABLE") || strings.HasPrefix(trimmed, "CREATE TABLE") {
				inDDL = true
			}
			if inDDL {
				if strings.Contains(line, ";") {
					inDDL = false
				}
				continue
			}

			matches := statusPattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				value := match[1]
				if !validAddonStatuses[value] {
					t.Errorf("%s: invalid addon status %q; valid values: active, planned, deprecated",
						filepath.Base(f), value)
				}
			}
		}
	}
}

// TestMigrations_UpDownPairs ensures every .up.sql has a matching .down.sql.
func TestMigrations_UpDownPairs(t *testing.T) {
	dir := migrationsDir(t)
	upFiles, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("globbing up files: %v", err)
	}

	for _, up := range upFiles {
		down := strings.Replace(up, ".up.sql", ".down.sql", 1)
		if _, err := os.Stat(down); err != nil {
			t.Errorf("missing down migration for %s", filepath.Base(up))
		}
	}
}

// pluginCreateTableRe matches `CREATE TABLE [IF NOT EXISTS] <name>` and captures
// the table name. Used to derive the set of plugin-owned tables.
var pluginCreateTableRe = regexp.MustCompile(`(?i)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+` + "`?" + `(\w+)` + "`?")

// pluginsDir returns the absolute path to internal/plugins/ from the project root.
func pluginsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	dir := filepath.Join(projectRoot, "internal", "plugins")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("plugins directory not found at %s: %v", dir, err)
	}
	return dir
}

// collectPluginOwnedTables walks every plugin migrations/ directory and
// extracts the set of tables created there. These tables are off-limits to
// core migrations because plugin migrations run AFTER core (ADR-028) — a core
// migration that references them crashes on a fresh DB.
func collectPluginOwnedTables(t *testing.T) map[string]string {
	t.Helper()
	owned := map[string]string{} // table -> owning plugin slug

	entries, err := os.ReadDir(pluginsDir(t))
	if err != nil {
		t.Fatalf("reading plugins dir: %v", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		migDir := filepath.Join(pluginsDir(t), slug, "migrations")
		if _, err := os.Stat(migDir); err != nil {
			continue
		}
		ups, err := filepath.Glob(filepath.Join(migDir, "*.up.sql"))
		if err != nil {
			t.Fatalf("globbing %s: %v", migDir, err)
		}
		for _, f := range ups {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("reading %s: %v", f, err)
			}
			for _, m := range pluginCreateTableRe.FindAllStringSubmatch(string(data), -1) {
				owned[strings.ToLower(m[1])] = slug
			}
		}
	}
	return owned
}

// TestMigrations_NoPluginTableReferences enforces the migration layering rule
// (see .ai/conventions.md §"Migration Safety Rules" #7): core migrations may
// only reference core schema. Referencing a plugin-owned table from
// db/migrations/ guarantees a crash on a fresh DB because plugin migrations
// haven't run yet.
//
// The plugin-owned table set is derived dynamically from each plugin's
// migrations/ directory, so adding a new plugin table automatically extends
// this guard with no test maintenance.
func TestMigrations_NoPluginTableReferences(t *testing.T) {
	owned := collectPluginOwnedTables(t)
	if len(owned) == 0 {
		t.Fatal("no plugin-owned tables discovered; guard would be vacuous")
	}

	dir := migrationsDir(t)
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("globbing migration files: %v", err)
	}

	// Match identifiers in contexts that read or write a table:
	// FROM, JOIN, INTO, UPDATE, DELETE FROM, REFERENCES.
	refPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bFROM\s+` + "`?" + `(\w+)` + "`?"),
		regexp.MustCompile(`(?i)\bJOIN\s+` + "`?" + `(\w+)` + "`?"),
		regexp.MustCompile(`(?i)\bINTO\s+` + "`?" + `(\w+)` + "`?"),
		regexp.MustCompile(`(?i)\bUPDATE\s+` + "`?" + `(\w+)` + "`?"),
		regexp.MustCompile(`(?i)\bREFERENCES\s+` + "`?" + `(\w+)` + "`?"),
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		content := string(data)

		for _, re := range refPatterns {
			for _, m := range re.FindAllStringSubmatch(content, -1) {
				name := strings.ToLower(m[1])
				if slug, ok := owned[name]; ok {
					t.Errorf("%s references plugin-owned table %q (owned by plugin %q); "+
						"move this statement to internal/plugins/%s/migrations/",
						filepath.Base(f), name, slug, slug)
				}
			}
		}
	}
}
