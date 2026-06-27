package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// TestDiagnosticsBatchReview_Render is the render smoke-test for the review
// fragment: it drives a real ParseBatch plan (mixed runnable / duplicate /
// unknown / gated rows) through the templ render and asserts the operator-facing
// markup is present and error-free. Catches templ runtime issues that compilation
// alone can't (nil derefs in the template, missing branches, etc.).
func TestDiagnosticsBatchReview_Render(t *testing.T) {
	plan, err := systems.ParseBatch(`{
	  "note": "why the old sheet renders",
	  "calls": [
	    {"name": "system.versions"},
	    {"name": "system.versions"},
	    {"name": "system.health"},
	    {"name": "nope.bogus"}
	  ]
	}`)
	if err != nil {
		t.Fatalf("ParseBatch: %v", err)
	}

	var buf bytes.Buffer
	data := DiagnosticsReviewData{Raw: "{...}", Plan: plan, CSRFToken: "tok"}
	if err := DiagnosticsBatchReview(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render review: %v", err)
	}
	html := buf.String()
	for _, want := range []string{
		"Review &amp; approve", // header (escaped ampersand)
		"system.versions",      // a runnable call
		"runs once",            // the duplicate note
		"no such function",     // the unknown note
		"full_dump",            // the gated full-dump hint
		"Approve",              // the run button (RunnableN > 0)
		"/admin/diagnostics/workspace/run",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("review HTML missing %q", want)
		}
	}
	// The re-embedded raw text must be present for the run step to re-parse.
	if !strings.Contains(html, `name="batch"`) {
		t.Error("review HTML missing the re-embedded batch field")
	}
}

// TestDiagnosticsBatchReview_Error renders the parse-error branch.
func TestDiagnosticsBatchReview_Error(t *testing.T) {
	var buf bytes.Buffer
	data := DiagnosticsReviewData{Err: "not a valid batch object", CSRFToken: "tok"}
	if err := DiagnosticsBatchReview(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render error review: %v", err)
	}
	if !strings.Contains(buf.String(), "not a valid batch object") {
		t.Error("error review HTML should show the parse error")
	}
}

// TestDiagnosticsBatchReview_NothingRunnable renders the all-skipped branch
// (no Approve button, the "fix the names" hint instead).
func TestDiagnosticsBatchReview_NothingRunnable(t *testing.T) {
	plan, err := systems.ParseBatch(`{"calls":[{"name":"nope.bogus"}]}`)
	if err != nil {
		t.Fatalf("ParseBatch: %v", err)
	}
	var buf bytes.Buffer
	if err := DiagnosticsBatchReview(DiagnosticsReviewData{Plan: plan, Raw: "{}"}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if strings.Contains(html, "/admin/diagnostics/workspace/run") {
		t.Error("no Approve&run button should render when nothing is runnable")
	}
	if !strings.Contains(html, "Nothing runnable") {
		t.Error("expected the nothing-runnable hint")
	}
}

// TestDiagnosticsBatchResult_Render smoke-tests the result fragment (copy pane).
func TestDiagnosticsBatchResult_Render(t *testing.T) {
	var buf bytes.Buffer
	res := "# Chronicle diagnostics — batch result\n\n_ran 1 function · 42 bytes · ~10 tokens_\n"
	if err := DiagnosticsBatchResult(DiagnosticsResultData{Result: res}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render result: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "data-ai-export-copy") || !strings.Contains(html, "data-ai-export-body") {
		t.Error("result fragment should wire the ai-export copy widget")
	}
	if !strings.Contains(html, "batch result") {
		t.Error("result fragment should contain the rendered output")
	}
}
