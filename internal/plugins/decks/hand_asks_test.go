package decks

import (
	"strings"
	"testing"
)

func (w *handWorld) tableBody() string {
	return w.get("/campaigns/camp-1/decks/workshop/"+w.deck+"/table", "u-dir").Body.String()
}

// A Director sees, on each copy in a hand, whether it was turned over and
// whether it is shown; a card asked for says who asks, with both answers; the
// answers are real moves. A player is never sent any of it.
func TestTheDirectorsTableShowsAsksAndHowCopiesAreHeld(t *testing.T) {
	w := newHandWorld(t, true, nil)
	fx := w.fx
	if body := w.tableBody(); !strings.Contains(body, "still face down") || strings.Contains(body, "asks to play") {
		t.Fatalf("a fresh copy: %.400s", body)
	}
	fx.must(func() error {
		_, err := fx.svc.Act(bg, camp, w.deck, w.tamCopy, "turn", fx.tam, ActInput{})
		return err
	}())
	if body := w.tableBody(); !strings.Contains(body, "turned over, not shown") {
		t.Errorf("a turned copy is not marked: %.400s", body)
	}
	fx.must(func() error {
		_, err := fx.svc.Act(bg, camp, w.deck, w.tamCopy, "show", fx.tam, ActInput{})
		return err
	}())
	if body := w.tableBody(); !strings.Contains(body, "turned over and shown to the table") {
		t.Errorf("a shown copy is not marked: %.400s", body)
	}

	// Ask first: the card names who asks and offers both answers.
	_, err := fx.svc.UpdateCard(bg, camp, w.deck, w.card, fx.dir, UpdateCardInput{AskFirst: patchOf(true)})
	fx.must(err)
	fx.must(func() error {
		_, err := fx.svc.Act(bg, camp, w.deck, w.tamCopy, "play", fx.tam, ActInput{})
		return err
	}())
	body := w.tableBody()
	for _, want := range []string{"asks to play this", `data-act="approve"`, `data-act="turn-down"`, "Let them", "Not now"} {
		if !strings.Contains(body, want) {
			t.Errorf("the ask row lacks %q", want)
		}
	}
	// What a player is sent of the same table: not the Director's row.
	if rec := w.get("/campaigns/camp-1/decks/workshop/"+w.deck+"/table", "u-tam"); rec.Code == 200 {
		t.Errorf("a player read the Director's table row: %d", rec.Code)
	}

	// Not now: the card stays in the hand and can be asked again.
	fx.must(func() error {
		_, err := fx.svc.Act(bg, camp, w.deck, w.tamCopy, "turn-down", fx.dir, ActInput{})
		return err
	}())
	if c := fx.copy(w.tamCopy); c.Zone != ZoneHand || c.AskedAt != nil {
		t.Errorf("after Not now: %+v", c)
	}
	if strings.Contains(w.tableBody(), "asks to play this") {
		t.Error("the ask is still listed after it was turned down")
	}
	fx.must(func() error {
		_, err := fx.svc.Act(bg, camp, w.deck, w.tamCopy, "play", fx.tam, ActInput{})
		return err
	}())
	fx.must(func() error {
		_, err := fx.svc.Act(bg, camp, w.deck, w.tamCopy, "approve", fx.dir, ActInput{})
		return err
	}())
	if c := fx.copy(w.tamCopy); c.Zone == ZoneHand {
		t.Errorf("Let them left the card in the hand: %+v", c)
	}
}

// What a player draws shows the same in the fragment whether or not a Director
// set the next and last card.
func TestAPlayersDrawReadsTheSameWithNextAndLastSet(t *testing.T) {
	run := func(nudged bool) string {
		w := newHandWorld(t, true, &SettingsInput{Knows: ptr("count"), Arrives: ptr("down"), OthersSee: ptr("nothing"), Draws: ptr("players")})
		fx := w.fx
		fx.choose(w.deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)}, PlayerInput{UserID: "u-ilse"})
		if nudged {
			cards := fx.repo.cardsOf(w.deck)
			fx.must(fx.nudge(w.deck, patchOf(cards[0]), patchOf(cards[1])))
		}
		res, err := fx.svc.Draw(bg, camp, w.deck, fx.tam, DrawInput{HeroID: "h-tam", Count: 1})
		fx.must(err)
		if len(res.CopyIDs) != 1 {
			t.Fatalf("drew %d", len(res.CopyIDs))
		}
		return scrub(w.get(handBase, "u-tam").Body.String(), w.deck)
	}
	if a, b := run(false), run(true); a != b {
		t.Errorf("the hand after a draw differs with next and last set:\n A: %s\n B: %s", a, b)
	}
}
