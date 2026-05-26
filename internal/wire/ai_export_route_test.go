// ai_export_route_test.go pins the owner-only access control on
// GET /campaigns/:id/ai-export/generate via AST inspection.
//
// C-AI-EXPORT-V1 PR-B introduces a new owner-only route that emits
// the campaign's content (including DM-only material when the owner
// opts into Permitted/Everything modes). Even though the renderer
// re-applies sanitize.HTMLPtr per SEC-6-AMENDED, a future refactor
// that silently lowered the route's role gate would expose Scribe
// or Player roles to GM-side intel they shouldn't see.
//
// Mirrors show_banner_route_test.go's shape — same AST traversal,
// same fail-pinpointed-on-drift pattern. Together with the
// renderer-side TestRenderers_FunnelThroughHtmlToMarkdown in
// internal/aiexport/, this test is the second leg of the AI-Export
// security wiring (renderer + route).
//
// Per cordinator/decisions/2026-05-21-core-tenets.md §T-B1 + §T-O2;
// cordinator/reports/chronicle/2026-05-26-c-ai-export-scoping.md
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

// TestAIExportRoute_HasOwnerGate asserts the
// /campaigns/:id/ai-export/generate route registration calls
// RequireRole(RoleOwner). Walks campaigns/routes.go's
// RegisterRoutes function body, finds the GET call with the
// "ai-export/generate" path literal, asserts one of its arguments
// is a CallExpr matching `RequireRole(RoleOwner)`.
func TestAIExportRoute_HasOwnerGate(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "plugins", "campaigns", "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}

	fn := findFuncDecl(file, "RegisterRoutes")
	if fn == nil || fn.Body == nil {
		t.Fatalf("RegisterRoutes not found / no body in %s", routesPath)
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
		// Walk every remaining argument looking for a call to
		// RequireRole whose first argument is the identifier
		// RoleOwner. Tolerant to other middlewares before/after.
		for _, arg := range call.Args[1:] {
			c, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			id, ok := c.Fun.(*ast.Ident)
			if !ok || id.Name != "RequireRole" {
				continue
			}
			if len(c.Args) < 1 {
				continue
			}
			if argIdent, ok := c.Args[0].(*ast.Ident); ok && argIdent.Name == "RoleOwner" {
				hasOwnerGate = true
				return false
			}
		}
		return false
	})

	if !found {
		t.Fatalf("could not locate the ai-export/generate GET call in RegisterRoutes —" +
			" route may have been renamed; locate by content + update this test alongside the rename")
	}
	if !hasOwnerGate {
		t.Errorf("ai-export/generate route registration is missing RequireRole(RoleOwner).\n"+
			"AI-export is owner-only by design — the export includes content (DM-only "+
			"in Permitted/Everything modes) the owner sees but Scribes/Players do not.\n"+
			"Per cordinator/reports/chronicle/2026-05-26-c-ai-export-scoping.md §5,\n"+
			"the route is owner-scoped. Restore RequireRole(RoleOwner) on the cg.GET\n"+
			"call in %s.", routesPath)
	}
}
