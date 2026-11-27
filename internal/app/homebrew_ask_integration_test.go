package app

// homebrew_ask_integration_test.go runs the "Ask the Director" step end to
// end over the real decks, trackers, homebrew and campaigns services on a
// scratch database: a player's card play stops at the ask, the Director's
// answer runs (or doesn't run) today's tracker and deck actions, and the
// player reads what came back. Skipped under -short and without a database.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// askCapture records what the ask step rang and published.
type askCapture struct {
	mu     sync.Mutex
	rings  []homebrew.Notice
	events []homebrew.AskEvent
}

func (c *askCapture) Ring(_ context.Context, _ string, n homebrew.Notice) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rings = append(c.rings, n)
	return nil
}

func (c *askCapture) AskChanged(_ string, e homebrew.AskEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

// askWorld is a story world whose Omens recipe asks before a played card's
// Doom step and the shuffle of more copies of it into the deck.
func askWorld(t *testing.T) (w *storyWorld, deck, wind string) {
	w = newStoryWorld(t)
	mustGalleryExec(t, w.db, `UPDATE users SET display_name = 'Tam' WHERE id = ?`, w.tamUser)
	deck = w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Arrives: ptrTo("up")})
	wind = w.card(deck, "Second wind", 1, decks.CreateCardInput{})
	w.recipe(rule(cardPlayedEvent, `{"card":"Second wind"}`, homebrew.TimingNow,
		doAct(homebrew.AskAction, `{"ask":"play Second wind"}`),
		doomBy(1),
		doAct(deckAddCopies, `{"deck":"Omens","card":"Second wind","copies":1}`)))
	copyID := w.deal(deck, wind, w.tamHero)
	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	return w, deck, wind
}

func (w *storyWorld) copiesOf(deck, cardID string) int {
	n := 0
	for _, c := range w.view(deck).Copies {
		if c.CardID == cardID {
			n++
		}
	}
	return n
}

func TestAskTheDirectorEndToEnd(t *testing.T) {
	tam := homebrew.Viewer{UserID: "", VisibilityRole: 1}
	tests := []struct {
		name   string
		answer homebrew.AnswerInput
		doom   int // Doom on Tam's page after, -1 for no tracker
		copies int
		state  string
		bell   string
	}{
		{"yes runs the steps as the rule says", homebrew.AnswerInput{Answer: homebrew.AnswerYes}, 1, 2, homebrew.AnswerYes,
			"Your Director said yes: play Second wind"},
		{"yes, and… runs them with the Director's changes", homebrew.AnswerInput{Answer: homebrew.AnswerYes, Steps: []homebrew.StepEdit{
			{Step: 1, Args: json.RawMessage(`{"by":3}`)},
			{Step: 2, Args: json.RawMessage(`{"deck":"Omens","card":"Second wind","copies":2}`)}}}, 3, 3, homebrew.AnswerYes,
			"Your Director said yes, with changes: play Second wind"},
		{"no runs nothing more", homebrew.AnswerInput{Answer: homebrew.AnswerNo}, -1, 1, homebrew.AnswerNo,
			"Your Director said no: play Second wind"},
		{"a written reply runs nothing more", homebrew.AnswerInput{Answer: homebrew.AnswerOther, Reply: "After the fight."}, -1, 1, homebrew.AnswerOther,
			"Your Director replied: After the fight."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, deck, wind := askWorld(t)
			tam.UserID = w.tamUser
			d := w.drawer()
			if len(d.Waiting) != 1 || d.Waiting[0].Ask == nil {
				t.Fatalf("waiting = %+v", d.Waiting)
			}
			ask := d.Waiting[0].Ask
			if ask.Asker != "Tam" || ask.AskerID != w.tamUser || ask.Ask != "play Second wind" || len(ask.Later) != 2 || len(ask.Later[0].Parts) != 1 {
				t.Fatalf("ask = %+v", ask)
			}
			if w.doomOn(w.tamHero) != -1 || w.copiesOf(deck, wind) != 1 {
				t.Fatal("a step after the ask ran before the answer")
			}
			mine, err := w.homebrew.MyAsks(storyCtx, w.camp, tam)
			w.must(err)
			if len(mine) != 1 || mine[0].State != homebrew.AskWaiting {
				t.Fatalf("Tam's asks = %+v", mine)
			}
			if len(w.asks.rings) != 1 || w.asks.rings[0].Message != "Tam wants to play Second wind" || strings.Join(w.asks.rings[0].Users, ",") != w.owner {
				t.Fatalf("Director's bell = %+v", w.asks.rings)
			}

			// A player can't answer, even their own ask.
			if _, err := w.homebrew.Answer(storyCtx, w.camp, d.Waiting[0].ID, tam, tt.answer); err == nil {
				t.Fatal("a player answered")
			}
			if _, err := w.homebrew.Answer(storyCtx, w.camp, d.Waiting[0].ID, w.hbDirector, tt.answer); err != nil {
				t.Fatal(err)
			}
			if got := w.doomOn(w.tamHero); got != tt.doom {
				t.Errorf("doom = %d, want %d", got, tt.doom)
			}
			if got := w.copiesOf(deck, wind); got != tt.copies {
				t.Errorf("copies = %d, want %d", got, tt.copies)
			}
			mine, err = w.homebrew.MyAsks(storyCtx, w.camp, tam)
			w.must(err)
			if len(mine) != 1 || mine[0].State != tt.state {
				t.Fatalf("Tam's asks = %+v", mine)
			}
			last := w.asks.rings[len(w.asks.rings)-1]
			if last.Message != tt.bell || last.Users[0] != w.tamUser || last.Link != "/campaigns/"+w.camp+"/homebrew/asks" {
				t.Errorf("Tam's bell = %+v", last)
			}
			if d := w.drawer(); len(d.Waiting) != 0 {
				t.Errorf("an answered ask still waits: %+v", d.Waiting)
			}
			// Answered once.
			if _, err := w.homebrew.Answer(storyCtx, w.camp, d.Waiting[0].ID, w.hbDirector, tt.answer); err == nil {
				t.Error("a second answer was taken")
			}
		})
	}
}

func TestAskTakenBackEndToEnd(t *testing.T) {
	w, deck, wind := askWorld(t)
	id := w.drawer().Waiting[0].ID
	ilse := homebrew.Viewer{UserID: w.ilseUser, VisibilityRole: 1}
	if _, err := w.homebrew.TakeBack(storyCtx, w.camp, id, ilse); err == nil {
		t.Fatal("another player took back Tam's ask")
	}
	if _, err := w.homebrew.TakeBack(storyCtx, w.camp, id, homebrew.Viewer{UserID: w.tamUser, VisibilityRole: 1}); err != nil {
		t.Fatal(err)
	}
	if len(w.drawer().Waiting) != 0 || w.doomOn(w.tamHero) != -1 || w.copiesOf(deck, wind) != 1 {
		t.Fatal("a taken-back ask still waits or ran")
	}
	if last := w.asks.events[len(w.asks.events)-1]; last.ID != id || last.State != homebrew.AskTakenBack || last.Asker != w.tamUser {
		t.Errorf("live = %+v", w.asks.events)
	}
}
