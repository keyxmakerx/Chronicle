// Package wire holds Chronicle's wire-contract integrity tests.
//
// C-CI-GUARDS-PHASE-2 Guard 3 (per cordinator/dispatches/chronicle/C-CI-GUARDS-PHASE-2.md):
// snapshot-test the set of Echo route registrations across the codebase so any
// PR that adds, removes, or renames a route surfaces as drift against a curated
// expected list (snapshot file).
//
// Cites: cordinator/decisions/2026-05-21-core-tenets.md §T-O2 (consumer-verified
// wire contracts); cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §5
// (the chronicle#323 lesson — duplicate-path silent fallthrough on the wrong
// auth surface); cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md
// §0.5 D7 (the comprehensive manual-list policy choice).
//
// PHASE 2A SCOPE (intentional simplifications, documented for Phase 2B follow-up):
//
//   1. AST-based static extraction, not runtime `e.Routes()` enumeration.
//      Runtime enumeration requires constructing the full App (real DB, Redis,
//      services); too heavy for the test layer. AST extraction catches every
//      route registered by a literal call like `<receiver>.METHOD("...", ...)`
//      with a string-literal path.
//
//   2. Echo group-prefix NOT resolved statically. The tuple recorded is
//      (file, method, path-literal-at-call-site). Renaming `e.Group("/admin")`
//      to `e.Group("/v2/admin")` does NOT trigger this test — the child routes
//      look identical at their call sites. Phase 2B can add prefix resolution
//      by tracking variable assignments through the AST.
//
//   3. Auth surface NOT classified. The dispatch spec'd a (method, path, auth)
//      tuple; we ship (file, method, path) for Phase 2A. The four auth surfaces
//      in §5.1 of the hygiene audit are middleware-determined, which needs
//      symbol resolution beyond what go/ast gives us without type info.
//      Phase 2B can lift this via golang.org/x/tools/go/packages.
//
//   4. Routes registered via loops, helper functions, or programmatic builders
//      are NOT captured. Today Chronicle uses literal METHOD-call registration
//      throughout (per the audit's §5.1 inventory); should a future refactor
//      introduce a builder, this test would silently miss those routes — Phase 2B
//      candidate for follow-up.
//
// To update the snapshot after intentionally adding/removing routes:
//
//	UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...
//
// The PR description must then cite this dispatch + the wire-surface decision
// (or amend the audit's §5 if a new auth surface is introduced).
package wire

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// routeCall is one captured Echo route registration site.
type routeCall struct {
	File   string // Repo-relative path to the .go file holding the call.
	Method string // HTTP verb (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS).
	Path   string // The path string-literal as it appears at the call site.
}

// String formats a routeCall as a snapshot line. Keep this format stable —
// the snapshot file's contents depend on it.
func (r routeCall) String() string {
	return fmt.Sprintf("%s\t%s\t%s", r.Method, r.Path, r.File)
}

// httpVerbs is the set of method names we recognize as Echo route registrations.
// Echo's *Echo and *Group both expose these as methods; the AST visitor checks
// for SelectorExpr like `recv.METHOD(...)` where METHOD ∈ this set.
var httpVerbs = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"PATCH":   true,
	"DELETE":  true,
	"HEAD":    true,
	"OPTIONS": true,
}

// extractRouteCalls walks the AST of every .go file under repoRoot/internal/
// and returns every Echo-style route registration call. File paths in the
// returned slice are repo-relative.
//
// Scan rules:
//   - Skip _test.go files (tests aren't route registration sites).
//   - Recognize `x.METHOD("path", ...)` where METHOD is in httpVerbs.
//   - First argument must be a STRING literal; calls with non-literal paths
//     (variables, helper functions) are skipped with a stderr note so the
//     coordinator can audit them.
func extractRouteCalls(t *testing.T, repoRoot string) []routeCall {
	t.Helper()
	internal := filepath.Join(repoRoot, "internal")

	var out []routeCall
	var nonLiteralPaths []string

	err := filepath.Walk(internal, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			// Parse errors mean the file isn't compilable as-is. The build job
			// catches these separately; this test shouldn't shadow that error.
			return nil
		}

		relPath, _ := filepath.Rel(repoRoot, path)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			method := sel.Sel.Name
			if !httpVerbs[method] {
				return true
			}
			if len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				// Non-literal first arg — could be a builder pattern or a
				// dynamically-computed path. Record for coordinator audit.
				pos := fset.Position(call.Pos())
				nonLiteralPaths = append(nonLiteralPaths,
					fmt.Sprintf("%s:%d: %s(<non-literal>, ...)", relPath, pos.Line, method))
				return true
			}
			pathStr, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			out = append(out, routeCall{
				File:   relPath,
				Method: method,
				Path:   pathStr,
			})
			return true
		})

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", internal, err)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Method != out[j].Method {
			return out[i].Method < out[j].Method
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].File < out[j].File
	})

	if len(nonLiteralPaths) > 0 {
		t.Logf("wire_contract_test: %d non-literal-path route registrations skipped "+
			"(coordinator audit candidates for Phase 2B prefix resolution):",
			len(nonLiteralPaths))
		for _, np := range nonLiteralPaths {
			t.Logf("  %s", np)
		}
	}

	return out
}

