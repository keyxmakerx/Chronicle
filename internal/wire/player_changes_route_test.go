// player_changes_route_test.go pins the owner-only gate on the "What players
// changed" routes via AST inspection. The page shows every player's edits and
// undoes them as the owner; none of it may reach a lower role.

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlayerChangesRoutes_HaveOwnerGate(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "plugins", "playerchanges", "handler.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}
	fn := findFuncDecl(file, "RegisterRoutes")
	if fn == nil || fn.Body == nil {
		t.Fatalf("RegisterRoutes not found / no body in %s", routesPath)
	}
	for _, want := range []struct{ method, path string }{
		{"GET", `"/players/changes"`},
		{"GET", `"/players/changes/results"`},
		{"POST", `"/players/changes/:cid/undo"`},
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
				if ident, ok := arg.(*ast.Ident); ok && ident.Name == "owner" {
					gated = true
				}
			}
			return false
		})
		if !found {
			t.Fatalf("could not locate the %s %s call in RegisterRoutes; update this test alongside a rename", want.method, want.path)
		}
		if !gated {
			t.Errorf("%s %s is missing the owner middleware argument in %s: it shows and undoes players' edits,"+
				" so it must stay owner-only.", want.method, want.path, routesPath)
		}
	}
	// The gate itself must be the Owner role.
	src, err := os.ReadFile(routesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "owner := campaigns.RequireRole(campaigns.RoleOwner)") {
		t.Errorf("%s: owner must be campaigns.RequireRole(campaigns.RoleOwner)", routesPath)
	}
}
