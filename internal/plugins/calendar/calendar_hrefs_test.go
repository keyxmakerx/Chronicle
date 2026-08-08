package calendar

// calendar_hrefs_test.go — C-CALV4-GAMEREADY §8 [GR-16].
//
// WHY THIS GUARD EXISTS. Two dead links shipped to real users, in two files, by
// two authors, both green in CI, both discovered only by a driven audit:
//
//	schedule_copy.go  ZoneHref: "/settings/profile"                  — 404
//	bench.go          "/campaigns/" + id + "/settings/members"       — 404
//
// Neither route has ever existed and there is no catch-all. The first was the
// ONLY repair offered in the state every new player starts in (users.timezone
// is nullable), so availability entry was dead on arrival; the second rendered
// once per zone-less roster member, in the PLAYER's DOM. These are the same bug
// twice, so a third is not a possibility — it is a schedule. This guard is what
// stops it.
//
// ── THE BLIND SPOT, NAMED RATHER THAN IMPLIED ────────────────────────────────
//
// It can only see hrefs it can EVALUATE STATICALLY. Specifically it reads:
//
//   - a string literal assigned to any field or variable whose name contains
//     "Href" (this is where schedule_copy.go's lived), and
//   - a return expression inside any function whose name contains "Href",
//     including a concatenation of literals and identifiers, which is
//     reconstructed as a route PATTERN with `:param` standing in for each
//     non-literal operand (this is where bench.go's lived), and
//   - a static `href="/…"` literal in any of the plugin's .templ files.
//
// Inside those it evaluates string literals, `+` concatenation, `fmt.Sprintf`
// (the format string alone: every verb becomes `:param`) and a `templ.SafeURL`
// conversion wrapping any of them.
//
// It CANNOT see an href assembled by a helper it does not recognise, built from
// a map or a slice, returned by another package, or joined from a value it
// cannot read (query-string builders are the common case here) — those are
// reported as UNRESOLVABLE in the test log and COUNTED, so the blind spot is
// visible rather than silent, but they are not failures. A guard that documents
// its blind spot is worth more than one that implies coverage it does not have.
//
// ── AND WHY THE ROUTE UNIVERSE IS REBUILT RATHER THAN READ STRAIGHT ──────────
//
// internal/wire/routes_snapshot.txt records the path literal AT THE CALL SITE,
// not the full URL: campaigns/routes.go registers `cg.GET("/members", …)` on a
// group declared as `e.Group("/campaigns/:id", …)`, so the snapshot line reads
// `GET /members`, and comparing an href against it directly would be comparing
// against half a path. The snapshot's own header names this as a deliberate
// Phase-2A simplification and points at "tracking variable assignments through
// the AST" as the fix.
//
// So: THE SNAPSHOT REMAINS THE AUTHORITY FOR WHICH ROUTES EXIST, and the AST
// supplies only the group prefix it does not record (hrefRouteUniverse). A
// snapshot line the AST cannot attribute to a group contributes its bare path,
// which widens the accepted set rather than inventing a failure — the second
// half of this guard's blind spot, and the reason a NEGATIVE result is a strong
// signal here while a positive one is merely "not obviously dead".

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// hrefRepoRoot resolves the repo root from this package's directory.
func hrefRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", "..", ".."))
}

// hrefFinding is one static href literal this guard could evaluate.
type hrefFinding struct {
	File string
	Line int
	Path string // a route PATTERN: identifiers already replaced by ":param".
}

