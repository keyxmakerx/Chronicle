package importer

// parser_fences_test.go — pins the fence-classification fix (operator,
// 2026-06-12: "failed to parse due to incorrect --- formatting"). The
// old splitter paired `---` fences blindly odd/even, so ONE horizontal
// rule in a body — or one unclosed front-matter block — shifted the
// pairing and corrupted every page after it. Fences are now classified
// by content: opener iff followed by a YAML key line; divider otherwise.

import (
	"strings"
	"testing"
)

func TestSplitPages_HorizontalRuleInBodyDoesNotShiftPages(t *testing.T) {
	input := "---\nname: Alpha\n---\nIntro text.\n\n---\n\nMore prose after a divider.\n\n---\nname: Beta\n---\nBeta body."
	pages := Parse(input)
	if len(pages) != 2 {
		t.Fatalf("want 2 pages, got %d", len(pages))
	}
	if pages[0].Name != "Alpha" || pages[1].Name != "Beta" {
		t.Errorf("names = %q, %q; want Alpha, Beta", pages[0].Name, pages[1].Name)
	}
	if pages[0].Status == StatusParseError || pages[1].Status == StatusParseError {
		t.Errorf("no page should error: %q / %q", pages[0].ParseError, pages[1].ParseError)
	}
	if !strings.Contains(pages[0].Body, "More prose after a divider.") {
		t.Errorf("Alpha's body lost the post-divider prose: %q", pages[0].Body)
	}
}

func TestSplitPages_UnclosedFrontMatterDamageIsContained(t *testing.T) {
	// Beta's closing fence is missing. Beta must error — and ONLY Beta;
	// Alpha and Gamma parse fine (the old pairing corrupted Gamma too).
	input := "---\nname: Alpha\n---\nAlpha body.\n\n---\nname: Beta\nBeta body without a closer.\n\n---\nname: Gamma\n---\nGamma body."
	pages := Parse(input)
	if len(pages) != 3 {
		t.Fatalf("want 3 pages, got %d", len(pages))
	}
	if pages[0].Name != "Alpha" || pages[0].Status == StatusParseError {
		t.Errorf("Alpha must parse clean: %q", pages[0].ParseError)
	}
	if pages[1].Status != StatusParseError || !strings.Contains(pages[1].ParseError, "closing `---`") {
		t.Errorf("Beta must report its missing closer, got status=%q err=%q", pages[1].Status, pages[1].ParseError)
	}
	if pages[2].Name != "Gamma" || pages[2].Status == StatusParseError {
		t.Errorf("Gamma must parse clean despite Beta's error: %q", pages[2].ParseError)
	}
}

func TestSplitPages_FourDashFencesAndCRLF(t *testing.T) {
	input := "----\r\nname: Alpha\r\n----\r\nAlpha body.\r\n"
	pages := Parse(input)
	if len(pages) != 1 {
		t.Fatalf("want 1 page, got %d", len(pages))
	}
	if pages[0].Name != "Alpha" || pages[0].Status == StatusParseError {
		t.Errorf("4-dash CRLF page must parse: name=%q err=%q", pages[0].Name, pages[0].ParseError)
	}
}

func TestSplitPages_LeadingDividerBeforeProseIsNotAPage(t *testing.T) {
	// A divider whose next line is prose must not open a page even at
	// input start — the H1 fallback handles naming instead.
	input := "---\n\nJust prose under a decorative rule.\n\n# Solo Page\nbody"
	pages := Parse(input)
	if len(pages) != 1 {
		t.Fatalf("want 1 page via H1 fallback, got %d", len(pages))
	}
	if pages[0].Name != "Solo Page" {
		t.Errorf("name = %q; want Solo Page", pages[0].Name)
	}
}
