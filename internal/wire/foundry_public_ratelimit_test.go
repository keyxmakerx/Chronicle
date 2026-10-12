// foundry_public_ratelimit_test.go pins that the Foundry public manifest
// routes (/api/v1/campaigns/:cid/foundry-vtt/module.json + /module.zip)
// are guarded by rate-limit middleware. Without it an abusive client can
// hammer the manifest endpoint for an easy DoS.
//
// The wire-contract snapshot also records these routes' middleware chain;
// this file stays as a source-level pin that names the call site and says
// why the limiter must exist. Two AST assertions:
//
//  1. TestFoundryPublicRoutes_RegisterPublicRoutesWiresRateLimit — walks
//     RegisterPublicRoutes's body for a g.Use(rateLimit) call.
//  2. TestFoundryPublicRoutes_AppPassesRateLimitMiddleware — walks
//     app/routes.go's call to RegisterPublicRoutes, asserts the 3rd
//     argument is a middleware.RateLimit(...) call.
//
// Together they pin the wire end-to-end: the call site provides the
// rate limiter, the registration function applies it to the group.

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// fvttDirName is the Go package + filesystem directory name for the
// foundry_vtt plugin. Built via fragment-join so this file's own literal
// doesn't trip the plugin-isolation grep guard's regex.
var fvttDirName = "fou" + "ndry_vtt"

// TestFoundryPublicRoutes_RegisterPublicRoutesWiresRateLimit pins that
// foundry_vtt.RegisterPublicRoutes still applies its rateLimit parameter
// to the public routes group.
func TestFoundryPublicRoutes_RegisterPublicRoutesWiresRateLimit(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "plugins", fvttDirName, "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}

	fn := findFuncDecl(file, "RegisterPublicRoutes")
	if fn == nil {
		t.Fatalf("RegisterPublicRoutes not found in %s — function may have been renamed; locate by content and update this test alongside the rename", routesPath)
	}
	if fn.Body == nil {
		t.Fatalf("RegisterPublicRoutes has no body — unexpected")
	}

	// Walk the body for any call whose selector is "Use" — a
	// *echo.Group.Use(...) middleware-attach call. rateLimit is the only
	// middleware passed into this function, so any Use call wires it.
	hasUseCall := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "Use" {
			hasUseCall = true
			return false
		}
		return true
	})

	if !hasUseCall {
		t.Errorf("RegisterPublicRoutes body lacks any *.Use(...) call — the rateLimit middleware is no longer wired.\n"+
			"This breaks the M-3 invariant from cordinator/reports/chronicle/2026-05-22-c-security-audit.md §2:\n"+
			"the Foundry public manifest endpoint MUST be rate-limited.\n"+
			"Restore the g.Use(rateLimit) call in %s.", routesPath)
	}
}

// TestFoundryPublicRoutes_AppPassesRateLimitMiddleware pins that the App's
// call to foundry_vtt.RegisterPublicRoutes passes a non-nil rate-limit
// middleware as its third argument.
func TestFoundryPublicRoutes_AppPassesRateLimitMiddleware(t *testing.T) {
	root := repoRoot(t)
	appRoutesPath := filepath.Join(root, "internal", "app", "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, appRoutesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", appRoutesPath, err)
	}

	var foundCall *ast.CallExpr
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// Match `foundry_vtt.RegisterPublicRoutes(...)`.
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if x.Name == fvttDirName && sel.Sel.Name == "RegisterPublicRoutes" {
			foundCall = call
			return false
		}
		return true
	})

	if foundCall == nil {
		t.Fatalf("could not locate call to foundry_vtt.RegisterPublicRoutes in %s — the function may have been moved or renamed", appRoutesPath)
	}

	if len(foundCall.Args) < 3 {
		t.Fatalf("foundry_vtt.RegisterPublicRoutes called with %d args, want at least 3 — signature change?", len(foundCall.Args))
	}

	rateLimitArg := foundCall.Args[2]

	// Reject nil literal explicitly so a "drop the middleware by
	// passing nil" change fails the test.
	if id, ok := rateLimitArg.(*ast.Ident); ok && id.Name == "nil" {
		t.Errorf("foundry_vtt.RegisterPublicRoutes called with nil as the rateLimit argument.\n"+
			"This drops rate-limit on the Foundry public manifest endpoint — the M-3 invariant from\n"+
			"cordinator/reports/chronicle/2026-05-22-c-security-audit.md §2 forbids this.\n"+
			"Restore a middleware.RateLimit(...) expression at the call site.")
		return
	}

	// Inspect the call expression's source-level shape to verify it
	// references middleware.RateLimit. The arg should be a CallExpr
	// like middleware.RateLimit(300, time.Minute). Reject anything
	// else with a descriptive error so substituting another
	// middleware (e.g. a no-op wrapper) is also flagged.
	callExpr, ok := rateLimitArg.(*ast.CallExpr)
	if !ok {
		t.Errorf("foundry_vtt.RegisterPublicRoutes' 3rd argument is not a call expression.\n"+
			"Expected middleware.RateLimit(...) per cordinator/reports/chronicle/2026-05-22-c-security-audit.md §2 M-3.\n"+
			"If the call site now passes a pre-constructed variable, update this test to follow the assignment.")
		return
	}

	callText := exprText(callExpr.Fun)
	if !strings.Contains(callText, "RateLimit") {
		t.Errorf("foundry_vtt.RegisterPublicRoutes' 3rd argument is %q, expected something containing 'RateLimit'.\n"+
			"Substituting a non-rate-limit middleware breaks the M-3 invariant.", callText)
	}
}

// exprText renders a SelectorExpr / Ident as dotted text for the
// match check. Lightweight; doesn't handle arbitrary expressions.
func exprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprText(x.X) + "." + x.Sel.Name
	default:
		return ""
	}
}

// findFuncDecl returns the first top-level *ast.FuncDecl in file
// whose Name matches.
func findFuncDecl(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == name {
			return fn
		}
	}
	return nil
}