// hrefRouteUniverse builds the set of FULL route paths.
//
// The snapshot is the AUTHORITY for which routes exist; the AST supplies only
// the group PREFIX the snapshot deliberately does not record (see its own
// header: "Echo group-prefix NOT resolved statically… Phase 2B can add prefix
// resolution by tracking variable assignments through the AST"). So this walks
// internal/ once, resolves `x := <recv>.Group("/lit", …)` chains to a prefix
// per local variable, and joins each `x.METHOD("/path", …)` call to its
// prefix — then keys those results by (METHOD, path-literal, file), which is
// exactly the snapshot's own tuple, and emits a full path ONLY for tuples the
// snapshot carries.
//
// A registration whose receiver is not a resolvable group variable (a route
// hung directly off the echo instance, or off a *echo.Group passed in as a
// function parameter) contributes its bare path — correct for the first case,
// and a widening for the second. Both are counted in the log.
func hrefRouteUniverse(t *testing.T, root string) []string {
	t.Helper()

	verbs := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "PATCH": true,
		"DELETE": true, "HEAD": true, "OPTIONS": true,
	}

	// full[method+"\t"+pathLit+"\t"+relFile] -> resolved full paths.
	full := map[string][]string{}

	err := filepath.Walk(filepath.Join(root, "internal"), func(path string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return werr
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)

		// Per-file prefix table, filled in source order — Go requires a group
		// variable to be assigned before it is used, so one forward pass is
		// enough for arbitrarily deep chains.
		prefix := map[string]string{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.AssignStmt:
				if len(v.Lhs) != 1 || len(v.Rhs) != 1 {
					return true
				}
				lhs, ok := v.Lhs[0].(*ast.Ident)
				if !ok {
					return true
				}
				call, ok := v.Rhs[0].(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel == nil || sel.Sel.Name != "Group" || len(call.Args) == 0 {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				seg, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					return true
				}
				parent := ""
				if recv, isIdent := sel.X.(*ast.Ident); isIdent {
					parent = prefix[recv.Name] // "" when the receiver is the echo instance
				}
				prefix[lhs.Name] = parent + seg
			case *ast.CallExpr:
				sel, ok := v.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel == nil || !verbs[sel.Sel.Name] || len(v.Args) == 0 {
					return true
				}
				lit, ok := v.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				p, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					return true
				}
				pre := ""
				if recv, isIdent := sel.X.(*ast.Ident); isIdent {
					pre = prefix[recv.Name]
				}
				key := sel.Sel.Name + "\t" + p + "\t" + rel
				full[key] = append(full[key], pre+p)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal/ for route prefixes: %v", err)
	}

	// THE SNAPSHOT DECIDES WHAT EXISTS. Every line of it contributes one or
	// more full paths; a line the AST could not attribute contributes its bare
	// path so the guard degrades toward permissive rather than toward a false
	// alarm.
	b, rerr := os.ReadFile(filepath.Join(root, "internal", "wire", "routes_snapshot.txt"))
	if rerr != nil {
		t.Fatalf("reading routes_snapshot.txt: %v", rerr)
	}
	var out []string
	lines, unattributed := 0, 0
	for _, line := range strings.Split(string(b), "\n") {
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		lines++
		if resolved, ok := full[line]; ok {
			out = append(out, resolved...)
			continue
		}
		unattributed++
		out = append(out, cols[1])
	}
	if lines < 100 {
		t.Fatalf("the route snapshot parsed to only %d entries — the format changed and this "+
			"guard would silently stop guarding", lines)
	}
	t.Logf("route universe: %d snapshot lines, %d full paths, %d line(s) the AST could not "+
		"attribute to a group (those contribute their bare path)", lines, len(out), unattributed)
	return out
}

// hrefSegmentsMatch compares a route pattern against a snapshot pattern. A
// segment beginning with ':' on EITHER side matches any single segment, which
// is what lets "/campaigns/:param/members" resolve against "/campaigns/:id" +
// "/members".
func hrefSegmentsMatch(got, want string) bool {
	g := strings.Split(strings.Trim(got, "/"), "/")
	w := strings.Split(strings.Trim(want, "/"), "/")
	if len(g) != len(w) {
		return false
	}
	for i := range g {
		if strings.HasPrefix(g[i], ":") || strings.HasPrefix(w[i], ":") {
			continue
		}
		if g[i] != w[i] {
			return false
		}
	}
	return true
}

