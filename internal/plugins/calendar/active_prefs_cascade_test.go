// active_prefs_cascade_test.go — C-SWEEP-R4 stage 25,
// data/calendar-active-cascade-wipes-prefs.
//
// calendar_active carries FOUR facts on one (user_id, campaign_id) row, and only
// the first of them is about a particular calendar:
//
//	calendar_id     the viewer's last switcher choice        CALENDAR-scoped
//	sidebar_pinned  the V2 shell's sidebar pin (007)         CAMPAIGN-scoped
//	block_layers    the calendar-v4 Block layer set (014)    CAMPAIGN-scoped
//	bench_sections  the Bench's collapsed sections (016)     CAMPAIGN-scoped
//
// fk_calendar_active_cal cascaded on DELETE, and a cascade deletes the ROW — so
// deleting one calendar reset every viewer's sidebar pin, layer set and Bench
// sections for the entire campaign. Migration 017 makes calendar_id nullable and
// moves the FK to ON DELETE SET NULL: the pointer is still cleared, the three
// preferences beside it survive.
//
// NO MARIADB RUNS IN THIS BUILD, which prefs_calendar_id_test.go's header
// already states for the sibling FK defect. So the two halves below assert the
// two things that CAN be established without one, and between them they cover
// the whole change:
//
//  1. the SHIPPED MIGRATION SQL leaves the constraint in the SET NULL state
//     and the column nullable — read out of the embedded files, in order, so
//     it measures what a real database would end up with rather than what one
//     file says;
//  2. the READER survives what the new schema can now hand it — a NULL — and
//     answers "" rather than erroring, which is what makes NULL and "no row"
//     the same answer to resolveActiveCalendar's ladder.
//
// Half 2 is not optional garnish: without it the migration alone would 500 every
// viewer in a campaign that had ever deleted a calendar, because scanning NULL
// into a plain string is a driver error.
package calendar

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// --- half 1: the shipped migration SQL --------------------------------------

// activeCalFKState is the state of fk_calendar_active_cal after replaying every
// migration in order: whether it exists, and what it does on delete.
type activeCalFKState struct {
	exists    bool
	onDelete  string // "CASCADE" | "SET NULL" | "" (unspecified → RESTRICT)
	nullable  bool
	fromFiles []string
}

var (
	// The three shapes that matter, matched loosely enough that reformatting
	// the SQL does not defeat the test and tightly enough that they cannot
	// match each other.
	reAddFK  = regexp.MustCompile(`(?is)add\s+constraint\s+(?:if\s+not\s+exists\s+)?fk_calendar_active_cal.*?references\s+calendars\s*\(\s*id\s*\)\s*(on\s+delete\s+(cascade|set\s+null|restrict|no\s+action))?`)
	reDropFK = regexp.MustCompile(`(?is)drop\s+foreign\s+key\s+(?:if\s+exists\s+)?fk_calendar_active_cal`)
	reModify = regexp.MustCompile(`(?is)modify\s+column\s+calendar_id\s+varchar\(\s*36\s*\)\s*(not\s+null|null)?`)
	// migration 006 creates the table with an inline CONSTRAINT clause.
	reCreateFK  = regexp.MustCompile(`(?is)constraint\s+fk_calendar_active_cal\s+foreign\s+key\s*\(\s*calendar_id\s*\)\s+references\s+calendars\s*\(\s*id\s*\)\s*(on\s+delete\s+(cascade|set\s+null|restrict|no\s+action))?`)
	reCreateCol = regexp.MustCompile(`(?is)calendar_id\s+varchar\(\s*36\s*\)\s+(not\s+null|null)`)
)

