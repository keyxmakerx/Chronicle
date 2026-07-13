// entity_type_reorder_route_test.go pins the Owner-only access control on
// PUT /campaigns/:id/entity-types/:etid/reorder via AST inspection.
//
// The sub-category reorder route was introduced Scribe-gated in C-NAV-V3 PR1
// and realigned to Owner in PR2 (0c / RC-15.4): reordering the entity-type
// taxonomy is structural campaign configuration — the same tier as every other
// entity-type mutation (create/update/delete) and the sidebar-config PUT, all
// Owner-only. The wire snapshot tracks method+path+file but NOT the role, so
// this AST test is the guard against a silent downgrade back to Scribe.
//
// It walks the AST of internal/plugins/entities/routes.go, finds the PUT call
// carrying "entity-types/:etid/reorder", and asserts one of its remaining
// arguments is RequireRole(RoleOwner).

package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestEntityTypeReorderRoute_HasOwnerGate asserts the sub-category reorder
// route registration carries RequireRole(RoleOwner).
func TestEntityTypeReorderRoute_HasOwnerGate(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "plugins", "entities", "routes.go")

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
		if !ok || sel.Sel.Name != "PUT" {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if !strings.Contains(lit.Value, "entity-types/:etid/reorder") {
			return true
		}
		found = true
		// One of the remaining args must be RequireRole(RoleOwner).
		for _, arg := range call.Args[1:] {
			rr, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			rrSel, ok := rr.Fun.(*ast.SelectorExpr)
			if !ok || rrSel.Sel.Name != "RequireRole" || len(rr.Args) != 1 {
				continue
			}
			roleSel, ok := rr.Args[0].(*ast.SelectorExpr)
			if ok && roleSel.Sel.Name == "RoleOwner" {
				hasOwnerGate = true
				return false
			}
		}
		return false
	})

	if !found {
		t.Fatalf("could not locate the entity-types/:etid/reorder PUT call in RegisterRoutes —" +
			" route may have been renamed; locate by content + update this test alongside the rename")
	}
	if !hasOwnerGate {
		t.Errorf("entity-types/:etid/reorder route registration is missing RequireRole(RoleOwner).\n" +
			"Sub-category reorder is structural entity-type configuration — Owner-only, matching every\n" +
			"other entity-type mutation and the sidebar-config PUT (0c / RC-15.4). Restore\n" +
			"campaigns.RequireRole(campaigns.RoleOwner) on the cg.PUT call in internal/plugins/entities/routes.go.")
	}
}
