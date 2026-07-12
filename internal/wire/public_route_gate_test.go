// Package wire — public-route gate sweep.
//
// C-PUBLIC-VIEW-FIX-R2 (per cordinator/dispatches/chronicle/C-PUBLIC-VIEW-FIX-R2.md):
// #522 restored public-campaign viewing by swapping 51 routes from
// RequireRole(RolePlayer) to RequireViewAccess(). Its own regression guard,
// however, only pinned 2 of those 51 routes — reverting timeline/routes.go:71
// back to RequireRole kept the whole suite green. This test closes that gap with
// a wire-level AST sweep: EVERY route registered on a group built with
// campaigns.AllowPublicCampaignAccess(...) must carry campaigns.RequireViewAccess()
// and must NOT carry campaigns.RequireRole(...). A public route that loses its
// view gate (or regains a role gate) fails here.
//
// WHY an AST sweep and not the runtime router: same rationale as
// wire_contract_test.go — constructing the full App needs a real DB/Redis. AST
// extraction is sufficient because Chronicle registers every route with a literal
// `<group>.METHOD("path", handler, middleware...)` call.
//
// Marker choice: the sweep keys on the AllowPublicCampaignAccess argument to
// e.Group(...), NOT on the group variable name (`pub` today). That is drift-proof:
// renaming the variable, or adding a new public group under a different name, is
// still caught. Group tracking is scoped per-FuncDecl so a file that opens both an
// authenticated `cg` group and a public `pub` group (or reuses the name `pub`
// across two functions, as maps/routes.go does) is handled correctly.
//
// C-ENTITY-VIS-PARITY (ride-along 4a) hardens the sweep against three evasions the
// per-route scan above could not see:
//   - a RequireRole passed as GROUP-LEVEL middleware to e.Group(...) alongside
//     AllowPublicCampaignAccess (applies to every route in the group, yet each
//     route's own args look clean);
//   - a RequireRole added to a public group via a later <group>.Use(...) call;
//   - a route registered through a NON-Ident receiver rooted at a public group
//     (e.g. a chained sub-group `pub.Group("/x").GET(...)`), whose view gate the
//     static per-route scan silently skipped. Such a registration now fails: its
//     gate cannot be statically verified, so it must fail closed here.
//
// Cites: cordinator/decisions/2026-05-21-core-tenets.md §T-B1 (security-first;
// auth-surface drift is a P0), §T-O2 (wire-contract integrity);
// cordinator/reports/coordinator/2026-07-11-r13-post-merge-review.md §2 +
// merge-gate addendum; cordinator/dispatches/chronicle/C-ENTITY-VIS-PARITY.md §4a.
package wire

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	publicGroupMarker = "AllowPublicCampaignAccess"
	viewAccessMarker  = "RequireViewAccess"
	requireRoleMarker = "RequireRole"
)

// publicRouteViolation is one route on a public group that fails the invariant.
type publicRouteViolation struct {
	File   string
	Line   int
	Method string
	Path   string
	Reason string
}

func (v publicRouteViolation) String() string {
	return fmt.Sprintf("%s:%d %s %q — %s", v.File, v.Line, v.Method, v.Path, v.Reason)
}

// TestPublicRoutesCarryViewAccess asserts the public-campaign auth invariant
// across every route file under internal/: routes on an AllowPublicCampaignAccess
// group carry RequireViewAccess and never RequireRole.
func TestPublicRoutesCarryViewAccess(t *testing.T) {
	root := repoRoot(t)
	internal := filepath.Join(root, "internal")

	var violations []publicRouteViolation
	var publicRouteCount int

	err := filepath.Walk(internal, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			// Non-compilable files are the build job's problem, not this test's.
			return nil
		}
		relPath, _ := filepath.Rel(root, path)

		// Scope group tracking per function so a reused variable name (maps/
		// routes.go opens a `pub` group in two different functions) can't leak
		// between functions.
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			publicGroups, groupViolations := collectPublicGroupVars(fset, fn, relPath)
			violations = append(violations, groupViolations...)
			if len(publicGroups) == 0 {
				continue
			}
			n, v := checkRoutesOnGroups(fset, fn, publicGroups, relPath)
			publicRouteCount += n
			violations = append(violations, v...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internal, err)
	}

	// Sanity: the sweep must actually be finding the public surface. If this
	// drops to zero the marker or the walk has silently broken.
	if publicRouteCount == 0 {
		t.Fatal("public-route sweep found ZERO routes on AllowPublicCampaignAccess " +
			"groups — the marker or walk is broken; this test would pass vacuously")
	}

	if len(violations) > 0 {
		sort.Slice(violations, func(i, j int) bool { return violations[i].String() < violations[j].String() })
		var b strings.Builder
		fmt.Fprintf(&b, "public-route auth-gate drift: %d route(s) on an %s group "+
			"violate the invariant (must carry %s, must not carry %s):\n\n",
			len(violations), publicGroupMarker, viewAccessMarker, requireRoleMarker)
		for _, v := range violations {
			b.WriteString("  ✗ " + v.String() + "\n")
		}
		b.WriteString("\nEvery public-campaign route must gate on campaigns.RequireViewAccess()\n")
		b.WriteString("(anon visitors pass only on a public campaign) and never on\n")
		b.WriteString("campaigns.RequireRole(...) (which 403s anon on a public campaign).\n")
		b.WriteString("Per cordinator/dispatches/chronicle/C-PUBLIC-VIEW-FIX-R2.md.\n")
		t.Fatal(b.String())
	}
}

