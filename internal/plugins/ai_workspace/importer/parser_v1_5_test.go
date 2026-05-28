// parser_v1_5_test.go covers the V1.5 verb-set parser extension
// (C-AI-WORKSPACE-V1-G): front-matter `action:` field validation,
// default-to-create, per-action body warnings, known-keys list.

package importer

import (
	"strings"
	"testing"
)

// TestParse_Action_DefaultsToCreate — empty Action field → parser
// fills in "create" so the downstream committer's default-dispatch
// branch fires (preserves V1 behavior for V1-era prompts).
func TestParse_Action_DefaultsToCreate(t *testing.T) {
	input := `---
name: Bob
type: character
---

# Bob

Body.`
	pages := Parse(input)
	if len(pages) != 1 || pages[0].FrontMatter.Action != ActionCreate {
		t.Fatalf("expected FrontMatter.Action defaulted to %q; got %q",
			ActionCreate, pages[0].FrontMatter.Action)
	}
	if pages[0].Status == StatusParseError {
		t.Errorf("default action should not yield StatusParseError; got ParseError=%q",
			pages[0].ParseError)
	}
}

// TestParse_Action_InvalidEnumRejected — unknown action value yields
// StatusParseError with friendly message naming the valid options.
func TestParse_Action_InvalidEnumRejected(t *testing.T) {
	input := `---
name: Bob
type: character
action: vaporize
---

# Bob

Body.`
	pages := Parse(input)
	p := pages[0]
	if p.Status != StatusParseError {
		t.Fatalf("Status = %q; want StatusParseError", p.Status)
	}
	if !strings.Contains(p.ParseError, "action:") || !strings.Contains(p.ParseError, "vaporize") {
		t.Errorf("ParseError = %q; want 'action: vaporize is not valid' shape", p.ParseError)
	}
}

// TestParse_Action_Delete_BodyWarning — action=delete + non-empty body
// emits a "body ignored" warning (helpful for AI-prompt-quality
// feedback) but doesn't block the row.
func TestParse_Action_Delete_BodyWarning(t *testing.T) {
	input := `---
name: Old Entity
action: delete
---

# Old

This body will be ignored.`
	pages := Parse(input)
	p := pages[0]
	if p.Status == StatusParseError {
		t.Fatalf("delete + body should NOT be a parse error; got %q", p.ParseError)
	}
	found := false
	for _, w := range p.Warnings {
		if strings.Contains(w, "Body content is ignored") || strings.Contains(w, "body") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected body-ignored warning; got Warnings = %v", p.Warnings)
	}
}

// TestParse_Action_Delete_EmptyBodyNoEmptyWarning — action=delete +
// empty body should NOT emit the "Page body is empty" warning (delete
// rows don't need a body; warning would be noise).
func TestParse_Action_Delete_EmptyBodyNoEmptyWarning(t *testing.T) {
	input := `---
name: Old Entity
action: delete
---`
	pages := Parse(input)
	p := pages[0]
	for _, w := range p.Warnings {
		if strings.Contains(w, "body is empty") {
			t.Errorf("delete + empty body should NOT emit body-empty warning; got %q", w)
		}
	}
}

// TestParse_Action_NotInUnknownKeys — adding `action` to front-matter
// must not surface as an "Unknown front-matter key" warning (V1-F
// pattern; the known-keys list must include the new field).
func TestParse_Action_NotInUnknownKeys(t *testing.T) {
	input := `---
name: Bob
type: character
action: update
---

# Bob

Body.`
	pages := Parse(input)
	p := pages[0]
	for _, w := range p.Warnings {
		if strings.Contains(w, "Unknown front-matter") && strings.Contains(w, "action") {
			t.Errorf("`action` field should be in the known-keys list; got warning %q", w)
		}
	}
}
