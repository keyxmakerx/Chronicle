package app

// homebrew_roll_first_integration_test.go runs "The player rolls first" end
// to end over the real decks, trackers, homebrew, rolls and campaigns
// services on a scratch database: a player's card play asks, the Director
// says Yes with the roll as its condition, the player rolls on the server,
// and the step after the roll runs only when the total meets the target.
// Skipped under -short and without a database.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
)

// rollFirstWorld is a story world whose Omens recipe asks before a played
// card, then has the player roll 2d10 needing 2 (always made) before Doom.
func rollFirstWorld(t *testing.T) *storyWorld {
	w := newStoryWorld(t)
	mustGalleryExec(t, w.db, `UPDATE users SET display_name = 'Tam' WHERE id = ?`, w.tamUser)
	deck := w.deck("Omens", decks.KindHero, w.tamHero, &decks.SettingsInput{Arrives: ptrTo("up")})
	wind := w.card(deck, "Second wind", 1, decks.CreateCardInput{})
	w.recipe(rule(cardPlayedEvent, `{"card":"Second wind"}`, homebrew.TimingNow,
		doAct(homebrew.AskAction, `{"ask":"play Second wind"}`),
		doAct(RollFirstAction, `{"dice":"2d10","target":2}`),
		doomBy(1)))
	copyID := w.deal(deck, wind, w.tamHero)
	if _, err := w.decks.Act(storyCtx, w.camp, deck, copyID, "play", w.tam, decks.ActInput{}); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestRollFirstEndToEnd(t *testing.T) {
	tests := []struct {
		name  string
		edits []homebrew.StepEdit
		state string
		doom  int
	}{
		{"made: the step after the roll runs", nil, homebrew.AskRolled, 1},
		// 2d10 can't make 21: the Director's Yes, and… raised the target.
		{"missed: the rule stops", []homebrew.StepEdit{{Step: 1, Args: json.RawMessage(`{"dice":"2d10","target":21}`)}}, homebrew.AskMissed, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := rollFirstWorld(t)
			tam := homebrew.Viewer{UserID: w.tamUser, VisibilityRole: 1}
			ilse := homebrew.Viewer{UserID: w.ilseUser, VisibilityRole: 1}
			id := w.drawer().Waiting[0].ID
			if _, err := w.homebrew.Answer(storyCtx, w.camp, id, w.hbDirector, homebrew.AnswerInput{Answer: homebrew.AnswerYes, Steps: tt.edits}); err != nil {
				t.Fatal(err)
			}
			// The rule now waits on Tam, not on the Director.
			d := w.drawer()
			if len(d.Waiting) != 1 || d.Waiting[0].Kind != homebrew.KindRoll || d.Waiting[0].Ask != nil || d.Waiting[0].Roll.PlayerID != w.tamUser {
				t.Fatalf("waiting = %+v", d.Waiting)
			}
			if w.doomOn(w.tamHero) != -1 {
				t.Fatal("the step after the roll ran before it")
			}
			mine, err := w.homebrew.MyAsks(storyCtx, w.camp, tam)
			w.must(err)
			if len(mine) != 1 || mine[0].State != homebrew.AskRoll || !mine[0].CanRoll || mine[0].Answered != homebrew.AnswerYes {
				t.Fatalf("Tam's asks = %+v", mine)
			}
			if last := w.asks.rings[len(w.asks.rings)-1]; last.Message != "Your Director said yes, if you roll first: play Second wind" {
				t.Errorf("Tam's bell = %+v", last)
			}

			if _, err := w.homebrew.Roll(storyCtx, w.camp, id, ilse); err == nil {
				t.Fatal("another player rolled Tam's roll")
			}
			if _, err := w.homebrew.Roll(storyCtx, w.camp, id, w.hbDirector); err == nil {
				t.Fatal("the Director rolled Tam's roll")
			}
			got, err := w.homebrew.Roll(storyCtx, w.camp, id, tam)
			w.must(err)
			if got.State != tt.state || got.Roll == nil || got.Roll.Total == nil {
				t.Fatalf("Tam's roll = %+v", got)
			}
			if doom := w.doomOn(w.tamHero); doom != tt.doom {
				t.Errorf("doom = %d, want %d", doom, tt.doom)
			}
			if len(w.drawer().Waiting) != 0 {
				t.Error("a made roll still waits")
			}

			// The roll is in the roll log as Tam's, with the server's total.
			page, err := w.rolls.List(storyCtx, w.camp, rolls.Viewer{UserID: w.ilseUser}, "", 10)
			w.must(err)
			if len(page.Rolls) != 1 {
				t.Fatalf("roll log = %+v", page.Rolls)
			}
			r := page.Rolls[0]
			if r.RollerID != w.tamUser || r.RollerName != "Tam" || r.Formula != "2d10" || r.Mode != rolls.ModePublic ||
				r.Total == nil || *r.Total != *got.Roll.Total || r.Label != "House rule roll" {
				t.Errorf("logged roll = %+v", r)
			}
			last := w.asks.rings[len(w.asks.rings)-1]
			if last.Kind != homebrew.BellRolled || last.Users[0] != w.owner || !strings.HasPrefix(last.Message, "Tam rolled ") {
				t.Errorf("Director's bell = %+v", last)
			}
		})
	}
}

func TestRollFirstSetAsideEndToEnd(t *testing.T) {
	w := rollFirstWorld(t)
	id := w.drawer().Waiting[0].ID
	if _, err := w.homebrew.Answer(storyCtx, w.camp, id, w.hbDirector, homebrew.AnswerInput{Answer: homebrew.AnswerYes}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.homebrew.Dismiss(storyCtx, w.camp, id, w.hbDirector); err != nil {
		t.Fatal(err)
	}
	tam := homebrew.Viewer{UserID: w.tamUser, VisibilityRole: 1}
	if _, err := w.homebrew.Roll(storyCtx, w.camp, id, tam); err == nil {
		t.Fatal("a set-aside roll was made")
	}
	mine, err := w.homebrew.MyAsks(storyCtx, w.camp, tam)
	w.must(err)
	if len(mine) != 1 || mine[0].State != homebrew.AskSetAside || w.doomOn(w.tamHero) != -1 {
		t.Fatalf("Tam's asks = %+v", mine)
	}
}

func TestRollFirstBadDiceRefusedAtSave(t *testing.T) {
	w := newStoryWorld(t)
	for _, dice := range []string{"2d7", "", "roll well", "60d6"} {
		_, err := w.homebrew.Create(storyCtx, w.camp, w.hbDirector, homebrew.RecipeInput{Name: "Bad dice", Doc: homebrew.Doc{
			Rules: []homebrew.Rule{rule(cardPlayedEvent, `{}`, homebrew.TimingNow,
				doAct(RollFirstAction, `{"dice":"`+dice+`","target":5}`))}}})
		if err == nil || !strings.Contains(err.Error(), "Write the dice") {
			t.Errorf("dice %q saved: %v", dice, err)
		}
	}
}
