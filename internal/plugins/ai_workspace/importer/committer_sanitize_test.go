// committer_sanitize_test.go is the LOAD-BEARING SEC-6 ingress
// mirror. It pins the structural invariant that every entity-
// mutation call site in committer.go (creator.Create /
// creator.Update / creator.UpdateEntry) is preceded by a
// MarkdownToHTML() funnel call in the same function body — i.e.
// every byte of HTML the import path stores has flowed through
// sanitize.HTML at least once before persistence.
//
// Mirror of internal/plugins/ai_workspace/aiexport/renderer_test.go
// (PR #349's TestRenderers_FunnelThroughHtmlToMarkdown). Same
// shape, opposite direction.
//
// Pinpoint quality: on failure, the test reports the EXACT line
// number of the unprotected entity-mutation call inside the
// offending function. The dispatch is explicit that vague failure
// messages should be tightened before commit — and this is the
// dispatch's load-bearing acceptance criterion.
//
// Per cordinator/decisions/2026-05-21-core-tenets.md §T-B1;
// cordinator/reports/chronicle/2026-05-26-c-sec-chunk-6-amended.md
// (egress pin pattern this mirrors);
// cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §5 (V1 acceptance invariants).

package importer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// exemptFunctions lists committer.go functions that may call
// creator.Create / Update / UpdateEntry WITHOUT a sibling
// MarkdownToHTML() call. Each entry MUST include the reason in
// the value — that's the documented justification a future
// reviewer / Cordinator agent reads when the list changes.
//
// Adding a function here is a security-relevant change; the PR
// description must explain why the funnel guarantee still holds.
var exemptFunctions = map[string]string{
	// V1.5 (C-AI-WORKSPACE-V1-G) renamed overwriteExisting → commitUpdate
	// for vocabulary consistency with the new front-matter `action: update`
	// verb. Exemption reason is identical — the caller (commitRow OR
	// commitUpdateExplicit) is responsible for the MarkdownToHTML funnel
	// before invoking; both callers are pinned by this same test.
	"commitUpdate": "Receives already-sanitized bodyJSON + bodyHTML as " +
		"parameters from commitRow or commitUpdateExplicit; both callers " +
		"are pinned by this same test and required to call MarkdownToHTML " +
		"before invoking.",
}

// TestCommitter_EntityMutationsFunnelThroughMarkdownToHTML walks
// committer.go's AST + asserts that every FuncDecl calling
// creator.Create / creator.Update / creator.UpdateEntry also
// contains a MarkdownToHTML() call somewhere in its body. The
// exemption list above identifies functions that legitimately
// don't (because they receive pre-sanitized strings).
func TestCommitter_EntityMutationsFunnelThroughMarkdownToHTML(t *testing.T) {
	src, err := os.ReadFile("committer.go")
	if err != nil {
		t.Fatalf("read committer.go: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "committer.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse committer.go: %v", err)
	}

	// Method names that mutate entity state. Direct calls to these
	// on c.creator.* are the canonical pin targets.
	mutators := map[string]bool{
		"Create":      true,
		"Update":      true,
		"UpdateEntry": true,
	}

	// V1.5 (C-AI-WORKSPACE-V1-G) — wrapper functions on the Committer
	// itself that are exempt from the body check (they receive pre-
	// sanitized values from their caller) but whose CALLERS must
	// still funnel through MarkdownToHTML. Treating these as
	// transitive mutators ensures `commitUpdateExplicit` (and any
	// future wrapper-of-a-wrapper) gets pinned correctly.
	transitiveMutators := exemptFunctions

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name

		// Find every entity-mutation call inside this function.
		// Two shapes:
		//   1. c.creator.<Mutator>(...)  — direct mutator
		//   2. c.<TransitiveMutator>(...) — wrapper of an exempt
		//      function; pin its callers transitively
		var mutationCalls []*ast.CallExpr
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Transitive wrapper match: c.commitUpdate(...) etc.
			// Skip when the function being checked IS the wrapper
			// itself (so commitUpdate doesn't recursively pin itself).
			if _, isWrapper := transitiveMutators[sel.Sel.Name]; isWrapper && sel.Sel.Name != name {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "c" {
					mutationCalls = append(mutationCalls, call)
					return true
				}
			}
			// Direct mutator match: c.creator.{Create,Update,UpdateEntry}.
			if !mutators[sel.Sel.Name] {
				return true
			}
			recv, ok := sel.X.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if recv.Sel.Name != "creator" {
				return true
			}
			mutationCalls = append(mutationCalls, call)
			return true
		})
		if len(mutationCalls) == 0 {
			continue
		}

		// Function has mutation calls. Is it exempt?
		if reason, exempt := exemptFunctions[name]; exempt {
			t.Logf("%s: exempt from MarkdownToHTML funnel — %s", name, reason)
			continue
		}

		// Not exempt — function body MUST also contain a
		// MarkdownToHTML( call.
		bodyStart := fset.Position(fn.Body.Pos()).Offset
		bodyEnd := fset.Position(fn.Body.End()).Offset
		bodyText := string(src[bodyStart:bodyEnd])
		if strings.Contains(bodyText, "MarkdownToHTML(") {
			continue
		}

		// Pinpoint: report the line number of each unprotected
		// mutation call. The dispatch is explicit that the pin's
		// failure must be precise.
		for _, call := range mutationCalls {
			pos := fset.Position(call.Pos())
			sel := call.Fun.(*ast.SelectorExpr).Sel.Name
			t.Errorf("committer.go:%d: %s() in function %s() is not "+
				"protected by a MarkdownToHTML() funnel call in the same "+
				"function body.\n"+
				"\n"+
				"SEC-6-AMENDED mirror: every entity-mutation call site MUST\n"+
				"funnel body content through MarkdownToHTML() before the\n"+
				"entity service sees it — that's the single ingress\n"+
				"sanitize point.\n"+
				"\n"+
				"Two fixes:\n"+
				"  1. Add `MarkdownToHTML(...)` call in %s before this site\n"+
				"     (preferred — keeps the funnel local to the function).\n"+
				"  2. Add %s to exemptFunctions in this file with a clear\n"+
				"     reason (only if the body content is already sanitized\n"+
				"     by the caller, e.g. it's passed in as a string param).\n",
				pos.Line, sel, name, name, name)
		}
	}
}

// TestCommitter_NoDirectGoldmarkBypass guards against a future
// refactor that imports goldmark directly into committer.go
// (bypassing MarkdownToHTML's sanitize.HTML step). Same shape as
// PR #349's getConverter-direct-access guard.
func TestCommitter_NoDirectGoldmarkBypass(t *testing.T) {
	src, err := os.ReadFile("committer.go")
	if err != nil {
		t.Fatalf("read committer.go: %v", err)
	}
	if strings.Contains(string(src), "goldmark.") {
		t.Errorf("committer.go imports goldmark. directly — bypass risk.\n" +
			"Markdown → HTML conversion MUST go through MarkdownToHTML() " +
			"(the single sanitize.HTML funnel). If you need a goldmark-only " +
			"call site, add it inside markdown_html.go where sanitize.HTML " +
			"runs after, then expose a new helper from there.")
	}
}
