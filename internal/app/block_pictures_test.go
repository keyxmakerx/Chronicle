package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// TestAppBlocksDeclareAPicture pins that every widget this package registers
// for another plugin names a drawing the Customizer's widget list knows. The
// registrations need live services, so the source is read instead: every
// entities.BlockMeta literal must carry a known Picture. The one exception is
// a package's own widget (Type "ext_widget" with the package's name as its
// label), which has none and gets the plain lines drawing.
func TestAppBlocksDeclareAPicture(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	seen := 0
	for _, pkg := range pkgs {
		for name, f := range pkg.Files {
			check := func(lit *ast.CompositeLit) {
				fields := map[string]ast.Expr{}
				for _, e := range lit.Elts {
					if kv, ok := e.(*ast.KeyValueExpr); ok {
						if id, ok := kv.Key.(*ast.Ident); ok {
							fields[id.Name] = kv.Value
						}
					}
				}
				if s, ok := fields["Type"].(*ast.BasicLit); ok && s.Value == `"ext_widget"` {
					return
				}
				seen++
				line := fset.Position(lit.Pos()).Line
				pic, ok := fields["Picture"]
				if !ok {
					t.Errorf("%s:%d: a widget registered here names no Picture", name, line)
					return
				}
				sel, ok := pic.(*ast.SelectorExpr)
				if !ok || !entities.PictureKinds[pictureValue(sel.Sel.Name)] {
					t.Errorf("%s:%d: Picture must be one of the entities.Picture* kinds", name, line)
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if isBlockMeta(lit.Type) {
					check(lit)
				} else if arr, ok := lit.Type.(*ast.ArrayType); ok && isBlockMeta(arr.Elt) {
					for _, e := range lit.Elts {
						if el, ok := e.(*ast.CompositeLit); ok {
							check(el)
						}
					}
				}
				return true
			})
		}
	}
	if seen < 20 {
		t.Fatalf("found only %d widget registrations; the scan has gone wrong", seen)
	}
}

// isBlockMeta reports whether a type expression is entities.BlockMeta.
func isBlockMeta(t ast.Expr) bool {
	sel, ok := t.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "BlockMeta"
}

// pictureValue maps a constant's name to its value.
func pictureValue(name string) string {
	values := map[string]string{
		"PictureHeading": entities.PictureHeading, "PictureLines": entities.PictureLines, "PictureQuote": entities.PictureQuote,
		"PictureImage": entities.PictureImage, "PictureCard": entities.PictureCard, "PictureFacts": entities.PictureFacts,
		"PictureSheet": entities.PictureSheet, "PictureLinks": entities.PictureLinks, "PictureGrid": entities.PictureGrid,
		"PictureFeed": entities.PictureFeed, "PictureTree": entities.PictureTree, "PictureGraph": entities.PictureGraph,
		"PictureMap": entities.PictureMap, "PictureCalendar": entities.PictureCalendar, "PictureTimeline": entities.PictureTimeline,
		"PictureTable": entities.PictureTable, "PictureCounter": entities.PictureCounter, "PictureSteps": entities.PictureSteps,
		"PictureTags": entities.PictureTags, "PictureDivider": entities.PictureDivider, "PictureSearch": entities.PictureSearch,
	}
	return values[name]
}
