// scan_contract_test.go is the Scan-arity contract guard for every
// repository in this package.
//
// WHY THIS EXISTS: a column-list constant (calendarCols, eventCols, ...) and
// the Scan destination list that consumes it drift apart whenever a
// migration adds a column to one side and not the other. Nothing in this
// package executes real SQL in a unit test, so no ordinary test could
// observe the mismatch: it surfaces only against a live database, as
// "sql: expected N destination arguments in Scan, not M". This guard closes
// that hole without one: it parses the owning source file with go/parser and
// compares the arity of the SELECT list (the constant's own value, read
// directly since this test lives in the same package) to the arity of the
// Scan call each function makes.
package calendar

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// countCols counts the top-level comma-separated SELECT expressions in a
// column-list string, splitting on top-level commas only so a
// multi-argument COALESCE(...) counts as one column rather than two.
func countCols(cols string) int {
	depth, n := 0, 1
	for _, r := range cols {
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
	return n
}

// scanArity parses filename (relative to this package dir, the same
// directory `go test` runs in) and returns the effective destination count
// of the first `.Scan(...)` call found inside the function named fnName.
func scanArity(t *testing.T, filename, fnName string) int {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", filename, err)
	}

	arity := -1
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
			// Only the FIRST Scan call in the function body is the row scan;
			// a helper that scans more than once would need this revisited.
			if arity == -1 {
				arity = countScanArgs(t, call)
			}
			return true
		})
		return false
	})

	if arity == -1 {
		t.Fatalf("no .Scan(...) call found in %s (%s): has it been restructured?", fnName, filename)
	}
	return arity
}

// countScanArgs counts a Scan call's effective destination count. Most call
// sites pass N literal arguments (len(call.Args)). Two shapes in this
// package instead spread a shared *Dests(&v) helper's returned []any, so two
// Scan sites can share one literal destination list instead of hand-copying
// it (see eventDests/eraDests): Scan(xDests(&v)...) and, for one trailing
// caller-supplied extra, Scan(append(xDests(&v), extra)...). Either spread
// shows as ONE argument node to the AST regardless of the helper's real
// element count, so this resolves back to that count instead of returning 1.
func countScanArgs(t *testing.T, call *ast.CallExpr) int {
	t.Helper()
	if len(call.Args) != 1 || !call.Ellipsis.IsValid() {
		return len(call.Args)
	}
	spread := call.Args[0]
	extra := 0
	if appendCall, ok := spread.(*ast.CallExpr); ok {
		if ident, ok := appendCall.Fun.(*ast.Ident); ok && ident.Name == "append" && len(appendCall.Args) >= 1 {
			extra = len(appendCall.Args) - 1
			spread = appendCall.Args[0]
		}
	}
	destsCall, ok := spread.(*ast.CallExpr)
	if !ok {
		t.Fatalf("Scan(...) spreads an expression scanArity doesn't recognize; teach it this shape or stop spreading")
	}
	fnIdent, ok := destsCall.Fun.(*ast.Ident)
	if !ok {
		t.Fatalf("Scan(...) spreads a non-identifier call; teach scanArity this shape")
	}
	return destsLiteralLen(t, fnIdent.Name) + extra
}

// destsLiteralLen finds the package-level function fnName (in any .go file
// in this package directory, since a *Dests helper and the Scan call
// spreading it can live in different files of the same package) and returns
// the element count of the []any{...} literal in its return statement.
func destsLiteralLen(t *testing.T, fnName string) int {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("globbing package dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, path := range paths {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Name == nil || fd.Name.Name != fnName || fd.Body == nil {
				continue
			}
			for _, stmt := range fd.Body.List {
				ret, ok := stmt.(*ast.ReturnStmt)
				if !ok || len(ret.Results) != 1 {
					continue
				}
				if lit, ok := ret.Results[0].(*ast.CompositeLit); ok {
					return len(lit.Elts)
				}
			}
		}
	}
	t.Fatalf("could not find a []any{...} return in %s to count its destinations", fnName)
	return 0
}

