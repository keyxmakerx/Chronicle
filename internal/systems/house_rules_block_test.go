package systems

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func houseBook() *Book {
	page := func(director bool) []BookPage {
		return []BookPage{{Director: director, Blocks: []BookBlock{{Type: "text", Text: "x"}}}}
	}
	return &Book{Parts: []BookPart{
		{Title: "Player's book", Chapters: []BookChapter{{ID: "combat", Title: "Combat", Pages: page(false)}}},
		{Title: housePartTitle, Chapters: []BookChapter{
			{ID: "house-potions", Title: "Potions", Intro: "Drinking one is a maneuver.", Pages: page(false)},
			{ID: "house-secret", Title: "Director's notes", Director: true, Pages: page(false)},
			{ID: "house-only-director-pages", Title: "Hidden pages", Pages: page(true)},
			{ID: "house-flank", Title: "Flanking", Pages: page(false)},
		}},
	}}
}

// The block lists only the house chapters, and only those the viewer's book
// still holds: a Director-only chapter, or one whose pages are all
// Director-only, is gone for a player and listed for the Director.
func TestHouseRulesFrom(t *testing.T) {
	base := "/campaigns/c1/systems/drawsteel"
	player := houseRulesFrom(FilterBook(houseBook(), false), base, "c1", false)
	var titles []string
	for _, r := range player.Rules {
		titles = append(titles, r.Title)
	}
	if got := strings.Join(titles, ","); got != "Potions,Flanking" || player.Total != 2 {
		t.Errorf("player sees %q (total %d), want Potions,Flanking", got, player.Total)
	}
	if player.Rules[0].Href != base+"#house-potions" || player.Rules[0].Intro != "Drinking one is a maneuver." {
		t.Errorf("first rule = %+v", player.Rules[0])
	}
	if player.Rules[0].ChapterID != "house-potions" {
		t.Errorf("first rule's chapter = %q", player.Rules[0].ChapterID)
	}
	if player.BookHref != base {
		t.Errorf("BookHref = %q", player.BookHref)
	}
	if dm := houseRulesFrom(FilterBook(houseBook(), true), base, "c1", false); dm.Total != 4 {
		t.Errorf("the Director sees %d chapters, want 4", dm.Total)
	}

	big := &Book{Parts: []BookPart{{Title: housePartTitle}}}
	for i := 0; i < houseRulesShown+3; i++ {
		big.Parts[0].Chapters = append(big.Parts[0].Chapters, BookChapter{ID: "h", Title: "T"})
	}
	if v := houseRulesFrom(big, base, "c1", false); len(v.Rules) != houseRulesShown || v.Total != houseRulesShown+3 {
		t.Errorf("rules %d total %d, want %d and %d", len(v.Rules), v.Total, houseRulesShown, houseRulesShown+3)
	}
	if v := houseRulesFrom(nil, base, "c1", false); len(v.Rules) != 0 {
		t.Error("no book, no rules")
	}
}

// Every leaf reads one book address, and a Director who is viewing as a player
// asks for the player's book there; the chapter each leaf draws and the link
// into the full book follow the chapter.
func TestHouseRulesFromLeafFields(t *testing.T) {
	base := "/campaigns/c1/systems/drawsteel"
	tests := []struct {
		name       string
		asPlayer   bool
		wantBook   string
		wantFirst  string
		wantHref   string
		wantCampID string
	}{
		{"read as oneself", false, base + "/book", "house-potions", base + "#house-potions", "c1"},
		{"viewing as a player", true, base + "/book?view=player", "house-potions", base + "#house-potions", "c1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := houseRulesFrom(FilterBook(houseBook(), true), base, "c1", tt.asPlayer)
			if v.BookURL != tt.wantBook || v.CampaignID != tt.wantCampID {
				t.Errorf("BookURL %q campaign %q, want %q %q", v.BookURL, v.CampaignID, tt.wantBook, tt.wantCampID)
			}
			if len(v.Rules) == 0 || v.Rules[0].ChapterID != tt.wantFirst || v.Rules[0].Href != tt.wantHref {
				t.Errorf("first rule = %+v", v.Rules)
			}
		})
	}
}

// Each rule opens to its introduction and then a leaf; the old link into the
// book is gone from the rule (the leaf carries "Open the book at this page"),
// and the block header still links to the whole rulebook.
func TestHouseRulesBlockDrawsLeaves(t *testing.T) {
	base := "/campaigns/c1/systems/drawsteel"
	v := houseRulesFrom(FilterBook(houseBook(), false), base, "c1", false)
	var buf bytes.Buffer
	if err := houseRulesBlock(v).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		`data-widget="rulebook"`,
		`data-leaf="house-potions"`,
		`data-leaf="house-flank"`,
		`data-book-url="` + base + `/book"`,
		`data-open-href="` + base + `#house-potions"`,
		`data-campaign-id="c1"`,
		`rulebook.css`,
		`Drinking one is a maneuver.`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Read it in the rulebook") {
		t.Error("the per-rule rulebook link is still drawn")
	}
	if n := strings.Count(out, `data-widget="rulebook"`); n != 2 {
		t.Errorf("%d leaves drawn, want 2", n)
	}
	if !strings.Contains(out, `href="`+base+`"`) {
		t.Error("the header's link to the whole rulebook is gone")
	}
}
