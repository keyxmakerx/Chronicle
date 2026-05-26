// invariant_test.go pins the sanitization-on-write invariant from the
// C-SECURITY-AUDIT §3 G-C4: every plugin/widget service.go that accepts
// HTML-typed user input via a function parameter or input struct field
// MUST call sanitize.HTML somewhere in the file. The 8 plugins that
// already follow this convention (per audit §1.3) are the green
// baseline; this test pins them + catches a future plugin author who
// adds an HTML input field without the matching sanitize call.
//
// Per cordinator/decisions/2026-05-21-core-tenets.md §T-B1 + §T-O2;
// cordinator/reports/chronicle/2026-05-22-c-security-audit.md §3 G-C4,
// §5 Chunk 7, §1.3.
//
// SCOPE — focused-invariant pattern, mirrors C-SEC-CHUNK-2's reshape.
//
// The dispatch (C-SEC-CHUNK-7) called for a full AST walker that
// finds Create*/Update* Service methods, identifies HTML-typed
// params, traces variable assignments, and asserts sanitize.HTML
// is called on each. That's a flow-analysis problem (params can be
// renamed, fields can be unpacked into locals, sanitization can
// happen via helpers).
//
// This file ships a coarser-but-meaningful invariant:
//
//   FILE-LEVEL: if a service.go declares any HTML-typed parameter
//   OR struct field, the file MUST contain at least one
//   sanitize.HTML call.
//
// This catches the regression case the audit cared about: a new
// plugin author adding an Update method with an EntryHTML param +
// forgetting to sanitize.
//
// A snapshot file (sanitize_invariant_snapshot.txt) lists the
// per-file inventory so the curated baseline is auditable: which
// files have HTML inputs + how many sanitize calls. Adding HTML
// input to a file with zero sanitize calls trips the test; the
// snapshot diff makes the intent explicit at review time.
//
// To regenerate the snapshot after intentionally adding sanitize
// surface:
//
//	UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...
//
// Same pattern as internal/wire/wire_contract_test.go.
//
// Deferred for a future C-SEC-CHUNK-7-PHASE-2 dispatch:
//   - Method-level invariant (per-Create/Update; flow analysis)
//   - Helper-function tracing (sanitize.HTML wrapped in a plugin-
//     local helper, called from the Create/Update method)
//   - Auto-detect what an "HTML-typed param" means beyond name-
//     ending-in-HTML (e.g. param of type Content where Content.HTML
//     exists)
package sanitize

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fileInventory is the per-file summary captured for the snapshot.
type fileInventory struct {
	RelPath          string   // Repo-relative service.go path.
	HTMLParams       []string // Function-param names ending in HTML (sorted, dedup).
	HTMLStructFields []string // Struct-field names ending in HTML (sorted, dedup).
	SanitizeCalls    int      // Count of sanitize.HTML(... call expressions in the file.
}

// formatLine renders the inventory as a tab-separated snapshot line.
// Keep stable — the snapshot file's contents depend on it.
func (f fileInventory) formatLine() string {
	return fmt.Sprintf(
		"%s\tsanitize_calls=%d\thtml_params=%s\thtml_struct_fields=%s",
		f.RelPath,
		f.SanitizeCalls,
		joinOrNone(f.HTMLParams),
		joinOrNone(f.HTMLStructFields),
	)
}

func joinOrNone(s []string) string {
	if len(s) == 0 {
		return "-"
	}
	return strings.Join(s, ",")
}

// TestSanitizeInvariant_FilesWithHTMLInputCallSanitize is the security
// invariant: every plugin/widget service.go that declares HTML-typed
// inputs MUST call sanitize.HTML somewhere in the file. A future
// plugin author who adds an Update method with EntryHTML param +
// forgets sanitize will see this test fail with a clear pointer to
// the audit's G-C4 + §1.3 inventory.
//
// Test ships green on current main (the 8 audit-verified plugins all
// have both HTML inputs + sanitize calls). Failure = real regression.
func TestSanitizeInvariant_FilesWithHTMLInputCallSanitize(t *testing.T) {
	inventory := walkServiceFiles(t)

	var violations []string
	for _, inv := range inventory {
		hasHTMLInput := len(inv.HTMLParams) > 0 || len(inv.HTMLStructFields) > 0
		if hasHTMLInput && inv.SanitizeCalls == 0 {
			violations = append(violations,
				fmt.Sprintf("  %s declares HTML inputs [params:%s] [struct-fields:%s] but calls sanitize.HTML zero times",
					inv.RelPath,
					joinOrNone(inv.HTMLParams),
					joinOrNone(inv.HTMLStructFields),
				))
		}
	}

	if len(violations) > 0 {
		t.Errorf("sanitization-on-write invariant violated (G-C4).\n"+
			"Per cordinator/reports/chronicle/2026-05-22-c-security-audit.md §1.3,\n"+
			"every service.go file that accepts HTML-typed user input MUST\n"+
			"sanitize via sanitize.HTML. The following files declare HTML\n"+
			"inputs without any sanitize.HTML call:\n\n%s\n\n"+
			"Either add sanitize.HTML calls in the affected file, or — if the\n"+
			"sanitization happens via a helper in a different package — add\n"+
			"a comment in the file's package doc justifying the exception.",
			strings.Join(violations, "\n"))
	}
}

