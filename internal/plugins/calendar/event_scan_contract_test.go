package calendar

// Event column/scan contract guard (C-CAL-RSVP-P1 Step-0 finding, widened by
// C-CALV4-TIEFIX-PB to cover a third consumer).
//
// WHY THIS EXISTS: `eventCols` and the Scan destination lists that consume it
// drift apart whenever a migration adds a column to one side and not the
// other. It has happened twice:
//   - scanEvents (repository.go) was missing &evt.RecurrenceDayOfWeek after
//     migration 011 added recurrence_day_of_week to eventCols: 37 columns
//     against 36 destinations, so EVERY event LIST query (month/week/day/
//     range/upcoming/search/ledger) failed at runtime with "sql: expected 37
//     destination arguments in Scan, not 36" while the single-row GetEvent
//     (its own correct inline scan) kept working.
//   - EventsForEntity (entity_ties_repository.go) was missing BOTH
//     &evt.RecurrenceDayOfWeek and &evt.CollectRSVPs (C-CALV4-TIEFIX-PB
//     Step-0 finding): 39 columns (eventCols' 38 + l.participation_role)
//     against 37 destinations. The scanEvents fix never touched this file, so
//     the same drift reappeared in a third place — this guard now covers it.
//
// It survives because nothing in this repository executes real SQL — there is
// no sqlmock/testify/dockertest in go.mod — so no unit test could observe the
// mismatch. This guard closes that hole WITHOUT a database: it parses the
// owning source file with go/parser and compares the arity of the SELECT list
// to the arity of each Scan call. Any future column added to one side and not
// the other fails here instead of in production.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// countEventCols returns the number of SELECT expressions in eventCols.
//
// Splits on top-level commas only, so a multi-argument COALESCE(...) counts as
// one column rather than two.
func countEventCols(t *testing.T) int {
	t.Helper()
	depth, n := 0, 1
	for _, r := range eventCols {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				n++
			}
		}
	}
	if depth != 0 {
		t.Fatalf("eventCols has unbalanced parentheses")
	}
	return n
}

// scanArity parses the named source file (relative to this package dir, same
// convention `go test` already runs under) and returns the number of
// arguments passed to the `.Scan(...)` call inside the named function.
func scanArity(t *testing.T, filename, fnName string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", filename, err)
	}

	var arity = -1
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name == nil || fd.Name.Name != fnName {
			return true
		}
		ast.Inspect(fd, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "Scan" {
				return true
			}
			// Only the FIRST Scan in the function body is the event scan; a
			// helper that scanned twice would need this guard revisited.
			if arity == -1 {
				arity = len(call.Args)
			}
			return true
		})
		return false
	})

	if arity == -1 {
		t.Fatalf("no .Scan(...) call found in %s (%s) — has it been restructured?", fnName, filename)
	}
	return arity
}

// eventScanSites enumerates every function that owns an INDEPENDENT Scan
// destination list built against eventCols — i.e. every function with its own
// inline `.Scan(...)` call, as opposed to one that just delegates to
// scanEvents (every List* query in repository.go: ListEventsForMonth,
// ListEventsForYear, ListAllEvents, ListEventsForDateRange,
// ListEventsForEntity, ListUpcomingEvents, SearchEvents — none of those need
// their own entry here, since scanEvents already covers their Scan call).
//
// Add a new entry whenever a new such consumer is introduced — that is
// exactly the mistake this guard exists to catch mechanically instead of at
// runtime in production (C-CALV4-TIEFIX-PB found the third one by hand).
//
// extra counts any additional columns the SELECT appends alongside eventCols
// itself (EventsForEntity also selects l.participation_role, so its Scan list
// is one longer than eventCols' own arity).
var eventScanSites = []struct {
	file  string
	fn    string
	extra int
}{
	{"repository.go", "GetEvent", 0},
	{"repository.go", "scanEvents", 0},
	{"entity_ties_repository.go", "EventsForEntity", 1},
}

// TestEventColsMatchScanDestinations pins the invariant every event read path
// depends on: SELECT arity == Scan arity, for each site in eventScanSites.
func TestEventColsMatchScanDestinations(t *testing.T) {
	cols := countEventCols(t)

	for _, site := range eventScanSites {
		got := scanArity(t, site.file, site.fn)
		want := cols + site.extra
		if got != want {
			t.Errorf("%s (%s) scans %d destinations but eventCols selects %d columns"+
				" (%d + %d extra).\n"+
				"Every event query through this path will fail at runtime with "+
				"\"sql: expected %d destination arguments in Scan, not %d\".\n"+
				"Add the new column to BOTH eventCols and this Scan list, in the same position.",
				site.fn, site.file, got, want, cols, site.extra, want, got)
		}
	}
}

// TestEventColsIncludesCollectRSVPs pins that the RSVP opt-in is actually read
// back onto the aggregate — a column in the table that no query selects would
// make the drawer toggle look like it silently forgets its state.
func TestEventColsIncludesCollectRSVPs(t *testing.T) {
	if !strings.Contains(eventCols, "e.collect_rsvps") {
		t.Error("eventCols must select e.collect_rsvps so Event.CollectRSVPs is populated on read")
	}
}
