package importer

// parser_fences_test.go pins fence classification: a `---` fence is an
// opener only when followed by a YAML key line, otherwise a divider.
// Pairing fences blindly odd/even would let one horizontal rule in a
// body, or one unclosed front-matter block, shift the pairing and
// corrupt every page after it.

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

// One `---` between a block with no body and the next block both closes the
// first and opens the second; the second must never become the first one's
// body, where nothing reads it.
func TestSplitPages_SharedFenceBetweenBodilessBlocks(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		names  []string
		bodies []string
	}{
		{
			name:   "encounter then map, one fence between",
			input:  "---\nkind: encounter\nname: Ambush\ncreatures:\n  - 4 Raider, group A\n---\nkind: map\nname: Docks\ngrid: 40 x 26\n---\nwater \"Harbour\" 0,0 to 40,8\n",
			names:  []string{"Ambush", "Docks"},
			bodies: []string{"", "water \"Harbour\" 0,0 to 40,8"},
		},
		{
			name:   "three body-less blocks chained on single fences",
			input:  "---\nkind: tag\nname: A\n---\nkind: tag\nname: B\n---\nkind: tag\nname: C\n---\n",
			names:  []string{"A", "B", "C"},
			bodies: []string{"", "", ""},
		},
		{
			name:   "the usual two fences still split as before",
			input:  "---\nkind: tag\nname: A\n---\n---\nkind: tag\nname: B\n---\nBody B.\n",
			names:  []string{"A", "B"},
			bodies: []string{"", "Body B."},
		},
		{
			name:   "a body line like Weather: grim is still body text",
			input:  "---\nname: Port\n---\nWeather: grim\n\n---\n\nMore prose.\n",
			names:  []string{"Port"},
			bodies: []string{"Weather: grim\n\n---\n\nMore prose."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages := Parse(tt.input)
			if len(pages) != len(tt.names) {
				t.Fatalf("got %d blocks, want %d", len(pages), len(tt.names))
			}
			for i, p := range pages {
				if p.Status == StatusParseError {
					t.Errorf("block %d: parse error %q", i, p.ParseError)
				}
				if p.Name != tt.names[i] {
					t.Errorf("block %d name = %q, want %q", i, p.Name, tt.names[i])
				}
				if strings.TrimSpace(p.Body) != tt.bodies[i] {
					t.Errorf("block %d body = %q, want %q", i, p.Body, tt.bodies[i])
				}
			}
		})
	}
}

func TestParse_DropsHeadingThatRepeatsTheName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		body  string
	}{
		{"front matter name and the same heading", "---\nname: Odo Vance\ntype: character\n---\n# Odo Vance\n\nThe harbourmaster.", "The harbourmaster."},
		{"heading in other capitals", "---\nname: Odo Vance\n---\n#  odo vance \nText.", "Text."},
		{"a different heading stays", "---\nname: Odo Vance\n---\n# Early life\nText.", "# Early life\nText."},
		{"a later heading with the name stays", "---\nname: Odo\n---\nIntro.\n\n# Odo\nMore.", "Intro.\n\n# Odo\nMore."},
		{"no front matter: the heading names the page and is not repeated", "# Brinehold\n\nA port.", "A port."},
		{"a record's body too", "---\nkind: game-night\nname: Session 1\n---\n# Session 1\nRecap.", "Recap."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages := Parse(tt.input)
			if len(pages) != 1 {
				t.Fatalf("got %d blocks", len(pages))
			}
			if pages[0].Body != tt.body {
				t.Errorf("body = %q, want %q", pages[0].Body, tt.body)
			}
		})
	}
}
