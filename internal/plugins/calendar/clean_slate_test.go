package calendar

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// CALV5-PLACEHOLDER: this file exists to guard migration 019 and is deleted
// when V5's own migrations replace it.

// cleanSlateMigration is the CALV5 wipe both guards in this file read, and
// cleanSlateVersion bounds which migrations count as "the chain 019 undoes":
// only versions BELOW it. Without the bound, V5's first 020+ CREATE TABLE
// would make the completeness guard demand that 019 — an applied, immutable
// migration by then — grow a new DROP, which is advice nobody may follow.
const (
	cleanSlateMigration = "019_calv5_clean_slate.up.sql"
	cleanSlateVersion   = 19
)

var (
	createTableRe = regexp.MustCompile("(?i)CREATE\\s+TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?`?([a-z_]+)`?")
	dropTableRe   = regexp.MustCompile("(?i)DROP\\s+TABLE\\s+IF\\s+EXISTS\\s+`?([a-z_]+)`?")
	deleteFromRe  = regexp.MustCompile("(?i)DELETE\\s+FROM\\s+`?([a-z_]+)`?")

	// fkDeclRe captures a foreign-key declaration: optional constraint name,
	// then the parent table. Run against comment-stripped, line-JOINED text —
	// "REFERENCES\n  calendars (id)" is valid MariaDB, and a line-by-line scan
	// proved blind to it.
	fkDeclRe = regexp.MustCompile("(?i)(?:CONSTRAINT\\s+`?([a-z0-9_]+)`?\\s+)?FOREIGN\\s+KEY\\s*\\([^)]*\\)\\s*REFERENCES\\s+`?([a-z0-9_]+)`?\\s*\\(")
	// dropFKRe captures a later migration removing a named constraint, which
	// RELEASES that declaration: migrations are append-only, so the original
	// declaration text never disappears — the only way an FK stops existing
	// is a DROP FOREIGN KEY in a later file, and the guard must honour it or
	// it holds the stubs hostage forever.
	dropFKRe = regexp.MustCompile("(?i)DROP\\s+FOREIGN\\s+KEY\\s+(?:IF\\s+EXISTS\\s+)?`?([a-z0-9_]+)`?")

	migVersionRe = regexp.MustCompile(`^(\d+)_`)
)

// migrationVersion extracts the numeric prefix of a migration filename, or -1.
func migrationVersion(name string) int {
	m := migVersionRe.FindStringSubmatch(name)
	if m == nil {
		return -1
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return v
}

// stripSQLComments removes `--` comment lines and joins the rest with spaces,
// so statements split across lines parse as one text and prose in comments is
// never mistaken for a statement.
func stripSQLComments(body string) string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, " ")
}

// readMigration returns one calendar migration's comment-stripped SQL.
func readMigration(t *testing.T, name string) string {
	t.Helper()
	body, err := fs.ReadFile(MigrationsFS, "migrations/"+name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return stripSQLComments(string(body))
}

// tablesCreatedBeforeCleanSlate parses migrations 001..018 and returns
// table -> the migration that created it. This is the ground truth both
// guards share: the set of tables 019 is responsible for undoing, and the
// only legitimate parents a sibling-plugin FK can pin.
func tablesCreatedBeforeCleanSlate(t *testing.T) map[string]string {
	t.Helper()
	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	created := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		v := migrationVersion(name)
		if v < 0 || v >= cleanSlateVersion {
			continue
		}
		for _, m := range createTableRe.FindAllStringSubmatch(readMigration(t, name), -1) {
			created[m[1]] = name
		}
	}
	if len(created) == 0 {
		t.Fatal("parsed no CREATE TABLE statements from 001-018 — the regex has drifted, " +
			"and a guard that matches nothing passes everything")
	}
	return created
}

