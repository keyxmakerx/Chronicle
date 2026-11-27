package app

// decks_play_ask_integration_test.go runs an "ask first" card's play as the
// Workshop's ask, over the real decks, homebrew, trackers and entities
// services on a scratch database. Skipped under -short and without one.

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

func appCodeOf(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// playAskWorld deals Pact, an ask-first card of Tam's Omens deck, to Tam,
// with a recipe whose rule on card.play-asked steps Doom by 2 and shuffles a
// copy of Pact into Omens on Yes, and Tam asks to play it.
func playAskWorld(t *testing.T) (w *storyWorld, deck, pact, copyID string) {
	t.Helper()
	w = newStoryWorld(t)
	deck = w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Arrives: ptrTo("up")})
	pact = w.card(deck, "Pact", 1, decks.CreateCardInput{AskFirst: true})
	w.recipe(rule(cardPlayAsked, `{"card":"pact"}`, homebrew.TimingNow,
		doomBy(2), doAct(deckAddCopies, `{"deck":"Omens","card":"Pact","copies":1}`)))
	copyID = w.deal(deck, pact, w.tamHero)
	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	return w, deck, pact, copyID
}

// theAsk is the one ask waiting in the drawer.
func (w *storyWorld) theAsk() homebrew.RunView {
	w.t.Helper()
	d := w.drawer()
	if len(d.Waiting) != 1 || d.Waiting[0].Ask == nil {
		w.t.Fatalf("waiting = %+v", d.Waiting)
	}
	return d.Waiting[0]
}

func (w *storyWorld) zoneOf(deck, copyID string) (string, bool) {
	w.t.Helper()
	for _, c := range w.view(deck).Copies {
		if c.ID == copyID {
			return c.Zone, c.AskedAt != nil
		}
	}
	return "", false
}