// hrefPathExpr evaluates an expression to a route pattern, or "" when it cannot.
// A non-literal operand of a concatenation becomes ":param" — which is exactly
// how `"/campaigns/" + campaignID + "/members"` becomes a checkable pattern.
func hrefPathExpr(e ast.Expr, top bool) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		if err != nil {
			return "", false
		}
		return s, true
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		l, lok := hrefPathExpr(v.X, false)
		r, rok := hrefPathExpr(v.Y, false)
		if !lok {
			l, lok = ":param", true
		}
		if !rok {
			r, rok = ":param", true
		}
		if !lok || !rok {
			return "", false
		}
		return l + r, true
	case *ast.ParenExpr:
		return hrefPathExpr(v.X, top)
	case *ast.CallExpr:
		// `fmt.Sprintf("/campaigns/%s/calendar/v2/%s", …)` is how MOST of this
		// plugin's hrefs are built, so an evaluator that could not read one
		// would have a blind spot wide enough to swallow the class it exists
		// for. The format string alone is enough: every verb becomes ":param",
		// which is exactly the shape hrefSegmentsMatch compares.
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || len(v.Args) == 0 {
			return "", false
		}
		// `templ.SafeURL(x)` is a conversion, not a builder — unwrap it, or
		// every href in calendar_v2_helpers.go would sit in the blind spot.
		if sel.Sel.Name == "SafeURL" {
			return hrefPathExpr(v.Args[0], top)
		}
		if sel.Sel.Name != "Sprintf" {
			return "", false
		}
		if pkg, isIdent := sel.X.(*ast.Ident); !isIdent || pkg.Name != "fmt" {
			return "", false
		}
		lit, ok := v.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		format, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", false
		}
		return hrefVerbRe.ReplaceAllString(format, ":param"), true
	}
	return "", false
}

// hrefVerbRe matches the printf verbs these href builders actually use. It is
// deliberately narrow: a verb it does not know leaves a literal `%` in the
// path, which fails to resolve and surfaces as a finding rather than passing
// quietly.
var hrefVerbRe = regexp.MustCompile(`%[#+\- 0]*[0-9]*(?:\.[0-9]+)?[svdqt]`)

// hrefCollect walks the plugin's own .go files for evaluable href literals.
func hrefCollect(t *testing.T, root string) (found []hrefFinding, unresolvable []string) {
	t.Helper()
	dir := filepath.Join(root, "internal", "plugins", "calendar")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the plugin directory: %v", err)
	}

	record := func(rel string, fset *token.FileSet, pos token.Pos, e ast.Expr) {
		p, ok := hrefPathExpr(e, true)
		if !ok {
			unresolvable = append(unresolvable,
				rel+":"+strconv.Itoa(fset.Position(pos).Line))
			return
		}
		if !strings.HasPrefix(p, "/") {
			// A fragment, a query-only href or an empty "no control" value.
			return
		}
		found = append(found, hrefFinding{File: rel, Line: fset.Position(pos).Line, Path: p})
	}

	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_templ.go") {
			continue
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if perr != nil {
			t.Fatalf("parsing %s: %v", name, perr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.KeyValueExpr:
				// `ZoneHref: "/account"` in a composite literal.
				if k, ok := v.Key.(*ast.Ident); ok && strings.Contains(k.Name, "Href") {
					record(name, fset, v.Pos(), v.Value)
				}
			case *ast.AssignStmt:
				// `row.AskHref = …` / `href := …`.
				for i, lhs := range v.Lhs {
					if i >= len(v.Rhs) {
						break
					}
					if hrefTargetsAnHref(lhs) {
						record(name, fset, v.Pos(), v.Rhs[i])
					}
				}
			case *ast.FuncDecl:
				// Every `return` inside a function whose name names an href.
				if v.Name == nil || !strings.Contains(v.Name.Name, "Href") || v.Body == nil {
					return true
				}
				ast.Inspect(v.Body, func(m ast.Node) bool {
					ret, ok := m.(*ast.ReturnStmt)
					if !ok || len(ret.Results) == 0 {
						return true
					}
					for _, r := range ret.Results {
						if lit, isLit := r.(*ast.BasicLit); isLit && lit.Value == `""` {
							continue // the explicit "no control" return
						}
						record(name, fset, ret.Pos(), r)
					}
					return true
				})
			}
			return true
		})
	}

	// The .templ half. Every static `href="/…"` literal — none exist today
	// (every calendar template computes its hrefs through templ.SafeURL), and
	// that absence is exactly why the Go half above carries the weight.
	reTemplHref := regexp.MustCompile(`href="(/[^"{]*)"`)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".templ") {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(dir, ent.Name())) //nolint:gosec // test-time read
		if rerr != nil {
			continue
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, m := range reTemplHref.FindAllStringSubmatch(line, -1) {
				found = append(found, hrefFinding{File: ent.Name(), Line: i + 1, Path: m[1]})
			}
		}
	}
	return found, unresolvable
}

