package decks

import (
	"strings"
	"testing"
)

func TestHeldAndPlayCardsGroupByHolderInFirstSeenOrder(t *testing.T) {
	tv := tableView{
		Held: []heldCard{{CopyID: "1", Where: "Brannoc"}, {CopyID: "2", Where: "the party's hand"}, {CopyID: "3", Where: "Brannoc"}},
		Play: []playCard{{CopyID: "4", Where: "the table"}, {CopyID: "5", Where: "the table"}},
	}
	held := tv.heldGroups()
	if len(held) != 2 || held[0].Where != "Brannoc" || len(held[0].Cards) != 2 || held[1].Where != "the party's hand" || len(held[1].Cards) != 1 {
		t.Errorf("held groups = %+v", held)
	}
	if play := tv.playGroups(); len(play) != 1 || len(play[0].Cards) != 2 {
		t.Errorf("play groups = %+v", play)
	}
	if got := holdsWords(1); got != "holds 1 card" {
		t.Errorf("holdsWords(1) = %q", got)
	}
	if got := holdsWords(12); got != "holds 12 cards" {
		t.Errorf("holdsWords(12) = %q", got)
	}
	if got := upperFirst("the party's hand"); got != "The party's hand" {
		t.Errorf("upperFirst = %q", got)
	}
}

func TestStyleLineJoinsTheCommaToTheName(t *testing.T) {
	if got := styleLine("Hero's boons", true); got != "Hero's boons, changed" {
		t.Errorf("styleLine = %q", got)
	}
	if got := styleLine("Hero's boons", false); got != "Hero's boons" {
		t.Errorf("styleLine = %q", got)
	}
}

// A hand of many cards is one line naming the holder and one row of cards, not
// a row of its own for each; the Look tab's style line has no stray space.
func TestTableRowGroupsAHandAndLookLineHasNoSpaceBeforeTheComma(t *testing.T) {
	r := newWorkshopRig(t, true)
	card := ""
	for _, c := range r.fx.repo.cards {
		card = c.ID
	}
	for _, cp := range r.fx.copiesOf(card) {
		r.fx.place(cp, ZoneHand, "", true)
	}
	body := r.get("/campaigns/camp-1/decks/workshop/"+r.deck+"/table", "u-dir").Body.String()
	if n := strings.Count(body, `class="dkw-hand"`); n != 1 {
		t.Errorf("%d groups for one holder, want 1", n)
	}
	if !strings.Contains(body, "The party&#39;s hand") && !strings.Contains(body, "The party's hand") {
		t.Error("the group does not name its holder")
	}
	if !strings.Contains(body, "holds 3 cards") {
		t.Error("the group does not say how many are held")
	}
	if !strings.Contains(body, `data-dkw-strip`) {
		t.Error("the held cards are not in a row that scrolls inside itself")
	}
	look := r.get("/campaigns/camp-1/decks/workshop/"+r.deck+"/look", "u-dir", "HX-Request", "true").Body.String()
	if strings.Contains(look, " , changed") || strings.Contains(look, "\n, changed") {
		t.Error("the style line has whitespace before the comma")
	}
}