// scanSite is one column-list-constant / scan-function pairing to check.
// extra counts destinations the Scan call reads beyond the column-list
// constant itself (e.g. EventsForEntity's trailing participation_role).
type scanSite struct {
	name  string
	cols  string
	file  string
	fn    string
	extra int
}

// TestScanArityMatchesColumnLists pins SELECT arity == Scan arity for every
// column-list/scan pairing in this package. Add an entry whenever a new one
// is introduced: that is exactly the mistake this guard exists to catch
// mechanically instead of in production.
func TestScanArityMatchesColumnLists(t *testing.T) {
	sites := []scanSite{
		{"scanCalendar", calendarCols, "repository.go", "scanCalendar", 0},
		{"scanMonth", monthCols, "repository.go", "scanMonth", 0},
		{"scanWeekday", weekdayCols, "repository.go", "scanWeekday", 0},
		{"scanMoon", moonCols, "repository.go", "scanMoon", 0},
		{"scanSeason", seasonCols, "repository.go", "scanSeason", 0},
		{"scanEra", eraCols, "repository.go", "scanEra", 0},
		{"scanCycle", cycleCols, "repository.go", "scanCycle", 0},
		{"scanCycleEntry", cycleEntryCols, "repository.go", "scanCycleEntry", 0},
		{"scanFestival", festivalCols, "repository.go", "scanFestival", 0},
		{"scanWeather", weatherCols, "weather_repository.go", "scanWeather", 0},
		{"scanEventKind", eventKindCols, "event_kind_repository.go", "scanEventKind", 0},
		{"scanEventRow", eventCols, "event_repository.go", "scanEventRow", 0},
		{"scanEntityTieRefs", entityTieWithRoleCols, "entity_ties_repository.go", "scanEntityTieRefs", 0},
		// EntitiesForCalendar has no role column (a calendar-scope tie can
		// span several roles across an entity's events/eras), so it scans
		// inline rather than through scanEntityTieRefs.
		{"EntitiesForCalendar", entityTieCols, "entity_ties_repository.go", "EntitiesForCalendar", 0},
		// EventsForEntity's Scan reuses scanEventRow's own destination list
		// (eventDests) plus one trailing extra (l.participation_role): see
		// scanEventRowWithExtra's doc comment.
		{"scanEventRowWithExtra", eventCols, "entity_ties_repository.go", "scanEventRowWithExtra", 1},
		// ErasForEntity's Scan reuses scanEra's own destination list (eraDests)
		// plus one trailing extra (l.participation_role).
		{"scanEraWithExtra", eraColsQualified, "entity_ties_repository.go", "scanEraWithExtra", 1},
	}

	for _, site := range sites {
		t.Run(site.name, func(t *testing.T) {
			want := countCols(site.cols) + site.extra
			got := scanArity(t, site.file, site.fn)
			if got != want {
				t.Errorf("%s (%s) scans %d destinations but its column list selects %d columns"+
					" (%d + %d extra).\n"+
					"Every query through this path will fail at runtime with "+
					"\"sql: expected %d destination arguments in Scan, not %d\".\n"+
					"Add the new column to BOTH the column-list constant and this Scan list, in the same position.",
					site.fn, site.file, got, want, want-site.extra, site.extra, want, got)
			}
		})
	}
}

// TestEventColsIncludesNewV5Columns pins that the three columns this package
// added to calendar_events are actually read back onto the aggregate: a
// column present in the table that no query selects would make Event.KindID/
// Announced/Payload silently and permanently empty.
func TestEventColsIncludesNewV5Columns(t *testing.T) {
	for _, want := range []string{"e.kind_id", "e.announced", "e.payload"} {
		if !strings.Contains(eventCols, want) {
			t.Errorf("eventCols must select %s so the matching Event field is populated on read", want)
		}
	}
}
