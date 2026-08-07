// export_adapter_wiring_test.go pins that every export/import section the
// campaign export envelope declares is actually wired to an adapter in
// app/routes.go.
//
// This exists because of the notes hole (sweep R4 stage 15): campaigns'
// ExportImportService had SetNoteExporter / SetNoteImporter setters and an
// envelope field for notes, but nothing ever called the setters. The
// adapters simply weren't written, so `Export` skipped the nil exporter and
// `Import` skipped the nil importer — a campaign backup omitted every shared
// note in the campaign and reported success. Nothing failed, because a nil
// adapter is silently a no-op by design (some sections legitimately depend
// on optional plugins).
//
// The nil-adapter-skips-silently design is worth keeping; what was missing
// is a place where "we meant to wire this" is written down. That is this
// test: it walks the AST of app/routes.go and requires a call to every
// setter named below. Adding a new export section means adding it here.
package wire

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// requiredExportSetters are the ExportImportService wiring calls that must
// appear in app/routes.go. Every one of these corresponds to a field on
// campaigns.CampaignExport that would otherwise be silently empty.
var requiredExportSetters = []string{
	"SetEntityExporter", "SetEntityImporter",
	"SetCalendarExporter", "SetCalendarImporter",
	"SetTimelineExporter", "SetTimelineImporter",
	"SetSessionExporter", "SetSessionImporter",
	"SetMapExporter", "SetMapImporter",
	"SetNoteExporter", "SetNoteImporter",
	"SetAddonExporter", "SetAddonImporter",
	"SetGroupExporter", "SetGroupImporter",
	"SetPostExporter", "SetPostImporter",
	"SetMediaExporter", "SetMediaBundler",
}

// TestExportAdapters_AllSectionsWired fails if app/routes.go stops calling
// any of the required wiring setters.
func TestExportAdapters_AllSectionsWired(t *testing.T) {
	root := repoRoot(t)
	routesPath := filepath.Join(root, "internal", "app", "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routesPath, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", routesPath, err)
	}

	seen := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		// Only count calls on the export service itself, so an unrelated
		// method of the same name elsewhere can't satisfy the pin.
		recv, ok := sel.X.(*ast.Ident)
		if !ok || recv.Name != "exportSvc" {
			return true
		}
		if len(call.Args) != 1 {
			return true
		}
		seen[sel.Sel.Name] = true
		return true
	})

	for _, setter := range requiredExportSetters {
		if !seen[setter] {
			t.Errorf("app/routes.go never calls exportSvc.%s — that export section "+
				"is silently skipped and its data never leaves (or re-enters) the instance", setter)
		}
	}
}