func TestAnAskFirstPlayIsTheWorkshopsAsk(t *testing.T) {
	type answer struct {
		drawer *homebrew.AnswerInput // answered in the drawer
		verb   string                // or on the deck's page
	}
	cases := []struct {
		name      string
		answer    answer
		wantZone  string
		wantDoom  int
		wantCopy  int
		wantState string
		wantBell  string
		detail    string
	}{
		{name: "yes in the drawer plays it and runs the rule", answer: answer{drawer: &homebrew.AnswerInput{Answer: homebrew.AnswerYes}},
			wantZone: decks.ZoneUsed, wantDoom: 2, wantCopy: 2, wantState: homebrew.RunDone, wantBell: "Your Director let you play Pact"},
		{name: "yes, and changes the rule's step", answer: answer{drawer: &homebrew.AnswerInput{Answer: homebrew.AnswerYes,
			Steps: []homebrew.StepEdit{{Step: 1, Args: []byte(`{"by":5}`)}}}},
			wantZone: decks.ZoneUsed, wantDoom: 5, wantCopy: 2, wantState: homebrew.RunDone, wantBell: "Your Director let you play Pact"},
		{name: "no turns it down", answer: answer{drawer: &homebrew.AnswerInput{Answer: homebrew.AnswerNo}},
			wantZone: decks.ZoneHand, wantDoom: -1, wantCopy: 1, wantState: homebrew.RunDeclined, wantBell: "Your Director turned down your ask to play Pact"},
		{name: "other turns it down with the reply", answer: answer{drawer: &homebrew.AnswerInput{Answer: homebrew.AnswerOther, Reply: "Not on holy ground."}},
			wantZone: decks.ZoneHand, wantDoom: -1, wantCopy: 1, wantState: homebrew.RunReplied,
			wantBell: "Your Director replied to your ask to play Pact", detail: "Not on holy ground."},
		{name: "let them on the deck's page answers the same ask", answer: answer{verb: "approve"},
			wantZone: decks.ZoneUsed, wantDoom: 2, wantCopy: 2, wantState: homebrew.RunDone, wantBell: "Your Director let you play Pact"},
		{name: "not now on the deck's page answers the same ask", answer: answer{verb: "turn-down"},
			wantZone: decks.ZoneHand, wantDoom: -1, wantCopy: 1, wantState: homebrew.RunDeclined, wantBell: "Your Director turned down your ask to play Pact"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, deck, pact, copyID := playAskWorld(t)
			if zone, asked := w.zoneOf(deck, copyID); zone != decks.ZoneHand || !asked {
				t.Fatalf("an asked play waits in the hand: %s asked=%v", zone, asked)
			}
			ask := w.theAsk()
			if ask.Recipe != "Card decks" || ask.Ask.Asker != "Tam" || ask.Ask.Ask != "play Pact from Omens" || len(ask.Ask.Later) != 2 {
				t.Errorf("the drawer's card = %+v / %+v", ask, ask.Ask)
			}
			if w.doomOn(w.tamHero) != -1 || len(w.drawer().Recent) != 0 {
				t.Fatal("something ran before the answer")
			}
			// The deck rang the Directors; the Workshop rang nobody.
			if len(w.asks.rings) != 0 {
				t.Errorf("the Workshop rang about the deck's ask: %+v", w.asks.rings)
			}
			w.capture.rings = nil

			// A player can't answer, in either place.
			tam := homebrew.Viewer{UserID: w.tamUser, VisibilityRole: 1}
			if _, err := w.homebrew.Answer(storyCtx, w.camp, ask.ID, tam, homebrew.AnswerInput{Answer: homebrew.AnswerYes}); appCodeOf(err) != 403 {
				t.Errorf("a player answered in the drawer: %v", err)
			}
			if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "approve", w.tam, decks.ActInput{}); appCodeOf(err) != 403 {
				t.Errorf("a player answered on the deck: %v", err)
			}

			var err error
			if tc.answer.drawer != nil {
				_, err = w.homebrew.Answer(storyCtx, w.camp, ask.ID, w.hbDirector, *tc.answer.drawer)
			} else {
				_, err = w.decks.Act(storyCtx, w.camp, deck, copyID, tc.answer.verb, w.dir, decks.ActInput{})
			}
			w.must(err)

			if zone, asked := w.zoneOf(deck, copyID); zone != tc.wantZone || asked {
				t.Errorf("the card is %s asked=%v, want %s", zone, asked, tc.wantZone)
			}
			if got := w.doomOn(w.tamHero); got != tc.wantDoom {
				t.Errorf("doom = %d, want %d", got, tc.wantDoom)
			}
			if got := w.copiesOf(deck, pact); got != tc.wantCopy {
				t.Errorf("copies = %d, want %d", got, tc.wantCopy)
			}
			d := w.drawer()
			if len(d.Waiting) != 0 {
				t.Errorf("answered in one place, it still waits: %+v", d.Waiting)
			}
			var state string
			for _, r := range d.Recent {
				if r.ID == ask.ID {
					state = r.Status
				}
			}
			if state != tc.wantState {
				t.Errorf("the ask is %q, want %q", state, tc.wantState)
			}
			var bell *decks.BellEntry
			for i, r := range w.capture.rings {
				if r.Kind == decks.BellAnswered {
					bell = &w.capture.rings[i]
				}
			}
			if bell == nil || bell.Message != tc.wantBell || bell.Detail != tc.detail || len(bell.Users) != 1 || bell.Users[0] != w.tamUser {
				t.Errorf("the player's bell = %+v", w.capture.rings)
			}
			if len(w.asks.rings) != 0 {
				t.Errorf("the Workshop rang about the deck's answer: %+v", w.asks.rings)
			}
			// Answered once: neither place takes a second answer.
			if _, err := w.homebrew.Answer(storyCtx, w.camp, ask.ID, w.hbDirector, homebrew.AnswerInput{Answer: homebrew.AnswerNo}); appCodeOf(err) != 409 {
				t.Errorf("a second answer in the drawer: %v", err)
			}
			for _, verb := range []string{"approve", "turn-down"} {
				if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, verb, w.dir, decks.ActInput{}); err == nil {
					t.Errorf("a second %s on the deck was taken", verb)
				}
			}
		})
	}
}