// replayActiveCalMigrations reads every embedded *.up.sql in version order and
// folds the statements that touch calendar_active's pointer column and foreign
// key, returning the state a real database would be left in.
//
// It reads the SHIPPED files rather than a copy, so a migration that reverts
// this is caught by the same test that proved it landed.
func replayActiveCalMigrations(t *testing.T, fsys fs.FS) activeCalFKState {
	t.Helper()

	names, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var ups []string
	for _, e := range names {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups = append(ups, e.Name())
		}
	}
	// Lexical order is version order: the files are zero-padded (001_… 017_…).
	sort.Strings(ups)

	var st activeCalFKState
	for _, name := range ups {
		body, err := fs.ReadFile(fsys, "migrations/"+name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sqlText := stripSQLComments(string(body))
		if !strings.Contains(strings.ToLower(sqlText), "calendar_active") {
			continue
		}
		touched := false

		if m := reCreateFK.FindStringSubmatch(sqlText); m != nil {
			st.exists, st.onDelete, touched = true, normaliseOnDelete(m[2]), true
		}
		if m := reCreateCol.FindStringSubmatch(sqlText); m != nil {
			st.nullable, touched = !strings.EqualFold(strings.Join(strings.Fields(m[1]), " "), "not null"), true
		}
		if reDropFK.MatchString(sqlText) {
			st.exists, st.onDelete, touched = false, "", true
		}
		if m := reModify.FindStringSubmatch(sqlText); m != nil {
			// A MODIFY with no explicit NOT NULL is nullable, which is MariaDB's
			// own default for a restated column definition.
			st.nullable, touched = !strings.EqualFold(strings.Join(strings.Fields(m[1]), " "), "not null"), true
		}
		if m := reAddFK.FindStringSubmatch(sqlText); m != nil {
			st.exists, st.onDelete, touched = true, normaliseOnDelete(m[2]), true
		}
		if touched {
			st.fromFiles = append(st.fromFiles, name)
		}
	}
	return st
}

func normaliseOnDelete(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), " "))
}

// stripSQLComments removes `--` line comments so a comment quoting the OLD
// clause (017's header quotes "ON DELETE CASCADE" while explaining why it is
// being replaced) cannot be mistaken for DDL.
func stripSQLComments(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// TestActiveCalendarFKDoesNotCascadeDelete is the schema regression.
//
// It replays the shipped migrations and asserts the END STATE, because that is
// what a database has: migration 006 really does declare ON DELETE CASCADE and
// that file is immutable, so any test that read one file in isolation would
// either assert the bug or assert nothing.
func TestActiveCalendarFKDoesNotCascadeDelete(t *testing.T) {
	st := replayActiveCalMigrations(t, MigrationsFS)

	if !st.exists {
		t.Fatalf("fk_calendar_active_cal does not exist after replaying %v — the pointer "+
			"must still be a real foreign key; SET NULL is the point, not dropping the "+
			"constraint", st.fromFiles)
	}
	if st.onDelete == "CASCADE" {
		t.Errorf("fk_calendar_active_cal is still ON DELETE CASCADE after replaying %v. "+
			"A cascade deletes the ROW, so deleting one calendar destroys every viewer's "+
			"sidebar_pinned, block_layers and bench_sections for the whole campaign — "+
			"three preferences that have nothing to do with the deleted calendar",
			st.fromFiles)
	}
	if st.onDelete != "SET NULL" {
		t.Errorf("fk_calendar_active_cal is ON DELETE %q after replaying %v; want SET NULL. "+
			"RESTRICT/NO ACTION would make deleting a calendar FAIL for any viewer who had "+
			"selected it, which is a worse outcome than the bug", st.onDelete, st.fromFiles)
	}
	if !st.nullable {
		t.Errorf("calendar_active.calendar_id is still NOT NULL after replaying %v — "+
			"SET NULL cannot fire against a NOT NULL column, so the delete would error "+
			"instead of clearing the pointer", st.fromFiles)
	}
}

// TestActiveCalMigrationReplayIsSelfTesting is the mutation test for the reader
// above: a replay that cannot SEE a cascade would pass the assertions vacuously.
// It runs the same fold over a synthetic set that ends in CASCADE and over one
// that ends in SET NULL, and requires the two to differ.
func TestActiveCalMigrationReplayIsSelfTesting(t *testing.T) {
	fold := func(files map[string]string) activeCalFKState {
		mapfs := fstest.MapFS{}
		for name, body := range files {
			mapfs["migrations/"+name] = &fstest.MapFile{Data: []byte(body)}
		}
		return replayActiveCalMigrations(t, mapfs)
	}

	const create = `CREATE TABLE IF NOT EXISTS calendar_active (
		user_id VARCHAR(36) NOT NULL,
		calendar_id VARCHAR(36) NOT NULL,
		CONSTRAINT fk_calendar_active_cal
		  FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE
	);`
	const relax = `ALTER TABLE calendar_active MODIFY COLUMN calendar_id VARCHAR(36) NULL;
		ALTER TABLE calendar_active DROP FOREIGN KEY IF EXISTS fk_calendar_active_cal;
		ALTER TABLE calendar_active ADD CONSTRAINT fk_calendar_active_cal
		  FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE SET NULL;`

	before := fold(map[string]string{"006_x.up.sql": create})
	if !before.exists || before.onDelete != "CASCADE" || before.nullable {
		t.Fatalf("the replay cannot see the ORIGINAL cascade — it would pass the real test "+
			"vacuously. got %+v", before)
	}
	after := fold(map[string]string{"006_x.up.sql": create, "017_x.up.sql": relax})
	if !after.exists || after.onDelete != "SET NULL" || !after.nullable {
		t.Fatalf("the replay cannot see the RELAXATION. got %+v", after)
	}
}

// --- half 2: the reader survives a NULL -------------------------------------

// nullPointerConn answers one query with a single NULL column — exactly what
// MariaDB returns for a calendar_active row whose calendar was deleted under the
// new SET NULL constraint.
type nullPointerConn struct{}

func (nullPointerConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &oneNullRow{}, nil
}
func (nullPointerConn) Prepare(string) (driver.Stmt, error) { return nil, io.EOF }
func (nullPointerConn) Close() error                        { return nil }
func (nullPointerConn) Begin() (driver.Tx, error)           { return nil, io.EOF }

type oneNullRow struct{ done bool }

func (r *oneNullRow) Columns() []string { return []string{"calendar_id"} }
func (r *oneNullRow) Close() error      { return nil }
func (r *oneNullRow) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = nil // the NULL
	return nil
}