// foreignKeyParentsOtherPluginsNeed scans every OTHER plugin's migrations for
// foreign keys whose parent is a table the calendar chain created, and returns
// parent -> the declarations that still need it. Derived from the SQL rather
// than hard-coded, so the guard moves when the SQL moves:
//
//   - the parent filter is membership in the chain's own CREATE TABLE set, not
//     a name prefix — entity_event_links and entity_era_links are calendar
//     tables without the calendar_ prefix, and a prefix filter waved through
//     a sibling FK to them, recreating the exact errno-150 defect;
//   - a NAMED declaration is released when a later migration in the same
//     plugin drops that constraint by name (append-only migrations mean the
//     declaration text never disappears — DROP FOREIGN KEY is the only way an
//     FK actually goes away);
//   - an UNNAMED declaration can never be released by this scan, and its
//     entry says so.
func foreignKeyParentsOtherPluginsNeed(t *testing.T, created map[string]string) map[string][]string {
	t.Helper()

	plugins, err := os.ReadDir("..")
	if err != nil {
		t.Fatalf("read plugins dir: %v", err)
	}

	needed := map[string][]string{}
	scanned := 0
	for _, p := range plugins {
		if !p.IsDir() || p.Name() == "calendar" {
			continue
		}
		dir := filepath.Join("..", p.Name(), "migrations")
		files, err := os.ReadDir(dir)
		if err != nil {
			continue // plugin has no migrations
		}

		type decl struct {
			name, parent, where string
		}
		var decls []decl
		droppedNames := map[string]bool{}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".up.sql") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				t.Fatalf("read %s/%s: %v", dir, f.Name(), err)
			}
			scanned++
			text := stripSQLComments(string(body))
			for _, m := range fkDeclRe.FindAllStringSubmatch(text, -1) {
				parent := strings.ToLower(m[2])
				if _, ok := created[parent]; !ok {
					continue
				}
				decls = append(decls, decl{name: strings.ToLower(m[1]), parent: parent,
					where: p.Name() + "/" + f.Name()})
			}
			for _, m := range dropFKRe.FindAllStringSubmatch(text, -1) {
				droppedNames[strings.ToLower(m[1])] = true
			}
		}
		for _, d := range decls {
			if d.name != "" && droppedNames[d.name] {
				continue // released by a later DROP FOREIGN KEY in this plugin
			}
			label := d.where + ": "
			if d.name != "" {
				label += "CONSTRAINT " + d.name + " -> " + d.parent +
					" (release it with a later ALTER TABLE ... DROP FOREIGN KEY " + d.name + ")"
			} else {
				label += "unnamed FOREIGN KEY -> " + d.parent +
					" (unnamed: this scan can never see it released)"
			}
			needed[d.parent] = append(needed[d.parent], label)
		}
	}

	if scanned == 0 {
		t.Fatal("scanned no sibling plugin migrations — the path has drifted, and a " +
			"guard that reads nothing passes everything")
	}
	return needed
}

// TestCleanSlateKeepsForeignKeyParentsOtherPluginsStillReference is the guard
// for the defect CI caught on PR #595: 019 dropped `calendars` and
// `calendar_events` while sessions/001 and timeline/001 still declared foreign
// keys to them.
//
// Those migrations are immutable, so the constraints are recreated verbatim on
// every fresh database. The calendar plugin migrates BEFORE both, so dropping
// the parents made sessions/001 and timeline/001 fail on statement 1 with
// errno 150, leaving both plugins DEGRADED at version 0 — sessions and
// timeline simply dead on every new install. On an existing database the same
// constraints block the DROP outright, failing the migration with DDL already
// committed and no rollback available.
//
// Emptying the parents does the same job with none of that: the deletes
// cascade and null the dependent rows through the constraints instead of
// fighting them. When V5 ships sessions/timeline migrations that DROP those
// constraints by name, this guard sees the release and starts requiring the
// stubs to be dropped instead — the completeness test's exemption vanishes
// with the constraints.
func TestCleanSlateKeepsForeignKeyParentsOtherPluginsStillReference(t *testing.T) {
	created := tablesCreatedBeforeCleanSlate(t)
	needed := foreignKeyParentsOtherPluginsNeed(t, created)
	if len(needed) == 0 {
		t.Skip("no other plugin references a calendar table; nothing to protect")
	}

	sql := readMigration(t, cleanSlateMigration)

	dropped := map[string]bool{}
	for _, m := range dropTableRe.FindAllStringSubmatch(sql, -1) {
		dropped[m[1]] = true
	}
	emptied := map[string]bool{}
	for _, m := range deleteFromRe.FindAllStringSubmatch(sql, -1) {
		emptied[m[1]] = true
	}

	for parent, decls := range needed {
		if dropped[parent] {
			t.Errorf("019 drops %s, but it is still the parent of a foreign key declared "+
				"by another plugin whose migration cannot be edited:\n  %s\n"+
				"On a fresh database that migration fails on its first statement "+
				"(errno 150) and the plugin is DEGRADED at version 0 — the feature is "+
				"dead on every new install. Empty the table instead of dropping it, or "+
				"first ship migrations for those plugins that remove the constraints.",
				parent, strings.Join(decls, "\n  "))
			continue
		}
		if !emptied[parent] {
			t.Errorf("019 neither drops nor empties %s, so the clean slate leaves its rows "+
				"behind. It must survive as an FK parent (%d declaration(s)), so it needs "+
				"an explicit DELETE FROM %s.", parent, len(decls), parent)
		}
	}
}

