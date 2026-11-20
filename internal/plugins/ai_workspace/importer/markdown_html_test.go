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

// A GFM task list must come out as the editor's checklist, with the box state
// in data-checked, because the sanitizer drops the checkbox input itself.
func TestMarkdownToHTMLTaskLists(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			"tight list keeps each state",
			"- [x] Hero built\n- [ ] Lines ready\n",
			`<ul data-type="taskList"><li data-type="taskItem" data-checked="true"><p>Hero built</p></li><li data-type="taskItem" data-checked="false"><p>Lines ready</p></li></ul>`,
		},
		{
			"loose list",
			"- [ ] One\n\n- [X] Two\n",
			`<ul data-type="taskList"><li data-type="taskItem" data-checked="false"><p>One</p></li><li data-type="taskItem" data-checked="true"><p>Two</p></li></ul>`,
		},
		{
			"inline marks survive",
			"- [ ] Ask the **DM**\n",
			`<ul data-type="taskList"><li data-type="taskItem" data-checked="false"><p>Ask the <strong>DM</strong></p></li></ul>`,
		},
		{
			"nested checklist stays inside its item",
			"- [ ] Parent\n  - [x] Child\n",
			`<ul data-type="taskList"><li data-type="taskItem" data-checked="false"><p>Parent</p><ul data-type="taskList"><li data-type="taskItem" data-checked="true"><p>Child</p></li></ul></li></ul>`,
		},
		{
			"a mixed list is left a plain list",
			"- [ ] Task\n- Plain\n",
			`<ul><li> Task</li><li>Plain</li></ul>`,
		},
		{
			"a plain list is untouched",
			"- one\n- two\n",
			`<ul><li>one</li><li>two</li></ul>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarkdownToHTML(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			// goldmark puts a newline between list items; only the markup matters.
			got = strings.ReplaceAll(got, "\n", "")
			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}