type nullPointerConnector struct{}

func (nullPointerConnector) Connect(context.Context) (driver.Conn, error) {
	return nullPointerConn{}, nil
}
func (nullPointerConnector) Driver() driver.Driver { return nullPointerDriver{} }

type nullPointerDriver struct{}

func (nullPointerDriver) Open(string) (driver.Conn, error) { return nil, io.EOF }

// TestGetActiveCalendarID_NullPointerReadsAsUnset is the reader regression.
//
// Migration 017 makes a NULL calendar_id reachable for the first time. Scanning
// a NULL into a plain `string` is a database/sql error, so a reader that was not
// updated alongside the schema turns every affected viewer's page into a 500 —
// trading a silent preference wipe for a hard failure. The answer must be "",
// which resolveActiveCalendar's ladder already reads as "fall back to the
// campaign default", the same as no row at all.
func TestGetActiveCalendarID_NullPointerReadsAsUnset(t *testing.T) {
	db := sql.OpenDB(nullPointerConnector{})
	t.Cleanup(func() { _ = db.Close() })
	repo := NewCalendarRepository(db)

	got, err := repo.GetActiveCalendarID(context.Background(), "user-1", "camp-1")
	if err != nil {
		t.Fatalf("a NULL calendar_id must read as unset, not as an error: %v\n"+
			"Migration 017's ON DELETE SET NULL makes this row reachable whenever a "+
			"calendar is deleted, so an error here is a 500 on every affected viewer's "+
			"page", err)
	}
	if got != "" {
		t.Errorf("a NULL calendar_id read as %q; want \"\" so resolveActiveCalendar falls "+
			"back to the campaign default exactly as it does for a viewer who has never "+
			"chosen one", got)
	}
}
