package systems

import (
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
	player := houseRulesFrom(FilterBook(houseBook(), false), base)
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
	if player.BookHref != base {
		t.Errorf("BookHref = %q", player.BookHref)
	}
	if dm := houseRulesFrom(FilterBook(houseBook(), true), base); dm.Total != 4 {
		t.Errorf("the Director sees %d chapters, want 4", dm.Total)
	}

	big := &Book{Parts: []BookPart{{Title: housePartTitle}}}
	for i := 0; i < houseRulesShown+3; i++ {
		big.Parts[0].Chapters = append(big.Parts[0].Chapters, BookChapter{ID: "h", Title: "T"})
	}
	if v := houseRulesFrom(big, base); len(v.Rules) != houseRulesShown || v.Total != houseRulesShown+3 {
		t.Errorf("rules %d total %d, want %d and %d", len(v.Rules), v.Total, houseRulesShown, houseRulesShown+3)
	}
	if v := houseRulesFrom(nil, base); len(v.Rules) != 0 {
		t.Error("no book, no rules")
	}
}
