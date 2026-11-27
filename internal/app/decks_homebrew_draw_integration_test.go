package app

// decks_homebrew_draw_integration_test.go runs deck.draw over the real decks,
// homebrew and trackers services on a scratch database (storyWorld). Skipped
// under -short and without a database.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// drawRecipe saves a recipe with a Doom tracker, a deck part "omens" on deck
// and these rules.
func (w *storyWorld) drawRecipe(deck string, rules ...homebrew.Rule) *homebrew.Recipe {
	w.t.Helper()
	r, err := w.homebrew.Create(storyCtx, w.camp, w.hbDirector, homebrew.RecipeInput{Name: "Draws", Doc: homebrew.Doc{
		Parts: []homebrew.Part{
			{Key: "doom", Kind: "tracker", Label: "Doom", See: homebrew.SeeParty, Data: json.RawMessage(`{"shape":"count"}`)},
			{Key: "omens", Kind: deckPartKind, Label: "Omens", See: homebrew.SeeParty, Data: json.RawMessage(fmt.Sprintf(`{"deckId":%q}`, deck))}},
		Rules: rules}})
	w.must(err)
	return r
}

func drawFrom(n int) homebrew.Do {
	return homebrew.Do{Action: deckDraw, Part: "omens", Args: json.RawMessage(fmt.Sprintf(`{"count":%d}`, n))}
}

// zones counts a deck's copies by zone, and the hand by holder.
func (w *storyWorld) zones(deck string) map[string]int {
	w.t.Helper()
	out := map[string]int{}
	for _, c := range w.view(deck).Copies {
		out[c.Zone]++
		if c.HolderEntityID != "" {
			out[c.Zone+":"+c.HolderEntityID]++
		}
	}
	return out
}

func (w *storyWorld) runOnlyHeld() *homebrew.RunView {
	w.t.Helper()
	d := w.drawer()
	if len(d.Waiting) != 1 {
		w.t.Fatalf("waiting = %d, want exactly one held rule", len(d.Waiting))
	}
	v, err := w.homebrew.RunHeld(storyCtx, w.camp, d.Waiting[0].ID, w.hbDirector)
	w.must(err)
	return v
}

func TestDeckDrawDealsToTheHeroWhoSetItOffThroughTheDecksOwnDraw(t *testing.T) {
	w := newStoryWorld(t)
	// An event deck nobody was chosen for: only Directors see it until a card
	// of it is in a hero's hand.
	omens := w.deck("Omens", decks.KindEvent, "", nil)
	w.card(omens, "Ash", 2, decks.CreateCardInput{})
	w.card(omens, "Dust", 1, decks.CreateCardInput{})
	r := w.drawRecipe(omens,
		rule(cardPlayedEvent, `{"card":"Fire"}`, homebrew.TimingNow, drawFrom(2)),
		rule(cardDrawnEvent, `{"deck":"Omens"}`, homebrew.TimingNow, doomBy(1)))
	if got := r.Doc.Parts[1]; got.See != homebrew.SeeDirector || string(got.Data) != fmt.Sprintf(`{"deckId":%q,"name":"Omens"}`, omens) {
		t.Fatalf("deck part stored as %+v", got)
	}
	w.capture.rings = nil

	w.story.CardPlayed(storyCtx, w.camp, decks.CardEvent{DeckName: "Boons", CardName: "Fire", HeroID: w.tamHero, By: decks.Viewer{UserID: w.tamUser}})
	if z := w.zones(omens); z[decks.ZonePile] != 3 {
		t.Fatalf("the rule dealt before a Director pressed Run: %v", z)
	}
	if d := w.drawer(); len(d.Waiting) != 1 || d.Waiting[0].Waits != "Part of it is your call, so it waits for you." {
		t.Fatalf("a rule that draws is held however it is timed: %+v", d.Waiting)
	}

	v := w.runOnlyHeld()
	if v.Status != homebrew.RunDone || strings.Join(v.Lines, "|") != "Dealt 2 cards from Omens to Tam's hand." {
		t.Fatalf("run = %+v", v)
	}
	z := w.zones(omens)
	if z[decks.ZonePile] != 1 || z[decks.ZoneHand+":"+w.tamHero] != 2 || z[decks.ZoneShown] != 0 {
		t.Errorf("zones = %v; want two in Tam's hand, one in the pile, none left shown", z)
	}
	// The bell tells Tam, and only Tam, as a Director's deal does.
	var dealt []string
	for _, b := range w.capture.rings {
		if b.Kind == decks.BellDealt {
			dealt = append(dealt, strings.Join(b.Users, ","))
		}
		for _, u := range b.Users {
			if u == w.ilseUser {
				t.Errorf("Ilse was told: %+v", b)
			}
		}
	}
	if strings.Join(dealt, "|") != w.tamUser+"|"+w.tamUser {
		t.Errorf("dealt bells = %q, want one per card for Tam", dealt)
	}
	// card.drawn fired once per card, with Tam's page as scope.
	if got := w.doomOn(w.tamHero); got != 2 {
		t.Errorf("Doom on Tam = %d, want 2 (card.drawn once per card)", got)
	}
	// The deck is still hidden from a player who holds none of it.
	list, err := w.decks.List(storyCtx, w.camp, w.ilse)
	w.must(err)
	for _, d := range list.(decks.PlayerList).Decks {
		if d.ID == omens {
			t.Error("Ilse can see Omens")
		}
	}
}

