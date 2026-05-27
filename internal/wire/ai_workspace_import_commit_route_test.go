// ai_workspace_import_commit_route_test.go pins the owner-only
// access control on POST /campaigns/:id/ai-workspace/import/commit
// via AST inspection.
//
// Mirror of ai_workspace_import_parse_route_test.go; same shape.
// The commit endpoint is the first AI Workspace route that mutates
// the database — it creates entity-type rows + entity rows from
// operator-supplied markdown. Non-owner access would let lower-role
// members write content to the campaign under the owner's name and
// would also expose DM-side material that may sit in the source
// (Permitted / Everything content mode upstream).
//
// Per cordinator/decisions/2026-05-21-core-tenets.md §T-B1 + §T-O2;
// cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §5 acceptance invariants (owner-scoped); V1-E dispatch.

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestAIWorkspaceImportCommitRoute_HasOwnerGate asserts that the
// /ai-workspace/import/commit route registration includes the
// requireOwner middleware argument.
func TestAIWorkspaceImportCommitRoute_HasOwnerGate(t *testing.T) {
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
		if !ok || sel.Sel.Name != "POST" {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if !strings.Contains(lit.Value, "ai-workspace/import/commit") {
			return true
		}
		found = true
		for _, arg := range call.Args[1:] {
			if ident, ok := arg.(*ast.Ident); ok && ident.Name == "requireOwner" {
				hasOwnerGate = true
				return false
			}
		}
		return false
	})

	if !found {
		t.Fatalf("could not locate the ai-workspace/import/commit POST call in RegisterOwnerRoutes —" +
			" route may have been renamed; locate by content + update this test alongside the rename")
	}
	if !hasOwnerGate {
		t.Errorf("ai-workspace/import/commit route registration is missing the requireOwner middleware argument.\n"+
			"The commit endpoint mutates the database — it creates entity-type rows and entity rows on the\n"+
			"operator's behalf. Exposing it to non-owner roles would let lower-role members write content\n"+
			"to the campaign and would expose DM-side material referenced in the markdown source. Restore\n"+
			"requireOwner on the cg.POST call in %s.", routesPath)
	}
}
