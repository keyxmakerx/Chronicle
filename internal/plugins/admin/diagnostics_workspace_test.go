package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

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

// TestDiagnosticsWorkspacePage_Render checks the page offers the questions and
// the hand-picking list, both posting to compose, with the checks list kept
// only behind the copy control. DW_PREVIEW=path also writes the page there.
func TestDiagnosticsWorkspacePage_Render(t *testing.T) {
	data := DiagnosticsWorkspaceData{
		FunctionsJSON: systems.FunctionsSpecJSON(),
		CSRFToken:     "tok",
		Campaigns:     []CampaignOption{{ID: "c1", Name: "Shattered Heroes", Slug: "heroes"}},
		Questions:     systems.DiagnosticQuestions(),
		Groups:        systems.DiagnosticGroups(),
	}
	var buf bytes.Buffer
	if err := DiagnosticsWorkspacePage(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render page: %v", err)
	}
	html := buf.String()
	if p := os.Getenv("DW_PREVIEW"); p != "" {
		_ = os.WriteFile(p, buf.Bytes(), 0o644)
	}
	for _, q := range data.Questions {
		if !strings.Contains(html, q.Title) {
			t.Errorf("page is missing the question %q", q.Title)
		}
	}
	for _, want := range []string{
		`data-widget="diag-picker"`,
		`hx-post="/admin/diagnostics/workspace/compose"`,
		`hx-post="/admin/diagnostics/workspace/parse"`,
		"Pick checks yourself",
		"Copy checks for your AI",
		`name="arg:host.errors"`,
		`<option value="c1">Shattered Heroes</option>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("page HTML missing %q", want)
		}
	}
	if got := strings.Count(html, `name="call" value=`); got < diagnosticsCheckCount(data.Groups) {
		t.Errorf("found %d check boxes, want at least the %d catalog checks", got, diagnosticsCheckCount(data.Groups))
	}
}

func TestSplitSentence(t *testing.T) {
	got := splitSentence("In {campaign}, look up {page}.")
	want := []sentencePart{{Text: "In "}, {Blank: "campaign"}, {Text: ", look up "}, {Blank: "page"}, {Text: "."}}
	if len(got) != len(want) {
		t.Fatalf("got %d parts, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The compose route reads the page's field names (call, arg:<check>) and
// answers with the review step, for site admins only.
func TestDiagnosticsCompose_Route(t *testing.T) {
	e := echo.New()
	RegisterRoutes(e, NewHandler(nil, nil, nil), sessionAuth{}, nil)
	form := url.Values{"call": {"host.errors", "sync.recent"}, "arg:host.errors": {"5"}, "note": {"Something broke"}}.Encode()

	rec := doReq(e, http.MethodPost, "/admin/diagnostics/workspace/compose", "admin-token", "application/x-www-form-urlencoded", []byte(form))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin compose: status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"2 of 2 call(s) will run", "Something broke", "host.errors", "sync.recent", "&#34;arg&#34;: &#34;5&#34;"} {
		if !strings.Contains(body, want) {
			t.Errorf("review is missing %q", want)
		}
	}

	rec = doReq(e, http.MethodPost, "/admin/diagnostics/workspace/compose", "player-token", "application/x-www-form-urlencoded", []byte(form))
	if rec.Code == http.StatusOK {
		t.Error("a player reached the compose route")
	}
}