// TestPublicRouteGate_CatchesEvasions proves the 4a extensions have teeth: it
// feeds a synthetic routes file — one func per evasion plus a clean control —
// through the SAME collectPublicGroupVars + checkRoutesOnGroups pipeline the real
// sweep uses, and asserts each evasion is flagged and the clean group is not.
// Without this, the extension could silently pass vacuously (the real codebase
// has none of these patterns today, so the sweep alone can't demonstrate it
// catches them).
func TestPublicRouteGate_CatchesEvasions(t *testing.T) {
	// Referenced identifiers need not resolve — ParseFile does no type-checking.
	const src = `package routes
func GroupLevelRole(e *echo.Echo) {
	pub := e.Group("/campaigns/:id", auth.OptionalAuth(s), campaigns.AllowPublicCampaignAccess(s), campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/x", h.X, campaigns.RequireViewAccess())
}
func UseRole(e *echo.Echo) {
	pub := e.Group("/campaigns/:id", campaigns.AllowPublicCampaignAccess(s))
	pub.Use(campaigns.RequireRole(campaigns.RolePlayer))
	pub.GET("/y", h.Y, campaigns.RequireViewAccess())
}
func SubGroupReceiver(e *echo.Echo) {
	pub := e.Group("/campaigns/:id", campaigns.AllowPublicCampaignAccess(s))
	pub.Group("/sub").GET("/z", h.Z, campaigns.RequireViewAccess())
}
func Clean(e *echo.Echo) {
	pub := e.Group("/campaigns/:id", campaigns.AllowPublicCampaignAccess(s))
	pub.GET("/ok", h.OK, campaigns.RequireViewAccess())
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "synthetic_routes.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse synthetic source: %v", err)
	}

	byFunc := make(map[string][]publicRouteViolation)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		groups, groupViolations := collectPublicGroupVars(fset, fn, "synthetic_routes.go")
		vs := append([]publicRouteViolation(nil), groupViolations...)
		if len(groups) > 0 {
			_, routeViolations := checkRoutesOnGroups(fset, fn, groups, "synthetic_routes.go")
			vs = append(vs, routeViolations...)
		}
		byFunc[fn.Name.Name] = vs
	}

	// Each evasion func must produce exactly one violation of the expected shape.
	cases := []struct {
		fn           string
		wantMethod   string
		reasonSubstr string
	}{
		{"GroupLevelRole", "Group", "group-level middleware"},
		{"UseRole", "Use", ".Use(...)"},
		{"SubGroupReceiver", "GET", "non-Ident receiver"},
	}
	for _, tc := range cases {
		vs := byFunc[tc.fn]
		if len(vs) != 1 {
			t.Errorf("%s: got %d violations, want 1: %+v", tc.fn, len(vs), vs)
			continue
		}
		if vs[0].Method != tc.wantMethod || !strings.Contains(vs[0].Reason, tc.reasonSubstr) {
			t.Errorf("%s: violation = {Method:%q Reason:%q}, want Method %q containing %q",
				tc.fn, vs[0].Method, vs[0].Reason, tc.wantMethod, tc.reasonSubstr)
		}
	}

	// The clean control must produce no violation — no over-flagging.
	if vs := byFunc["Clean"]; len(vs) != 0 {
		t.Errorf("Clean public group over-flagged: %+v", vs)
	}
}

// collectPublicGroupVars returns the set of local variable names in fn that are
// assigned an e.Group(...) whose middleware arguments include a call to
// AllowPublicCampaignAccess. Handles both `pub := e.Group(...)` (define) and
// `pub = e.Group(...)` (assign). It also returns a violation for any public
// group whose e.Group(...) constructor ALSO carries a RequireRole middleware arg
// — a group-level RequireRole 403s anon on every route in the group while each
// route's own args look clean, so the per-route scan cannot see it (4a).
func collectPublicGroupVars(fset *token.FileSet, fn *ast.FuncDecl, relPath string) (map[string]bool, []publicRouteViolation) {
	groups := make(map[string]bool)
	var violations []publicRouteViolation
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		var lhs []ast.Expr
		var rhs []ast.Expr
		switch s := n.(type) {
		case *ast.AssignStmt:
			lhs, rhs = s.Lhs, s.Rhs
		default:
			return true
		}
		if len(lhs) != 1 || len(rhs) != 1 {
			return true
		}
		ident, ok := lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Group" {
			return true
		}
		// Scan the group's middleware args for the public marker and for a
		// group-level RequireRole (both in the same e.Group(...) call).
		isPublic := false
		hasRole := false
		for _, arg := range call.Args {
			argCall, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			name := exprText(argCall.Fun)
			if strings.HasSuffix(name, publicGroupMarker) {
				isPublic = true
			}
			if strings.HasSuffix(name, requireRoleMarker) {
				hasRole = true
			}
		}
		if isPublic {
			groups[ident.Name] = true
			if hasRole {
				violations = append(violations, publicRouteViolation{
					File: relPath, Line: fset.Position(call.Pos()).Line, Method: "Group", Path: ident.Name,
					Reason: "public group constructed with " + requireRoleMarker +
						" group-level middleware (403s anon on every route in the group)",
				})
			}
		}
		return true
	})
	return groups, violations
}

// checkRoutesOnGroups walks fn for route registrations (`<group>.METHOD(...)`)
// whose receiver is one of the public groups, and returns the count plus any
// invariant violations. Beyond the per-route arg scan, it also flags (4a): a
// RequireRole added to a public group via `<group>.Use(...)`, and any route
// registered through a non-Ident receiver rooted at a public group (e.g. a
// chained sub-group), whose gate cannot be statically verified.
func checkRoutesOnGroups(fset *token.FileSet, fn *ast.FuncDecl, publicGroups map[string]bool, relPath string) (int, []publicRouteViolation) {
	var count int
	var violations []publicRouteViolation

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Group-level middleware injected after construction: a public group's
		// own .Use(RequireRole(...)) 403s anon on every route it carries.
		if sel.Sel.Name == "Use" {
			if recv, ok := sel.X.(*ast.Ident); ok && publicGroups[recv.Name] {
				for _, arg := range call.Args {
					argCall, ok := arg.(*ast.CallExpr)
					if !ok {
						continue
					}
					if strings.HasSuffix(exprText(argCall.Fun), requireRoleMarker) {
						violations = append(violations, publicRouteViolation{
							File: relPath, Line: fset.Position(call.Pos()).Line, Method: "Use", Path: recv.Name,
							Reason: "public group gains " + requireRoleMarker + " via .Use(...) (403s anon on every route in the group)",
						})
					}
				}
			}
			return true
		}

		if !httpVerbs[sel.Sel.Name] {
			return true
		}

		// A route whose receiver is NOT a bare ident but is rooted at a public
		// group (e.g. `pub.Group("/x").GET(...)`) escaped the per-route scan
		// below. Its view gate cannot be statically verified — fail closed.
		if _, isIdent := sel.X.(*ast.Ident); !isIdent {
			if root := rootIdent(sel.X); root != nil && publicGroups[root.Name] {
				count++
				violations = append(violations, publicRouteViolation{
					File: relPath, Line: fset.Position(call.Pos()).Line, Method: sel.Sel.Name, Path: "<non-ident receiver>",
					Reason: "route registered through a non-Ident receiver rooted at public group " + root.Name +
						" (e.g. a chained sub-group); its " + viewAccessMarker +
						" gate cannot be statically verified — register directly on the group var",
				})
			}
			return true
		}

		recv := sel.X.(*ast.Ident)
		if !publicGroups[recv.Name] {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}

		count++

		path := "<non-literal>"
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if unq, err := strconv.Unquote(lit.Value); err == nil {
				path = unq
			}
		}

		hasViewAccess := false
		hasRequireRole := false
		for _, arg := range call.Args[1:] {
			argCall, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			name := exprText(argCall.Fun)
			if strings.HasSuffix(name, viewAccessMarker) {
				hasViewAccess = true
			}
			if strings.HasSuffix(name, requireRoleMarker) {
				hasRequireRole = true
			}
		}

		line := fset.Position(call.Pos()).Line
		if hasRequireRole {
			violations = append(violations, publicRouteViolation{
				File: relPath, Line: line, Method: sel.Sel.Name, Path: path,
				Reason: "carries " + requireRoleMarker + " on a public group (403s anon)",
			})
		}
		if !hasViewAccess {
			violations = append(violations, publicRouteViolation{
				File: relPath, Line: line, Method: sel.Sel.Name, Path: path,
				Reason: "missing " + viewAccessMarker + " gate",
			})
		}
		return true
	})

	return count, violations
}

// rootIdent walks a receiver expression down through selector and call chains to
// its leftmost identifier (e.g. `pub.Group("/x").GET`'s receiver `pub.Group("/x")`
// roots at `pub`). Returns nil when the chain does not bottom out in an ident.
func rootIdent(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x
		case *ast.SelectorExpr:
			e = x.X
		case *ast.CallExpr:
			e = x.Fun
		case *ast.IndexExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil
		}
	}
}