// snapshotPath returns the path to the curated expected-routes file.
func snapshotPath(t *testing.T) string {
	t.Helper()
	// Walk up to the repo root from this test file.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// internal/wire — repo root is two dirs up.
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	return filepath.Join(root, "internal", "wire", "routes_snapshot.txt")
}

// repoRoot returns the Chronicle repo root inferred from the test's cwd.
func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

// TestWireContractConformance asserts that the set of Echo route registrations
// across internal/ exactly matches the snapshot at internal/wire/routes_snapshot.txt.
//
// Drift in either direction (route added without snapshot update, or snapshot
// entry without a matching call site) fails this test. To resolve:
//
//	UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...
//
// then commit the regenerated snapshot in the same PR that introduced the
// intentional change. The PR description must explain WHY (which decision /
// audit / dispatch motivated the change).
func TestWireContractConformance(t *testing.T) {
	actual := extractRouteCalls(t, repoRoot(t))
	actualLines := make([]string, len(actual))
	for i, r := range actual {
		actualLines[i] = r.String()
	}
	actualSnapshot := strings.Join(actualLines, "\n") + "\n"

	snap := snapshotPath(t)

	if os.Getenv("UPDATE_ROUTES_SNAPSHOT") != "" {
		if err := os.WriteFile(snap, []byte(actualSnapshot), 0o644); err != nil {
			t.Fatalf("write snapshot %s: %v", snap, err)
		}
		t.Logf("UPDATE_ROUTES_SNAPSHOT set — wrote %d routes to %s", len(actual), snap)
		return
	}

	want, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("read snapshot %s: %v\n"+
			"If this is the first run, regenerate with:\n"+
			"  UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...",
			snap, err)
	}

	if string(want) != actualSnapshot {
		// Report a focused diff so the failure is actionable.
		wantLines := strings.Split(strings.TrimRight(string(want), "\n"), "\n")
		gotLines := strings.Split(strings.TrimRight(actualSnapshot, "\n"), "\n")

		wantSet := map[string]bool{}
		for _, l := range wantLines {
			wantSet[l] = true
		}
		gotSet := map[string]bool{}
		for _, l := range gotLines {
			gotSet[l] = true
		}

		var added, removed []string
		for l := range gotSet {
			if !wantSet[l] {
				added = append(added, l)
			}
		}
		for l := range wantSet {
			if !gotSet[l] {
				removed = append(removed, l)
			}
		}
		sort.Strings(added)
		sort.Strings(removed)

		var msg strings.Builder
		msg.WriteString("wire-contract drift detected.\n\n")
		if len(added) > 0 {
			msg.WriteString(fmt.Sprintf("Routes added since snapshot (%d):\n", len(added)))
			for _, l := range added {
				msg.WriteString("  + " + l + "\n")
			}
			msg.WriteString("\n")
		}
		if len(removed) > 0 {
			msg.WriteString(fmt.Sprintf("Routes removed since snapshot (%d):\n", len(removed)))
			for _, l := range removed {
				msg.WriteString("  - " + l + "\n")
			}
			msg.WriteString("\n")
		}
		msg.WriteString("If the drift is intentional, regenerate the snapshot:\n")
		msg.WriteString("  UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...\n\n")
		msg.WriteString("then commit the updated routes_snapshot.txt in the same PR.\n")
		msg.WriteString("Per cordinator/decisions/2026-05-21-core-tenets.md §T-O2,\n")
		msg.WriteString("the PR description must cite the decision/audit that motivated\n")
		msg.WriteString("the route change, especially if it touches an auth surface.\n")

		t.Fatal(msg.String())
	}
}

// TestWireContractSnapshotShape sanity-checks the snapshot file format itself
// so a corrupted snapshot doesn't masquerade as a passing test.
func TestWireContractSnapshotShape(t *testing.T) {
	snap := snapshotPath(t)
	data, err := os.ReadFile(snap)
	if err != nil {
		t.Skipf("snapshot %s not present (first run); will be created by UPDATE_ROUTES_SNAPSHOT", snap)
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		t.Fatalf("snapshot %s is empty — that's suspicious; Chronicle has many routes", snap)
	}
	for i, l := range lines {
		parts := strings.Split(l, "\t")
		if len(parts) != 3 {
			t.Errorf("snapshot line %d malformed (want 3 tab-separated fields, got %d): %q",
				i+1, len(parts), l)
			continue
		}
		method, path, file := parts[0], parts[1], parts[2]
		if !httpVerbs[method] {
			t.Errorf("snapshot line %d: unknown HTTP method %q", i+1, method)
		}
		if path != "" && !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "*") {
			t.Errorf("snapshot line %d: path %q doesn't start with / or *", i+1, path)
		}
		if !strings.HasSuffix(file, ".go") {
			t.Errorf("snapshot line %d: file %q doesn't end with .go", i+1, file)
		}
	}
}
