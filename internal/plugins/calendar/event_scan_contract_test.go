package calendar

// Event column/scan contract guard (C-CAL-RSVP-P1 Step-0 finding).
//
// WHY THIS EXISTS: `eventCols` and the two Scan destination lists that consume
// it (GetEvent's inline scan and scanEvents) drifted apart when migration 011
// added recurrence_day_of_week to the column list but not to scanEvents. The
// result was 37 columns against 36 destinations, so EVERY event LIST query
// (month/week/day/range/upcoming/search/ledger) failed at runtime with
// "sql: expected 37 destination arguments in Scan, not 36" while the
// single-row GetEvent kept working.
//
// It survived because nothing in this repository executes real SQL — there is
// no sqlmock/testify/dockertest in go.mod — so no unit test could observe the
// mismatch. This guard closes that hole WITHOUT a database: it parses
// repository.go with go/parser and compares the arity of the SELECT list to the
// arity of each Scan call. Any future column added to one side and not the
// other fails here instead of in production.

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

// scanArity parses repository.go and returns the number of arguments passed to
// the `.Scan(...)` call inside the named function.
func scanArity(t *testing.T, fnName string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "repository.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing repository.go: %v", err)
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
		t.Fatalf("no .Scan(...) call found in %s — has repository.go been restructured?", fnName)
	}
	return arity
}

// TestEventColsMatchScanDestinations pins the invariant both event read paths
// depend on: SELECT arity == Scan arity, for the row scan AND the list scan.
func TestEventColsMatchScanDestinations(t *testing.T) {
	cols := countEventCols(t)

	for _, fn := range []string{"GetEvent", "scanEvents"} {
		got := scanArity(t, fn)
		if got != cols {
			t.Errorf("%s scans %d destinations but eventCols selects %d columns.\n"+
				"Every event query through this path will fail at runtime with "+
				"\"sql: expected %d destination arguments in Scan, not %d\".\n"+
				"Add the new column to BOTH eventCols and this Scan list, in the same position.",
				fn, got, cols, cols, got)
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
