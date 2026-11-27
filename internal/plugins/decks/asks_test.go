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
	closed  []string
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

func (r *recAsks) Close(_ context.Context, _, _, copyID string, v Viewer, why string) error {
	if !v.Director {
		return errors.New("closed without a Director's standing")
	}
	if r.waiting[copyID] {
		delete(r.waiting, copyID)
		r.closed = append(r.closed, why)
	}
	return nil
}

// A waiting card that leaves its wait any other way closes its ask, so the
// drawer never offers an answer for a card that moved; a move that keeps it
// waiting leaves the ask open.
func TestAWaitingCardMovedAnotherWayClosesItsAsk(t *testing.T) {
	cases := []struct {
		name string
		move func(fx *fixture, deck, card, cp string) error
		why  string
	}{
		{"a Director plays it", func(fx *fixture, deck, _, cp string) error {
			_, err := fx.svc.Act(bg, camp, deck, cp, "play", fx.dir, ActInput{})
			return err
		}, "Closed: a Director played the card."},
		{"a Director discards it", func(fx *fixture, deck, _, cp string) error {
			_, err := fx.svc.Act(bg, camp, deck, cp, "discard", fx.dir, ActInput{})
			return err
		}, "Closed: the card was discarded."},
		{"its player discards it", func(fx *fixture, deck, _, cp string) error {
			_, err := fx.svc.Act(bg, camp, deck, cp, "discard", fx.tam, ActInput{})
			return err
		}, "Closed: the card was discarded."},
		{"a Director takes it back", func(fx *fixture, deck, _, cp string) error {
			_, err := fx.svc.Act(bg, camp, deck, cp, "take-back", fx.dir, ActInput{})
			return err
		}, "Closed: a Director took the card back."},
		{"a Director keeps it in another deck", func(fx *fixture, deck, _, cp string) error {
			other := fx.deck("Trophies", KindParty, nil)
			_, err := fx.svc.Act(bg, camp, deck, cp, "keep", fx.dir, ActInput{TargetDeckID: other})
			return err
		}, "Closed: a Director kept the card in another deck."},
		{"a Director gathers the deck", func(fx *fixture, deck, _, _ string) error {
			_, err := fx.svc.Gather(bg, camp, deck, fx.dir)
			return err
		}, "Closed: a Director gathered the deck."},
		{"its card is deleted", func(fx *fixture, deck, card, _ string) error {
			_, err := fx.svc.DeleteCard(bg, camp, deck, card, fx.dir)
			return err
		}, "Closed: the card is no longer in the deck."},
		{"its deck is deleted", func(fx *fixture, deck, _, _ string) error {
			return fx.svc.Delete(bg, camp, deck, fx.dir)
		}, "Closed: the card is no longer in the deck."},
		{"a Director turning it keeps it waiting", func(fx *fixture, deck, _, cp string) error {
			_, err := fx.svc.Act(bg, camp, deck, cp, "turn", fx.dir, ActInput{})
			return err
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asks := &recAsks{waiting: map[string]bool{}}
			fx := newFixtureWith(t, WithAsks(asks))
			asks.svc = fx.svc
			deck := fx.deck("Boons", KindHero, &SettingsInput{Arrives: ptr("up"), Discard: ptr(true)})
			boon := fx.card(deck, CreateCardInput{Name: "Boon", AskFirst: true})
			cp := fx.copiesOf(boon)[0]
			fx.place(cp, ZoneHand, "h-tam", true)
			_, err := fx.svc.Act(bg, camp, deck, cp, "play", fx.tam, ActInput{})
			fx.must(err)

			fx.must(tc.move(fx, deck, boon, cp))
			if tc.why == "" {
				if len(asks.closed) != 0 || !asks.waiting[cp] {
					t.Errorf("the ask closed: %v", asks.closed)
				}
				return
			}
			if len(asks.closed) != 1 || asks.closed[0] != tc.why || asks.waiting[cp] {
				t.Errorf("closed = %v, want [%s]", asks.closed, tc.why)
			}
		})
	}
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
