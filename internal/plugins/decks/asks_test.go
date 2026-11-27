package decks

import (
	"context"
	"errors"
	"testing"
)

// recAsks stands in for the Workshop's ask: it records each waiting play and,
// for a copy it holds, answers by calling back AnswerAsk once, as the ask's
// owner does. A copy it does not hold is an ask from before.
type recAsks struct {
	svc     Service
	asked   []PlayAsk
	waiting map[string]bool
	reply   string
	fail    error
}

func (r *recAsks) Ask(_ context.Context, _ string, a PlayAsk) error {
	r.asked = append(r.asked, a)
	if r.fail != nil {
		return r.fail
	}
	r.waiting[a.CopyID] = true
	return nil
}

func (r *recAsks) Answer(ctx context.Context, campaignID, deckID, copyID string, v Viewer, yes bool) (bool, error) {
	if !r.waiting[copyID] {
		return false, nil
	}
	delete(r.waiting, copyID)
	_, err := r.svc.AnswerAsk(ctx, campaignID, deckID, copyID, v, AskAnswer{Yes: yes, Reply: r.reply})
	return true, err
}

func TestAskFirstGoesThroughOneAsk(t *testing.T) {
	cases := []struct {
		name string
		// before asks the play while no ask is wired, as a play from before.
		before   bool
		failAsk  bool
		verb     string
		reply    string
		wantZone string
		wantBell string
		detail   string
	}{
		{name: "let them answers the ask", verb: "approve", wantZone: ZoneUsed, wantBell: "Your Director let you play Boon"},
		{name: "not now answers the ask", verb: "turn-down", wantZone: ZoneHand, wantBell: "Your Director turned down your ask to play Boon"},
		{name: "a reply reaches the player", verb: "turn-down", reply: "Not while the bridge burns.", wantZone: ZoneHand,
			wantBell: "Your Director replied to your ask to play Boon", detail: "Not while the bridge burns."},
		{name: "a play from before is answered on the deck", before: true, verb: "approve", wantZone: ZoneUsed, wantBell: "Your Director let you play Boon"},
		{name: "an ask that could not be made still waits on the deck", failAsk: true, verb: "turn-down", wantZone: ZoneHand,
			wantBell: "Your Director turned down your ask to play Boon"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asks := &recAsks{waiting: map[string]bool{}, reply: tc.reply}
			if tc.failAsk {
				asks.fail = errors.New("the workshop is away")
			}
			fx := newFixtureWith(t, WithAsks(asks))
			asks.svc = fx.svc
			deck := fx.deck("Boons", KindHero, &SettingsInput{Arrives: ptr("up")})
			boon := fx.card(deck, CreateCardInput{Name: "Boon", AskFirst: true})
			cp := fx.copiesOf(boon)[0]
			fx.place(cp, ZoneHand, "h-tam", true)

			if tc.before {
				// The same store, served before asks went through the Workshop.
				old := NewService(fx.repo, fx.heroes, fx.members, WithRandom(func(int) int { return 0 }))
				_, err := old.Act(bg, camp, deck, cp, "play", fx.tam, ActInput{})
				fx.must(err)
			} else {
				_, err := fx.svc.Act(bg, camp, deck, cp, "play", fx.tam, ActInput{})
				fx.must(err)
				if len(asks.asked) != 1 {
					t.Fatalf("asks = %+v", asks.asked)
				}
				a := asks.asked[0]
				if a.CopyID != cp || a.CardName != "Boon" || a.DeckName != "Boons" || a.HeroID != "h-tam" || a.HeroName != "Tam" || a.By.UserID != "u-tam" {
					t.Errorf("ask = %+v", a)
				}
			}
			if c := fx.copy(cp); c.Zone != ZoneHand || c.AskedAt == nil {
				t.Fatalf("an asked play must wait, not play: %+v", c)
			}
			// A player can't answer, whichever way it would go.
			if _, err := fx.svc.Act(bg, camp, deck, cp, tc.verb, fx.tam, ActInput{}); code(err) != 403 {
				t.Errorf("a player answered: %v", err)
			}

			fx.cap.rings = nil
			_, err := fx.svc.Act(bg, camp, deck, cp, tc.verb, fx.dir, ActInput{})
			fx.must(err)
			if c := fx.copy(cp); c.Zone != tc.wantZone || c.AskedAt != nil {
				t.Errorf("after %s: %+v", tc.verb, c)
			}
			if len(fx.cap.rings) != 1 || fx.cap.rings[0].Message != tc.wantBell || fx.cap.rings[0].Detail != tc.detail {
				t.Errorf("bell = %+v", fx.cap.rings)
			}
			// Answered once: a second answer, from anywhere, is refused.
			for _, verb := range []string{"approve", "turn-down"} {
				if _, err := fx.svc.Act(bg, camp, deck, cp, verb, fx.dir, ActInput{}); code(err) != 409 && code(err) != 404 {
					t.Errorf("a second %s was taken: %v", verb, err)
				}
			}
		})
	}
}
