// parser_test.go pins the multi-page split + per-page front-matter
// parsing behavior. Test inputs mimic real AI output: mixed FM /
// no-FM, malformed YAML, invalid enum, multi-page with FM, fallback
// H1 split when no FM anywhere.
//
// Stop-and-flag check (scoping §1.4): real Claude/ChatGPT output
// samples may differ from these fixtures. Phase 5's smoke test
// expands the fixture set.
package importer

import (
	"strings"
	"testing"
)

func TestParse_EmptyInput(t *testing.T) {
	if got := Parse(""); len(got) != 0 {
		t.Errorf("empty input → %d pages, want 0", len(got))
	}
	if got := Parse("   \n\n  "); len(got) != 0 {
		t.Errorf("whitespace input → %d pages, want 0", len(got))
	}
}

// TestParse_SinglePage_FullFrontMatter is the happy-path canonical:
// one page, all FM fields populated, well-formed body. Status=New.
func TestParse_SinglePage_FullFrontMatter(t *testing.T) {
	input := `---
name: Goblin Camp
type: location
subcategory: encampment
visibility: private
tags: [goblins, hostile]
---

# Goblin Camp

A ruined goblin camp in the Drowned Reach.`

	pages := Parse(input)
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}
	p := pages[0]
	if p.Status != StatusNew {
		t.Errorf("Status = %q, want StatusNew", p.Status)
	}
	if p.Name != "Goblin Camp" {
		t.Errorf("Name = %q, want 'Goblin Camp'", p.Name)
	}
	if p.FrontMatter.Type != "location" {
		t.Errorf("Type = %q, want 'location'", p.FrontMatter.Type)
	}
	if p.FrontMatter.Subcategory != "encampment" {
		t.Errorf("Subcategory = %q", p.FrontMatter.Subcategory)
	}
	if p.FrontMatter.Visibility != "private" {
		t.Errorf("Visibility = %q", p.FrontMatter.Visibility)
	}
	if len(p.FrontMatter.Tags) != 2 {
		t.Errorf("Tags = %v", p.FrontMatter.Tags)
	}
	if !strings.Contains(p.Body, "A ruined goblin camp") {
		t.Errorf("Body missing body text:\n%s", p.Body)
	}
	if p.ParseError != "" {
		t.Errorf("expected no parse error; got %q", p.ParseError)
	}
}

// TestParse_SinglePage_NoFrontMatter_H1AsName falls back to H1.
func TestParse_SinglePage_NoFrontMatter_H1AsName(t *testing.T) {
	input := `# Sigil

A wind elemental bonded to Lyra Vance.`
	pages := Parse(input)
	if len(pages) != 1 {
		t.Fatalf("got %d pages", len(pages))
	}
	p := pages[0]
	if p.HasFrontMatter {
		t.Errorf("HasFrontMatter true when no `---` block present")
	}
	if p.Name != "Sigil" {
		t.Errorf("Name = %q, want 'Sigil'", p.Name)
	}
	if p.Status != StatusNew {
		t.Errorf("Status = %q, want StatusNew", p.Status)
	}
}

// TestParse_NoName_ParseError fires when there's neither a `name:`
// FM key NOR an H1 heading.
func TestParse_NoName_ParseError(t *testing.T) {
	input := `---
type: location
---

Some body text but no heading.`
	pages := Parse(input)
	if len(pages) != 1 {
		t.Fatalf("got %d pages", len(pages))
	}
	p := pages[0]
	if p.Status != StatusParseError {
		t.Errorf("Status = %q, want StatusParseError", p.Status)
	}
	if !strings.Contains(p.ParseError, "Could not determine page name") {
		t.Errorf("ParseError = %q", p.ParseError)
	}
}

// TestParse_InvalidVisibility_ParseError fires on a typo / wrong-case
// visibility value. The error message includes the bad value so
// the operator can fix the source.
func TestParse_InvalidVisibility_ParseError(t *testing.T) {
	input := `---
name: Bone Citadel
type: location
visibility: PUBLIC
---

# Bone Citadel`
	pages := Parse(input)
	p := pages[0]
	if p.Status != StatusParseError {
		t.Errorf("Status = %q, want StatusParseError", p.Status)
	}
	if !strings.Contains(p.ParseError, "PUBLIC") {
		t.Errorf("ParseError should quote the bad value; got %q", p.ParseError)
	}
	if !strings.Contains(p.ParseError, "private") {
		t.Errorf("ParseError should list valid values; got %q", p.ParseError)
	}
}

// TestParse_MalformedFrontMatter_ParseError catches bad YAML.
func TestParse_MalformedFrontMatter_ParseError(t *testing.T) {
	input := `---
name: "unterminated string
type: location
---

# X`
	pages := Parse(input)
	p := pages[0]
	if p.Status != StatusParseError {
		t.Errorf("Status = %q, want StatusParseError; full: %+v", p.Status, p)
	}
	if !strings.Contains(p.ParseError, "front-matter YAML") {
		t.Errorf("ParseError = %q", p.ParseError)
	}
}

