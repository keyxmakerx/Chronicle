// vault_import_routes_test.go pins that every Manage > Import route carries the
// requireOwner middleware. The import creates pages and uploads files under the
// owner's name, so a route that slipped past the gate would let a lower role
// write into the campaign.

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestVaultImportRoutes_AllHaveOwnerGate(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "plugins", "vault_import", "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}
	fn := findFuncDecl(file, "RegisterOwnerRoutes")
	if fn == nil || fn.Body == nil {
		t.Fatalf("RegisterOwnerRoutes not found in %s", routesPath)
	}

	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	routes := 0
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !methods[sel.Sel.Name] || len(call.Args) < 2 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		routes++
		gated := false
		for _, arg := range call.Args[2:] {
			if id, ok := arg.(*ast.Ident); ok && id.Name == "requireOwner" {
				gated = true
			}
		}
		if !gated {
			t.Errorf("%s %s is missing the requireOwner middleware in %s", sel.Sel.Name, lit.Value, routesPath)
		}
		return true
	})
	if routes == 0 {
		t.Fatalf("no routes found in RegisterOwnerRoutes; the test needs updating alongside a rename")
	}
}