// TestCleanSlateDropsEveryTableTheChainCreated is the completeness check on the
// CALV5 wipe: every table migrations 001-018 create must be dropped by 019 —
// or, when a sibling plugin's live foreign key forbids the drop, emptied.
//
// The failure it exists for is silent and permanent. A table left out is not an
// error at boot and not a failing query — it is an orphan sitting in the
// operator's live database forever, holding player data nobody can see, that
// V5 will later collide with when it creates a table by the same name.
func TestCleanSlateDropsEveryTableTheChainCreated(t *testing.T) {
	created := tablesCreatedBeforeCleanSlate(t)
	cleanSlate := readMigration(t, cleanSlateMigration)
	if strings.TrimSpace(cleanSlate) == "" {
		t.Fatal("migration 019 (the CALV5 clean slate) is missing — the wipe cannot run")
	}

	dropped := map[string]bool{}
	for _, m := range dropTableRe.FindAllStringSubmatch(cleanSlate, -1) {
		dropped[m[1]] = true
	}
	emptied := map[string]bool{}
	for _, m := range deleteFromRe.FindAllStringSubmatch(cleanSlate, -1) {
		emptied[m[1]] = true
	}

	// Tables another plugin still points a foreign key at cannot be dropped
	// here; they are emptied instead, and the FK guard above holds them to
	// that. The exemption is derived, not hard-coded, so the two guards can
	// never disagree — and it disappears the moment the constraints are
	// released, at which point this test starts demanding the stubs go too
	// (in a NEW migration; 019 is immutable once applied anywhere).
	fkParents := foreignKeyParentsOtherPluginsNeed(t, created)

	var missing []string
	for table, origin := range created {
		if dropped[table] {
			continue
		}
		if len(fkParents[table]) > 0 && emptied[table] {
			continue
		}
		missing = append(missing, table+" (created by "+origin+")")
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("019 neither drops nor legitimately empties %s — it would survive the "+
			"clean slate as an invisible orphan and collide with V5's schema later "+
			"(emptying is legitimate only while a sibling plugin's foreign key pins it)", m)
	}

	// The reverse direction: touching a table this chain never created —
	// dropping OR emptying it — is either a typo or another plugin's table,
	// and reaching into another plugin's table from here is exactly the
	// cross-plugin reach rule 8 forbids.
	for table := range dropped {
		if _, ok := created[table]; !ok {
			t.Errorf("019 drops %s, which no calendar migration creates — typo, or "+
				"another plugin's table being dropped from the calendar's chain", table)
		}
	}
	for table := range emptied {
		if _, ok := created[table]; !ok {
			t.Errorf("019 empties %s, which no calendar migration creates — typo, or "+
				"another plugin's data being wiped from the calendar's chain", table)
		}
	}
}
