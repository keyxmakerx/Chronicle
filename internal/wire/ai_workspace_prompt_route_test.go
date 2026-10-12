// ai_workspace_prompt_route_test.go pins the owner-only access control on
// GET /campaigns/:id/ai-workspace/prompt/generate via AST inspection. The
// prompt builder can include DM-only campaign content in Permitted/
// Everything modes, so a silently dropped gate would expose it to
// non-owner roles.

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestAIWorkspacePromptRoute_HasOwnerGate asserts that the
// /ai-workspace/prompt/generate route registration includes the
// requireOwner middleware argument.
func TestAIWorkspacePromptRoute_HasOwnerGate(t *testing.T) {
	for _, path := range []string{"ai-workspace/prompt/generate", "ai-workspace/prompt/instructions"} {
		t.Run(path, func(t *testing.T) { checkOwnerGate(t, path) })
	}
}

// checkOwnerGate asserts the GET route for path carries requireOwner.
func checkOwnerGate(t *testing.T, path string) {
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
		if !strings.Contains(lit.Value, path) {
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
		t.Fatalf("could not locate the %s GET call in RegisterOwnerRoutes —"+
			" route may have been renamed; locate by content + update this test alongside the rename", path)
	}
	if !hasOwnerGate {
		t.Errorf("%s route registration is missing the requireOwner middleware argument.\n"+
			"The prompt builder can include DM-only campaign content in Permitted/Everything modes; "+
			"exposing it to non-owner roles would leak GM-side material.\n"+
			"Per cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md §5,\n"+
			"the route is owner-scoped. Restore requireOwner on the cg.GET call in %s.", path, routesPath)
	}
}
