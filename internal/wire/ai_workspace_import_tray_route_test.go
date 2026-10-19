// ai_workspace_import_tray_route_test.go pins the owner-only access control
// on the AI Import tray routes via AST inspection. The tray holds the parts of
// an AI's answer pasted by the owner, which can carry Director-only material,
// and clearing it deletes them; neither may reach a lower role.

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestAIWorkspaceImportTrayRoutes_HaveOwnerGate asserts that both tray
// route registrations include the requireOwner middleware argument.
func TestAIWorkspaceImportTrayRoutes_HaveOwnerGate(t *testing.T) {
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

	for _, want := range []struct{ method, path string }{
		{"GET", `"/ai-workspace/import/tray"`},
		{"POST", `"/ai-workspace/import/tray/clear"`},
	} {
		var found, gated bool
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != want.method || len(call.Args) < 1 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || !strings.EqualFold(lit.Value, want.path) {
				return true
			}
			found = true
			for _, arg := range call.Args[1:] {
				if ident, ok := arg.(*ast.Ident); ok && ident.Name == "requireOwner" {
					gated = true
				}
			}
			return false
		})
		if !found {
			t.Fatalf("could not locate the %s %s call in RegisterOwnerRoutes; update this test alongside a rename", want.method, want.path)
		}
		if !gated {
			t.Errorf("%s %s is missing the requireOwner middleware argument in %s: the tray holds the owner's pasted"+
				" parts and clearing it deletes them, so it must stay owner-only.", want.method, want.path, routesPath)
		}
	}
}
