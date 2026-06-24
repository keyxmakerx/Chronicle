// Package database provides connection setup for MariaDB and Redis.
// This file validates migration SQL files to catch schema mismatches early.
package database

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
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

// TestExpectedCoreMigrationVersion_MatchesMax pins the health-check floor
// (ExpectedCoreMigrationVersion, used in cmd/server/main.go) to the highest
// on-disk core migration. It guards the drift that the 000030 incident exposed:
// the constant was 29 while the newest migration was 30. If the constant lags
// the real max, a deploy missing the newest migration passes a floor check it
// should fail; if it leads, every normal boot fails the floor check. Reuses the
// production HighestSourceVersion so the parse logic can't diverge.
func TestExpectedCoreMigrationVersion_MatchesMax(t *testing.T) {
	max, err := HighestSourceVersion(migrationsDir(t))
	if err != nil {
		t.Fatalf("scanning migrations: %v", err)
	}
	if max == 0 {
		t.Fatal("no migrations found in db/migrations")
	}
	if ExpectedCoreMigrationVersion != max {
		t.Fatalf("ExpectedCoreMigrationVersion = %d, but the highest db/migrations file is %d — "+
			"update the constant in internal/database/migrate_state.go to match",
			ExpectedCoreMigrationVersion, max)
	}
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

// --- Migration safety guards added after the 000030 incident (ADR-044) ---

// TestMigrations_GaplessSequence asserts the core migration numbers form a
// contiguous 1..N run with no gaps or duplicates. A gap makes golang-migrate's
// file source stop early; a duplicate is ambiguous. (See ADR-044.)
func TestMigrations_GaplessSequence(t *testing.T) {
	dir := migrationsDir(t)
	ups, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("globbing up files: %v", err)
	}
	var nums []int
	for _, f := range ups {
		base := filepath.Base(f)
		i := strings.IndexByte(base, '_')
		if i <= 0 {
			t.Fatalf("migration without a NNNNNN_ prefix: %s", base)
		}
		n, err := strconv.Atoi(base[:i])
		if err != nil {
			t.Fatalf("non-numeric migration prefix in %s: %v", base, err)
		}
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for i, n := range nums {
		if want := i + 1; n != want {
			t.Fatalf("core migrations are not gapless/contiguous: expected %d at sorted position %d but got %d (full sequence: %v)",
				want, i, n, nums)
		}
	}
}

// TestMigrations_PluginUpDownPairs extends the up/down-pair guarantee to every
// plugin's migrations/ directory (core is covered by TestMigrations_UpDownPairs).
// A present down file is also what lets a future build's source "contain" a
// version a rolled-back DB recorded — part of the 000030 recovery story.
func TestMigrations_PluginUpDownPairs(t *testing.T) {
	entries, err := os.ReadDir(pluginsDir(t))
	if err != nil {
		t.Fatalf("reading plugins dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		migDir := filepath.Join(pluginsDir(t), e.Name(), "migrations")
		if _, err := os.Stat(migDir); err != nil {
			continue
		}
		ups, err := filepath.Glob(filepath.Join(migDir, "*.up.sql"))
		if err != nil {
			t.Fatalf("globbing %s: %v", migDir, err)
		}
		for _, up := range ups {
			down := strings.Replace(up, ".up.sql", ".down.sql", 1)
			if _, err := os.Stat(down); err != nil {
				t.Errorf("missing down migration for %s/%s", e.Name(), filepath.Base(up))
			}
		}
	}
}

// grandfatheredNonIdempotentMigrations exempts migration FILES that pre-date the
// idempotent-DDL rule (they use bare ADD COLUMN / CREATE TABLE / DROP without
// IF [NOT] EXISTS). They are immutable — the migration-immutability guard
// (tools/check-migration-immutability.sh) forbids editing them — so they're
// exempt forever. NEW migrations must be idempotent so the dirty-state recovery
// path (the Force(v-1) re-run in migrate.go) is unconditionally safe. Generated
// from the current tree; keyed by basename (unique across core + all plugins).
var grandfatheredNonIdempotentMigrations = map[string]bool{
	// core
	"000002_note_sharing.up.sql":                   true,
	"000004_cover_image.up.sql":                    true,
	"000006_owner_dashboard.up.sql":                true,
	"000009_entity_type_preset_category.up.sql":    true,
	"000011_inventory_instances.up.sql":            true,
	"000012_entity_is_folder.up.sql":               true,
	"000013_sidebar_nodes.up.sql":                  true,
	"000014_entity_favorites.up.sql":               true,
	"000015_saved_filters.up.sql":                  true,
	"000016_entity_player_notes.up.sql":            true,
	"000018_campaign_archive_and_join_code.up.sql": true,
	"000019_entity_type_parent.up.sql":             true,
	"000020_entity_search_text.up.sql":             true,
	"000022_entity_owner_user.up.sql":              true,
	"000023_entity_notes.up.sql":                   true,
	"000025_entity_map_id.up.sql":                  true,
	"000026_media_content_hash.up.sql":             true,
	"000029_entity_type_claimable.up.sql":          true,
	// plugins
	"002_event_extended_fields.up.sql":       true,
	"004_v2_foundation.up.sql":               true,
	"007_sidebar_pinned.up.sql":              true,
	"008_worldstate_model.up.sql":            true,
	"010_calendar_visibility.up.sql":         true,
	"011_event_recurrence_dow.up.sql":        true,
	"001_consolidate_foundry_modules.up.sql": true,
	"002_marker_visibility_rules.up.sql":     true,
	"003_marker_pin_category.up.sql":         true,
	"004_marker_foundry_id.up.sql":           true,
	"006_layer_fog_updated_at.up.sql":        true,
	"002_submission_workflow.up.sql":         true,
	"003_version_prerelease.up.sql":          true,
	"002_api_key_vtt_tag.up.sql":             true,
}

var (
	ddlAddColumnRe   = regexp.MustCompile(`(?i)\bADD\s+COLUMN\b`)
	ddlCreateTableRe = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\b`)
	ddlDropRe        = regexp.MustCompile(`(?i)\bDROP\s+(COLUMN|TABLE|INDEX)\b`)
	ddlIfNotExistsRe = regexp.MustCompile(`(?i)\bIF\s+NOT\s+EXISTS\b`)
	ddlIfExistsRe    = regexp.MustCompile(`(?i)\bIF\s+EXISTS\b`)
)

// scanNonIdempotentDDL returns descriptions of non-idempotent DDL lines.
func scanNonIdempotentDDL(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var problems []string
	for i, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue // SQL comment
		}
		switch {
		case ddlAddColumnRe.MatchString(line) && !ddlIfNotExistsRe.MatchString(line):
			problems = append(problems, fmt.Sprintf("line %d: ADD COLUMN without IF NOT EXISTS", i+1))
		case ddlCreateTableRe.MatchString(line) && !ddlIfNotExistsRe.MatchString(line):
			problems = append(problems, fmt.Sprintf("line %d: CREATE TABLE without IF NOT EXISTS", i+1))
		case ddlDropRe.MatchString(line) && !ddlIfExistsRe.MatchString(line):
			problems = append(problems, fmt.Sprintf("line %d: DROP without IF EXISTS", i+1))
		}
	}
	return problems
}

// TestMigrations_IdempotentDDL fails NEW migrations that use bare
// ADD COLUMN / CREATE TABLE / DROP (no IF [NOT] EXISTS). Idempotent DDL makes the
// dirty-state recovery path safe (a partial run + retry won't die on "Duplicate
// column", Error 1060). Historical offenders are grandfathered (they're immutable).
// Mechanizes .ai/conventions.md §"Migration Safety Rules" #8.
func TestMigrations_IdempotentDDL(t *testing.T) {
	var files []string
	core, err := filepath.Glob(filepath.Join(migrationsDir(t), "*.up.sql"))
	if err != nil {
		t.Fatalf("globbing core migrations: %v", err)
	}
	files = append(files, core...)

	entries, err := os.ReadDir(pluginsDir(t))
	if err != nil {
		t.Fatalf("reading plugins dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		migDir := filepath.Join(pluginsDir(t), e.Name(), "migrations")
		if _, err := os.Stat(migDir); err != nil {
			continue
		}
		ups, err := filepath.Glob(filepath.Join(migDir, "*.up.sql"))
		if err != nil {
			t.Fatalf("globbing %s: %v", migDir, err)
		}
		files = append(files, ups...)
	}

	for _, f := range files {
		base := filepath.Base(f)
		if grandfatheredNonIdempotentMigrations[base] {
			continue
		}
		if problems := scanNonIdempotentDDL(t, f); len(problems) > 0 {
			t.Errorf("non-idempotent DDL in %s — use ADD COLUMN IF NOT EXISTS / CREATE TABLE IF NOT EXISTS / "+
				"DROP ... IF EXISTS so dirty-state recovery is safe:\n  %s", base, strings.Join(problems, "\n  "))
		}
	}
}