func TestTheDeckPageAndTheDrawerCantBothAnswer(t *testing.T) {
	w, deck, _, copyID := playAskWorld(t)
	ask := w.theAsk()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = w.homebrew.Answer(storyCtx, w.camp, ask.ID, w.hbDirector, homebrew.AnswerInput{Answer: homebrew.AnswerYes})
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = w.decks.Act(storyCtx, w.camp, deck, copyID, "turn-down", w.dir, decks.ActInput{})
	}()
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("answers taken = %d (%v)", ok, errs)
	}
	zone, asked := w.zoneOf(deck, copyID)
	if asked || (errs[0] == nil && zone != decks.ZoneUsed) || (errs[1] == nil && zone != decks.ZoneHand) {
		t.Errorf("the card is %s asked=%v after %v", zone, asked, errs)
	}
}

func TestAPlayAskedBeforeTheChangeIsStillAnswered(t *testing.T) {
	w := newStoryWorld(t)
	deck := w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Arrives: ptrTo("up")})
	pact := w.card(deck, "Pact", 1, decks.CreateCardInput{AskFirst: true})
	copyID := w.deal(deck, pact, w.tamHero)
	// The same database, served as before plays were put to the Workshop.
	before := decks.NewService(decks.NewRepository(w.db), w.heroes, w.members, decks.WithRandom(func(int) int { return 0 }))
	if _, err := before.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	if d := w.drawer(); len(d.Waiting) != 0 {
		t.Fatalf("an old play has no ask: %+v", d.Waiting)
	}
	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "approve", w.dir, decks.ActInput{}); err != nil {
		t.Fatalf("Let them on an old play: %v", err)
	}
	if zone, asked := w.zoneOf(deck, copyID); zone != decks.ZoneUsed || asked {
		t.Errorf("the old play is %s asked=%v", zone, asked)
	}
}

func TestAPlayAskNamesTheCardOnlyToWhoSeesTheDeck(t *testing.T) {
	w := newStoryWorld(t)
	// An event deck is listed to Tam only while his hero holds one of its cards.
	deck := w.deck("Secrets", decks.KindEvent, "", &decks.SettingsInput{Arrives: ptrTo("up")})
	pact := w.card(deck, "Pact", 1, decks.CreateCardInput{AskFirst: true})
	copyID := w.deal(deck, pact, w.tamHero)
	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	tam := homebrew.Viewer{UserID: w.tamUser, VisibilityRole: 1}
	mine := func() homebrew.AskView {
		t.Helper()
		got, err := w.homebrew.MyAsks(storyCtx, w.camp, tam)
		if err != nil || len(got) != 1 {
			t.Fatalf("Tam's asks = %+v, %v", got, err)
		}
		return got[0]
	}
	if a := mine(); a.Ask != "play Pact from Secrets" || a.State != homebrew.AskWaiting || a.CanTakeBack {
		t.Errorf("while Tam holds it: %+v", a)
	}
	ask := w.theAsk()
	if _, err := w.homebrew.Answer(storyCtx, w.camp, ask.ID, w.hbDirector, homebrew.AnswerInput{Answer: homebrew.AnswerYes}); err != nil {
		t.Fatal(err)
	}
	// Played to the used pile, the deck is hidden from Tam again.
	if a := mine(); a.Ask != "play a card" || strings.Contains(a.Ask, "Secrets") || a.State != homebrew.AnswerYes {
		t.Errorf("once the deck is hidden: %+v", a)
	}
	// The Director still reads it in full.
	for _, r := range w.drawer().Recent {
		if r.ID == ask.ID && (r.Ask == nil || r.Ask.Ask != "play Pact from Secrets") {
			t.Errorf("the Director's line: %+v", r.Ask)
		}
	}
}