func TestDeckDrawRefusesWithTheDecksOwnWordsOrForNoHero(t *testing.T) {
	w := newStoryWorld(t)
	omens := w.deck("Omens", decks.KindEvent, "", nil)
	only := w.card(omens, "Ash", 1, decks.CreateCardInput{})
	w.drawRecipe(omens, rule(cardPlayedEvent, `{}`, homebrew.TimingNow, drawFrom(1)))
	play := func(hero string) {
		w.story.CardPlayed(storyCtx, w.camp, decks.CardEvent{DeckName: "Boons", CardName: "Fire", HeroID: hero, By: decks.Viewer{UserID: w.tamUser}})
	}

	// From the party's hand there is no hero to draw for.
	play("")
	if v := w.runOnlyHeld(); v.Status != homebrew.RunFailed || !strings.Contains(strings.Join(v.Lines, "|"), "did not run: There is no hero to draw for") {
		t.Errorf("no scope: %+v", v)
	}
	if z := w.zones(omens); z[decks.ZonePile] != 1 {
		t.Errorf("a refused draw moved a card: %v", z)
	}

	w.deal(omens, only, w.orrinHero)
	play(w.tamHero)
	if v := w.runOnlyHeld(); v.Status != homebrew.RunFailed || strings.Join(v.Lines, "|") != "Draw cards for the hero did not run: Nothing left in this deck." {
		t.Errorf("empty deck: %+v", v)
	}
}

// A rule set off by its own draw is held again, so each further draw needs a
// Director's press and nothing loops; the 50-run budget never comes into it.
func TestADrawThatSetsOffItsOwnRuleWaitsInsteadOfLooping(t *testing.T) {
	w := newStoryWorld(t)
	omens := w.deck("Omens", decks.KindEvent, "", nil)
	w.card(omens, "Ash", 4, decks.CreateCardInput{})
	w.drawRecipe(omens,
		rule(cardPlayedEvent, `{"card":"Fire"}`, homebrew.TimingNow, drawFrom(1)),
		rule(cardDrawnEvent, `{"deck":"Omens"}`, homebrew.TimingNow, drawFrom(1)))
	w.story.CardPlayed(storyCtx, w.camp, decks.CardEvent{DeckName: "Boons", CardName: "Fire", HeroID: w.tamHero, By: decks.Viewer{UserID: w.tamUser}})
	for press := 1; press <= 3; press++ {
		if v := w.runOnlyHeld(); v.Status != homebrew.RunDone {
			t.Fatalf("press %d: %+v", press, v)
		}
		if got := w.zones(omens)[decks.ZoneHand+":"+w.tamHero]; got != press {
			t.Fatalf("after %d presses Tam holds %d cards", press, got)
		}
	}
}
