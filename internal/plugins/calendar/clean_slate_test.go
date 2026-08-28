package calendar

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// cleanSlateMigration is the CALV5 wipe both guards in this file read.
const cleanSlateMigration = "019_calv5_clean_slate.up.sql"

// CALV5-PLACEHOLDER: this file exists to guard migration 019 and is deleted
// when V5's own migrations replace it.

var (
	createTableRe = regexp.MustCompile("(?i)CREATE\\s+TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?`?([a-z_]+)`?")
	dropTableRe   = regexp.MustCompile("(?i)DROP\\s+TABLE\\s+IF\\s+EXISTS\\s+`?([a-z_]+)`?")
	deleteFromRe  = regexp.MustCompile("(?i)DELETE\\s+FROM\\s+`?([a-z_]+)`?")
	referencesRe  = regexp.MustCompile("(?i)REFERENCES\\s+`?([a-z_]+)`?\\s*\\(")
)

// foreignKeyParentsOtherPluginsNeed scans every OTHER plugin's migrations for
// foreign keys whose parent is a calendar table, and returns parent -> the
// declarations that need it. The set is derived from the SQL rather than
// hard-coded, so adding or removing such an FK moves this guard with it
// instead of leaving a stale list behind.
func foreignKeyParentsOtherPluginsNeed(t *testing.T) map[string][]string {
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
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".up.sql") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				t.Fatalf("read %s/%s: %v", dir, f.Name(), err)
			}
			scanned++
			for _, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "--") {
					continue
				}
				for _, m := range referencesRe.FindAllStringSubmatch(line, -1) {
					parent := m[1]
					if parent != "calendars" && !strings.HasPrefix(parent, "calendar_") {
						continue
					}
					needed[parent] = append(needed[parent],
						p.Name()+"/"+f.Name()+": "+strings.TrimSpace(line))
				}
			}
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
// fighting them.
func TestCleanSlateKeepsForeignKeyParentsOtherPluginsStillReference(t *testing.T) {
	needed := foreignKeyParentsOtherPluginsNeed(t)
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

// readMigration returns one migration file's SQL with comment lines stripped,
// so a table named only in prose is never mistaken for a statement. The
// comments in 019 name several tables on purpose.
func readMigration(t *testing.T, name string) string {
	t.Helper()
	body, err := fs.ReadFile(MigrationsFS, "migrations/"+name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// TestCleanSlateDropsEveryTableTheChainCreated is the completeness check on the
// CALV5 wipe: every table migrations 001-018 create must be dropped by 019.
//
// The failure it exists for is silent and permanent. A table left out is not an
// error at boot and not a failing query — it is an orphan sitting in the
// operator's live database forever, holding player data nobody can see, that
// V5 will later collide with when it creates a table by the same name.
func TestCleanSlateDropsEveryTableTheChainCreated(t *testing.T) {
	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	created := map[string]string{} // table -> migration that created it
	var cleanSlate string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		sql := readMigration(t, name)
		if strings.HasPrefix(name, "019_") {
			cleanSlate = sql
			continue
		}
		for _, m := range createTableRe.FindAllStringSubmatch(sql, -1) {
			created[m[1]] = name
		}
	}

	if cleanSlate == "" {
		t.Fatal("migration 019 (the CALV5 clean slate) is missing — the wipe cannot run")
	}
	if len(created) == 0 {
		t.Fatal("parsed no CREATE TABLE statements from 001-018 — the regex has drifted, " +
			"and a guard that matches nothing passes everything")
	}

	dropped := map[string]bool{}
	for _, m := range dropTableRe.FindAllStringSubmatch(cleanSlate, -1) {
		dropped[m[1]] = true
	}

	// Tables another plugin still points a foreign key at cannot be dropped
	// here; they are emptied instead, and
	// TestCleanSlateKeepsForeignKeyParentsOtherPluginsStillReference is what
	// holds them to that. Exempt them from the drop requirement rather than
	// hard-coding names, so the two guards can never disagree.
	fkParents := foreignKeyParentsOtherPluginsNeed(t)

	var missing []string
	for table, origin := range created {
		if dropped[table] || len(fkParents[table]) > 0 {
			continue
		}
		missing = append(missing, table+" (created by "+origin+")")
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("019 never drops %s — it would survive the clean slate as an "+
			"invisible orphan and collide with V5's schema later", m)
	}

	// The reverse direction: a DROP for a table this chain never created is
	// either a typo or another plugin's table, and dropping another plugin's
	// table from here is exactly the cross-plugin reach rule 8 forbids.
	for table := range dropped {
		if _, ok := created[table]; !ok {
			t.Errorf("019 drops %s, which no calendar migration creates — typo, or "+
				"another plugin's table being dropped from the calendar's chain", table)
		}
	}
}
