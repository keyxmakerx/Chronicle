package calendar

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// CALV5-PLACEHOLDER: this file exists to guard migration 019 and is deleted
// when V5's own migrations replace it.

var (
	createTableRe = regexp.MustCompile("(?i)CREATE\\s+TABLE\\s+(?:IF\\s+NOT\\s+EXISTS\\s+)?`?([a-z_]+)`?")
	dropTableRe   = regexp.MustCompile("(?i)DROP\\s+TABLE\\s+IF\\s+EXISTS\\s+`?([a-z_]+)`?")
)

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

	var missing []string
	for table, origin := range created {
		if !dropped[table] {
			missing = append(missing, table+" (created by "+origin+")")
		}
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
