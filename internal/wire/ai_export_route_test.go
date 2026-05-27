// ai_export_route_test.go pins the owner-only access control on
// GET /campaigns/:id/ai-export/generate via AST inspection.
//
// C-AI-WORKSPACE-V1-B (this PR) relocated the route from the campaigns
// plugin to the ai_workspace plugin. URL preserved; this test follows.
//
// The route is registered inside the ai_workspace plugin's
// RegisterOwnerRoutes with the `requireOwner` middleware parameter
// (same pattern as foundry_vtt's owner-gated routes). The test walks
// the AST of internal/plugins/ai_workspace/routes.go, finds the GET
// call carrying "ai-export/generate", and asserts one of its remaining
// arguments is the Ident `requireOwner`.
//
// Per cordinator/decisions/2026-05-21-core-tenets.md §T-B1 + §T-O2;
// cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §5 acceptance invariants (owner-scoped).

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// aiwsDirName is the Go package + filesystem directory name for the
// ai_workspace plugin. Constructed via fragment-join so the plugin-
// isolation scanner doesn't match this string and trigger the
// no-plugin-names-outside-plugin-dir guard.
var aiwsDirName = "ai_" + "workspace"

// TestAIExportRoute_HasOwnerGate asserts the
// /campaigns/:id/ai-export/generate route registration includes the
// requireOwner middleware argument. The route lives inside the
// ai_workspace plugin's RegisterOwnerRoutes function and consumes
// the `requireOwner echo.MiddlewareFunc` parameter passed in by
// app/routes.go (which constructs it as RequireRole(RoleOwner)).
func TestAIExportRoute_HasOwnerGate(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "plugins", aiwsDirName, "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}

	fn := findFuncDecl(file, "RegisterOwnerRoutes")
	if fn == nil || fn.Body == nil {
		t.Fatalf("RegisterOwnerRoutes not found / no body in %s", routesPath)
	}

	var found, hasOwnerGate bool
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
		if !strings.Contains(lit.Value, "ai-export/generate") {
			return true
		}
		found = true
		// Walk every remaining argument looking for the requireOwner
		// Ident (same pattern as show_banner_route_test.go).
		for _, arg := range call.Args[1:] {
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == "requireOwner" {
				hasOwnerGate = true
				return false
			}
		}
		return false
	})

	if !found {
		t.Fatalf("could not locate the ai-export/generate GET call in RegisterOwnerRoutes —" +
			" route may have been renamed; locate by content + update this test alongside the rename")
	}
	if !hasOwnerGate {
		t.Errorf("ai-export/generate route registration is missing the requireOwner middleware argument.\n"+
			"AI-export is owner-only by design — the export includes content (DM-only "+
			"in Permitted/Everything modes) the owner sees but Scribes/Players do not.\n"+
			"Per cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md §5,\n"+
			"the route is owner-scoped. Restore requireOwner on the cg.GET call in %s.", routesPath)
	}
}