// TestParse_UnknownYAMLKeys_Warning surfaces unknown keys via the
// Warnings slice without blocking import.
func TestParse_UnknownYAMLKeys_Warning(t *testing.T) {
	input := `---
name: Sigil
type: creature
mood: cheerful
ai_generated_field: yes
---

# Sigil`
	pages := Parse(input)
	p := pages[0]
	if p.Status != StatusNew {
		t.Errorf("Status = %q, want StatusNew (unknown keys shouldn't block)", p.Status)
	}
	if len(p.Warnings) == 0 {
		t.Errorf("expected warnings about unknown keys; got none")
	}
	combined := strings.Join(p.Warnings, " ")
	if !strings.Contains(combined, "mood") || !strings.Contains(combined, "ai_generated_field") {
		t.Errorf("warnings missing one of mood/ai_generated_field: %v", p.Warnings)
	}
}

// TestParse_NoH1NoName_WarnsWhenFMPresent flags the operator that
// the H1 wasn't usable + the name fell back to FM `name:`.
func TestParse_NoFMNameButHasH1_WarnsAboutFallback(t *testing.T) {
	input := `---
type: character
---

# Lyra Vance

Body.`
	pages := Parse(input)
	p := pages[0]
	if p.Status != StatusNew {
		t.Errorf("Status = %q, want StatusNew", p.Status)
	}
	if p.Name != "Lyra Vance" {
		t.Errorf("Name = %q, want 'Lyra Vance' from H1", p.Name)
	}
	found := false
	for _, w := range p.Warnings {
		if strings.Contains(w, "No `name:`") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a warning about missing FM name; got %v", p.Warnings)
	}
}

// TestParse_MultiPage_FrontMatterSplit pins the canonical multi-
// page split — `---` opener is the boundary; H1 inside a body
// doesn't false-split.
func TestParse_MultiPage_FrontMatterSplit(t *testing.T) {
	input := `---
name: Page One
type: character
---

# Page One

Body of page one.

Some text with a `+ "`#`" + ` symbol that isn't an H1.

---
name: Page Two
type: location
---

# Page Two

Body of page two.

---
name: Page Three
type: item
---

# Page Three

Body.`
	pages := Parse(input)
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
	wantNames := []string{"Page One", "Page Two", "Page Three"}
	wantTypes := []string{"character", "location", "item"}
	for i, p := range pages {
		if p.Name != wantNames[i] {
			t.Errorf("pages[%d].Name = %q, want %q", i, p.Name, wantNames[i])
		}
		if p.FrontMatter.Type != wantTypes[i] {
			t.Errorf("pages[%d].Type = %q, want %q", i, p.FrontMatter.Type, wantTypes[i])
		}
		if p.Status != StatusNew {
			t.Errorf("pages[%d].Status = %q, want StatusNew", i, p.Status)
		}
	}
	if !strings.Contains(pages[0].Body, "Body of page one") {
		t.Errorf("pages[0] body lost text")
	}
}

// TestParse_MultiPage_H1Fallback exercises the no-FM-anywhere
// fallback: split on `# Heading`.
func TestParse_MultiPage_H1Fallback(t *testing.T) {
	input := `# Page A

Body A.

# Page B

Body B.`
	pages := Parse(input)
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].Name != "Page A" || pages[1].Name != "Page B" {
		t.Errorf("names: %q, %q", pages[0].Name, pages[1].Name)
	}
}

// TestParse_PreambleBeforeFirstFMDropped — AI tools sometimes
// disregard the §3.5 prompt's "no preamble" instruction. The
// parser drops anything before the first `---` opener.
func TestParse_PreambleBeforeFirstFMDropped(t *testing.T) {
	input := `Here are three new NPCs for your campaign:

---
name: First NPC
type: character
---

# First NPC

Body.`
	pages := Parse(input)
	if len(pages) != 1 {
		t.Fatalf("got %d pages", len(pages))
	}
	if pages[0].Name != "First NPC" {
		t.Errorf("Name = %q", pages[0].Name)
	}
	if strings.Contains(pages[0].Body, "Here are three") {
		t.Errorf("preamble leaked into body")
	}
}

// TestParse_MixedStatesPerScopingMockup is the dispatch's "smoke
// test" condition — three pages with mixed states:
//   - Full FM (StatusNew)
//   - No FM + no H1 (StatusParseError — no name)
//   - Invalid visibility (StatusParseError)
func TestParse_MixedStatesPerScopingMockup(t *testing.T) {
	input := `---
name: Maro Halvi
type: character
visibility: private
---

# Maro Halvi

Body.

---
type: location
---

Page with FM but no name and no H1.

---
name: Bone Citadel
type: location
visibility: PUBLIC
---

# Bone Citadel

Body.`
	pages := Parse(input)
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}
	want := []ParseStatus{StatusNew, StatusParseError, StatusParseError}
	for i, p := range pages {
		if p.Status != want[i] {
			t.Errorf("pages[%d].Status = %q, want %q", i, p.Status, want[i])
		}
	}
	if pages[0].Name != "Maro Halvi" {
		t.Errorf("pages[0].Name = %q", pages[0].Name)
	}
	if !strings.Contains(pages[2].ParseError, "PUBLIC") {
		t.Errorf("pages[2] should flag the bad visibility value")
	}
}