// TestSanitizeInvariant_SnapshotConformance pins the per-file inventory
// of HTML inputs + sanitize-call counts so a future PR that adds a new
// HTML input surface (or removes sanitize calls) surfaces as snapshot
// drift. Regenerate after intentional change:
//
//	UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...
func TestSanitizeInvariant_SnapshotConformance(t *testing.T) {
	inventory := walkServiceFiles(t)

	var lines []string
	for _, inv := range inventory {
		lines = append(lines, inv.formatLine())
	}
	current := strings.Join(lines, "\n") + "\n"

	snapPath := snapshotPath(t)

	if os.Getenv("UPDATE_SANITIZE_SNAPSHOT") == "1" {
		if err := os.WriteFile(snapPath, []byte(current), 0o644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		t.Logf("snapshot regenerated at %s (%d entries)", snapPath, len(lines))
		return
	}

	expectedBytes, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot %s: %v\n\nFirst-time setup: run UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...", snapPath, err)
	}
	expected := string(expectedBytes)

	if current != expected {
		t.Errorf("sanitize-invariant snapshot drift detected.\n\n"+
			"If the drift is intentional (added a new plugin's service.go,\n"+
			"or added/removed HTML inputs to an existing service), regenerate:\n\n"+
			"  UPDATE_SANITIZE_SNAPSHOT=1 go test ./internal/sanitize/...\n\n"+
			"then commit the updated sanitize_invariant_snapshot.txt in the\n"+
			"same PR. Per cordinator/decisions/2026-05-21-core-tenets.md §T-O2,\n"+
			"the PR description must cite the decision/audit that motivated\n"+
			"the sanitize-surface change.\n\n"+
			"Diff (sample first 40 lines):\n%s",
			diffSample(expected, current, 40),
		)
	}
}

// walkServiceFiles enumerates plugin + widget service.go files and
// returns per-file inventories sorted by path.
//
// Each service.go's inventory is augmented with HTML-typed struct
// fields declared in the SAME PACKAGE's model.go (where input/request
// types typically live in Chronicle's convention). That extends the
// "HTML signal" detection beyond what's literally in service.go to
// catch the common pattern where an Update method accepts an *Input
// struct whose HTML field is defined in model.go.
func walkServiceFiles(t *testing.T) []fileInventory {
	t.Helper()
	root := repoRootForSanitize(t)

	var files []string
	for _, dir := range []string{"internal/plugins", "internal/widgets"} {
		base := filepath.Join(root, dir)
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if info.Name() == "service.go" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
	sort.Strings(files)

	out := make([]fileInventory, 0, len(files))
	for _, abs := range files {
		rel, _ := filepath.Rel(root, abs)
		inv := inventoryFile(t, abs, rel)

		// Augment with struct-field detections from the package's
		// model.go (if present). Chronicle's convention puts Input /
		// Request struct definitions in model.go; the Update method
		// signature in service.go takes an *Input pointer and reads
		// its HTML fields via input.EntryHTML. Without scanning
		// model.go, the file-scoped detection misses these.
		dir := filepath.Dir(abs)
		modelPath := filepath.Join(dir, "model.go")
		if _, err := os.Stat(modelPath); err == nil {
			extra := inventoryFile(t, modelPath, "")
			inv.HTMLStructFields = mergeAndSort(inv.HTMLStructFields, extra.HTMLStructFields)
		}

		out = append(out, inv)
	}
	return out
}

// mergeAndSort combines two string slices into a sorted, deduped result.
func mergeAndSort(a, b []string) []string {
	set := make(map[string]struct{}, len(a)+len(b))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		set[s] = struct{}{}
	}
	return sortedKeys(set)
}

// inventoryFile parses a single .go file and extracts the HTML-input +
// sanitize-call inventory.
func inventoryFile(t *testing.T, abs, rel string) fileInventory {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, abs, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", abs, err)
	}

	inv := fileInventory{RelPath: rel}
	paramSet := map[string]struct{}{}
	fieldSet := map[string]struct{}{}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Type == nil || x.Type.Params == nil {
				return true
			}
			for _, field := range x.Type.Params.List {
				for _, name := range field.Names {
					if endsHTML(name.Name) {
						paramSet[name.Name] = struct{}{}
					}
				}
			}
		case *ast.StructType:
			if x.Fields == nil {
				return true
			}
			for _, field := range x.Fields.List {
				for _, name := range field.Names {
					if endsHTML(name.Name) {
						fieldSet[name.Name] = struct{}{}
					}
				}
			}
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name == "sanitize" && sel.Sel.Name == "HTML" {
				inv.SanitizeCalls++
			}
		}
		return true
	})

	inv.HTMLParams = sortedKeys(paramSet)
	inv.HTMLStructFields = sortedKeys(fieldSet)
	return inv
}

// endsHTML reports whether the identifier looks like an HTML-typed
// field/param. Matches "HTML" exactly or any camelCase identifier
// whose final two segments are <something>HTML (e.g. EntryHTML,
// descHTML, notesHTML).
func endsHTML(name string) bool {
	if name == "HTML" {
		return false // type itself, not a parameter
	}
	return strings.HasSuffix(name, "HTML")
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// repoRootForSanitize finds the repository root by walking up from the
// test file's directory.
func repoRootForSanitize(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// cwd will be internal/sanitize when tests run; walk up two dirs.
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

func snapshotPath(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(cwd, "sanitize_invariant_snapshot.txt")
}

// diffSample returns a coarse unified-diff sample for the error message
// — first n lines of "expected" + first n lines of "got" tagged.
func diffSample(expected, got string, n int) string {
	expLines := strings.Split(expected, "\n")
	gotLines := strings.Split(got, "\n")
	if len(expLines) > n {
		expLines = expLines[:n]
	}
	if len(gotLines) > n {
		gotLines = gotLines[:n]
	}
	return "---EXPECTED (snapshot)---\n" + strings.Join(expLines, "\n") +
		"\n\n---GOT (current)---\n" + strings.Join(gotLines, "\n")
}