// hrefTargetsAnHref reports whether an assignment's left-hand side names an href.
func hrefTargetsAnHref(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return strings.Contains(v.Name, "Href") || strings.Contains(v.Name, "href")
	case *ast.SelectorExpr:
		return v.Sel != nil && strings.Contains(v.Sel.Name, "Href")
	}
	return false
}

// TestCalendarHrefs_AllResolve asserts every statically-evaluable href the
// calendar plugin emits resolves against internal/wire/routes_snapshot.txt.
//
// MUTATION-TESTED: restoring either "/settings/profile" (schedule_copy.go) or
// "/settings/members" (bench.go) turns this red, and both were demonstrated
// before this guard was reported. See TestCalendarHrefs_TheGuardCanFail, which
// pins that capability in the suite itself rather than in a report nobody runs.
func TestCalendarHrefs_AllResolve(t *testing.T) {
	root := hrefRepoRoot(t)
	universe := hrefRouteUniverse(t, root)
	found, unresolvable := hrefCollect(t, root)

	if len(found) == 0 {
		t.Fatal("this guard found NO hrefs at all — it has stopped guarding, which is worse " +
			"than the bug it exists for")
	}
	t.Logf("evaluated %d static href literal(s); %d expression(s) were not statically "+
		"evaluable and are this guard's NAMED blind spot: %v",
		len(found), len(unresolvable), unresolvable)

	for _, f := range found {
		if !hrefResolves(f.Path, universe) {
			t.Errorf("%s:%d emits href %q, which resolves against NO route in "+
				"internal/wire/routes_snapshot.txt. There is no catch-all, so this 404s for "+
				"whoever it renders to.", f.File, f.Line, f.Path)
		}
	}
}

// hrefResolves reports whether an href pattern matches any full route path.
func hrefResolves(path string, universe []string) bool {
	// Query strings and fragments are not part of the route.
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	if path == "" || path == "/" {
		return true
	}
	for _, want := range universe {
		if hrefSegmentsMatch(path, want) {
			return true
		}
	}
	return false
}

// TestCalendarHrefs_TheGuardCanFail is the mutation test, run in-process so the
// capability is proven on every CI run rather than asserted once in a report.
// Both shipped defects are replayed as literals; both must be judged dead.
func TestCalendarHrefs_TheGuardCanFail(t *testing.T) {
	root := hrefRepoRoot(t)
	universe := hrefRouteUniverse(t, root)

	for _, dead := range []string{
		"/settings/profile",                  // [GR-14]'s defect, verbatim
		"/campaigns/:param/settings/members", // [GR-15]'s defect, as the pattern the guard builds
	} {
		if hrefResolves(dead, universe) {
			t.Errorf("the guard judged %q REACHABLE; it is one of the two 404s this slice "+
				"fixed, so a guard that accepts it cannot catch the third", dead)
		}
	}
	// …and the two repairs must be judged live, or the guard is merely strict.
	for _, live := range []string{
		"/account",
		"/campaigns/:param/members",
	} {
		if !hrefResolves(live, universe) {
			t.Errorf("the guard judged the repaired href %q unreachable; it exists in the "+
				"snapshot and this guard would block the fix", live)
		}
	}
}
