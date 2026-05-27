// markdown_html_test.go pins the goldmark → sanitize.HTML funnel.
// SEC-6-AMENDED's structural-pin enforcement lands in Phase 5
// (alongside the committer); this file pins the BEHAVIOR — a
// malicious markdown input produces a sanitized HTML output.
package importer

import (
	"strings"
	"testing"
)

func TestMarkdownToHTML_BasicConversion(t *testing.T) {
	got, err := MarkdownToHTML("# Title\n\nHello **world**.")
	if err != nil {
		t.Fatalf("MarkdownToHTML: %v", err)
	}
	for _, want := range []string{"<h1", "Title", "<strong>world</strong>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

// TestMarkdownToHTML_StripsRawHTML guards against the AI emitting
// raw HTML inside markdown source — goldmark's WithUnsafe is OFF
// (default), so embedded <script> shouldn't pass through.
func TestMarkdownToHTML_StripsRawHTML(t *testing.T) {
	input := "Hello <script>alert(1)</script> world\n"
	got, err := MarkdownToHTML(input)
	if err != nil {
		t.Fatalf("MarkdownToHTML: %v", err)
	}
	if strings.Contains(strings.ToLower(got), "<script") {
		t.Errorf("<script> survived goldmark + sanitize:\n%s", got)
	}
}

// TestMarkdownToHTML_StripsJavascriptLinks covers the `[text](javascript:...)`
// vector — operators paste AI output; AI can be tricked into
// emitting javascript: URLs. bluemonday's UGCPolicy strips them.
func TestMarkdownToHTML_StripsJavascriptLinks(t *testing.T) {
	input := "[click](javascript:alert(1))"
	got, err := MarkdownToHTML(input)
	if err != nil {
		t.Fatalf("MarkdownToHTML: %v", err)
	}
	if strings.Contains(strings.ToLower(got), "javascript:") {
		t.Errorf("javascript: URL survived sanitize:\n%s", got)
	}
}

// TestMarkdownToHTML_PreservesTablesGFM verifies the GFM extension
// is wired — AI tools use tables heavily.
func TestMarkdownToHTML_PreservesTablesGFM(t *testing.T) {
	input := `| Col1 | Col2 |
|------|------|
| A    | B    |`
	got, err := MarkdownToHTML(input)
	if err != nil {
		t.Fatalf("MarkdownToHTML: %v", err)
	}
	for _, want := range []string{"<table", "<thead", "<tr", "<th", "Col1", "Col2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q (GFM table not parsed):\n%s", want, got)
		}
	}
}

// TestMarkdownToHTML_PreservesLists covers ordered + unordered lists.
func TestMarkdownToHTML_PreservesLists(t *testing.T) {
	got, err := MarkdownToHTML("- one\n- two\n\n1. uno\n2. dos\n")
	if err != nil {
		t.Fatalf("MarkdownToHTML: %v", err)
	}
	if !strings.Contains(got, "<ul>") || !strings.Contains(got, "<ol>") {
		t.Errorf("lists not parsed:\n%s", got)
	}
}

// TestMarkdownToHTML_PreservesCodeBlocks ensures fenced code
// converts to <pre><code>.
func TestMarkdownToHTML_PreservesCodeBlocks(t *testing.T) {
	input := "```go\nfunc main() {}\n```"
	got, err := MarkdownToHTML(input)
	if err != nil {
		t.Fatalf("MarkdownToHTML: %v", err)
	}
	if !strings.Contains(got, "<pre>") || !strings.Contains(got, "<code") {
		t.Errorf("code block not parsed:\n%s", got)
	}
}

func TestMarkdownToHTML_Empty(t *testing.T) {
	got, err := MarkdownToHTML("")
	if err != nil || got != "" {
		t.Errorf("empty → (%q, %v); want (\"\", nil)", got, err)
	}
}
