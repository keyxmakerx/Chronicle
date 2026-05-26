// show_banner_route_test.go pins the owner-only access control on
// /foundry-vtt/show-banner-fragment via AST inspection of the route
// registration. Permanent regression guard added by NW-2.2 Chunk
// D2-cleanup: the inline role gate in campaigns.Handler that
// previously double-checked owner-only was removed (route-level
// requireOwner middleware enforces it). This test makes the
// route-level enforcement load-bearing — if a future contributor
// drops the requireOwner argument from the show-banner-fragment
// registration, this test fails with a clear pointer.
//
// Per cordinator/decisions/2026-05-21-core-tenets.md §T-B1 + §T-O2;
// cordinator/reports/chronicle/2026-05-26-c-d2-cleanup-verification.md
// (the role-gate-removal-safety verification).
//
// Same AST-assertion shape as foundry_public_ratelimit_test.go's
// middleware-pin pattern (PR #339); generalizes to other security-
// sensitive routes.

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestShowBannerFragmentRoute_HasOwnerGate pins that the
// /foundry-vtt/show-banner-fragment route registration includes the
// requireOwner middleware argument. The campaigns-side inline role
// gate that previously double-checked owner was removed in D2-cleanup;
// this test makes the route-level gate the load-bearing enforcement.
func TestShowBannerFragmentRoute_HasOwnerGate(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "plugins", fvttDirName, "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}

	fn := findFuncDecl(file, "RegisterOwnerRoutes")
	if fn == nil {
		t.Fatalf("RegisterOwnerRoutes not found in %s", routesPath)
	}
	if fn.Body == nil {
		t.Fatalf("RegisterOwnerRoutes has no body")
	}

	// Find the GET call whose first arg is a string literal
	// containing "show-banner-fragment". Assert its arguments
	// include `requireOwner` (the middleware param name).
	var found bool
	var hasRequireOwner bool
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "GET" {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if !strings.Contains(lit.Value, "show-banner-fragment") {
			return true
		}
		found = true
		// Check remaining args for an Ident named "requireOwner".
		for _, arg := range call.Args[1:] {
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == "requireOwner" {
				hasRequireOwner = true
				return false
			}
		}
		return false
	})

	if !found {
		t.Fatalf("could not locate the show-banner-fragment GET call in RegisterOwnerRoutes — route may have been renamed; locate by content and update this test alongside the rename")
	}
	if !hasRequireOwner {
		t.Errorf("show-banner-fragment route registration is missing the requireOwner middleware argument.\n"+
			"This breaks the route-level access control that was made load-bearing in NW-2.2 Chunk D2-cleanup\n"+
			"(the inline campaigns-side role gate at handler.go:421 was removed because requireOwner\n"+
			"enforces owner-only at the route layer).\n"+
			"Restore the requireOwner argument on the cg.GET call in %s.", routesPath)
	}
}
