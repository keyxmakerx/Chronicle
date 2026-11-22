package decks

import (
	"reflect"
	"testing"
)

// heard says who a captured message reached, by the same rule the hub applies:
// a Directors-only message reaches no player, a members message every one, and
// a list only the people on it.
func heard(a Audience, user string) bool {
	if a.Members {
		return true
	}
	for _, u := range a.Users {
		if u == user {
			return true
		}
	}
	return false
}

// recordFor is the sequence of messages one player's connection receives.
func recordFor(c *capture, user string) []bool {
	var out []bool
	for _, a := range c.told {
		out = append(out, heard(a, user))
	}
	return out
}

func told(c *capture, user string) int {
	n := 0
	for _, h := range recordFor(c, user) {
		if h {
			n++
		}
	}
	return n
}

// A deal to Tam's hero tells Tam, and Ilse who may see Tam's hero's backs; a
// Scribe the deck is not for hears nothing. A Director's draw tells no player.
func TestADealTellsTheirPlayersAndADrawTellsNoOne(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Boons", KindHero, &SettingsInput{Knows: ptr("nothing"), OthersSee: ptr("back"), Draws: ptr("directors")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
	fx.card(deck, CreateCardInput{Name: "Ember", Copies: ptr(4)})
	fx.cap.told = nil

	res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 2})
	fx.must(err)
	if len(fx.cap.told) != 2 || told(fx.cap, "u-tam")+told(fx.cap, "u-ilse")+told(fx.cap, "u-scribe") != 0 {
		t.Fatalf("a Director's draw told players: %+v", fx.cap.told)
	}
	_, err = fx.svc.Act(bg, camp, deck, res.CopyIDs[0], "deal", fx.dir, ActInput{To: "hand", HeroID: "h-tam"})
	fx.must(err)
	last := fx.cap.told[len(fx.cap.told)-1]
	if !reflect.DeepEqual(last, Audience{Users: []string{"u-ilse", "u-tam"}}) {
		t.Errorf("a deal to Tam told %+v", last)
	}
	if heard(last, "u-scribe") {
		t.Error("the Scribe was told")
	}
	if len(fx.cap.rings) != 1 || fx.cap.rings[0].Message != "A new card from Boons" || !reflect.DeepEqual(fx.cap.rings[0].Users, []string{"u-tam"}) {
		t.Errorf("bell = %+v", fx.cap.rings)
	}
}

// Whatever a Director does out of sight, what each player's connection
// receives is the same messages in the same order as when only the visible deal
// happens: a hidden draw and put back, the next and last cards, a card edit, a
// deal inside a deck no player is on.
func TestTwoWorldsEveryPlayerHearsTheSameMessages(t *testing.T) {
	run := func(t *testing.T, hidden bool) (*capture, *fixture) {
		fx := newFixture(t)
		deck := fx.deck("Omens", KindHero, &SettingsInput{Knows: ptr("count"), Arrives: ptr("down"), OthersSee: ptr("back"), Draws: ptr("players")})
		fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)}, PlayerInput{UserID: "u-ilse"})
		x := fx.card(deck, CreateCardInput{Name: "Xcard", Words: "XWORDS", Copies: ptr(3)})
		y := fx.card(deck, CreateCardInput{Name: "Ycard", Words: "YWORDS", Copies: ptr(3)})
		vault := fx.deck("Vault", KindEvent, nil)
		vc := fx.card(vault, CreateCardInput{Name: "VaultCard", Words: "VAULTWORDS", Copies: ptr(3)})
		fx.cap.told, fx.cap.rings = nil, nil

		hiddenActs := func() {
			res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 5})
			fx.must(err)
			for _, id := range res.CopyIDs[:4] {
				_, err := fx.svc.Act(bg, camp, deck, id, "put-back", fx.dir, ActInput{})
				fx.must(err)
			}
			fx.must(fx.nudge(deck, patchOf(y), patchOf(x)))
			_, err = fx.svc.UpdateCard(bg, camp, deck, y, fx.dir, UpdateCardInput{Words: patchOf("YWORDS, edited")})
			fx.must(err)
			_, err = fx.svc.DealCard(bg, camp, vault, fx.dir, DealCardInput{CardID: vc, To: "hand", HeroID: "h-npc"})
			fx.must(err)
		}
		if hidden {
			hiddenActs()
		}
		// The visible part: a deal to Tam's hero, then Tam turns it over and shows it.
		res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: x, To: "hand", HeroID: "h-tam"})
		fx.must(err)
		_, err = fx.svc.Act(bg, camp, deck, res.CopyID, "turn", fx.tam, ActInput{})
		fx.must(err)
		_, err = fx.svc.Act(bg, camp, deck, res.CopyID, "show", fx.tam, ActInput{})
		fx.must(err)
		return fx.cap, fx
	}
	plain, _ := run(t, false)
	hidden, _ := run(t, true)
	for _, user := range []string{"u-tam", "u-ilse", "u-scribe"} {
		var a, b []bool
		for _, h := range recordFor(plain, user) {
			if h {
				a = append(a, h)
			}
		}
		// A Director-only message is no message at all to a player, so the
		// recordings compare what each connection actually received.
		for _, h := range recordFor(hidden, user) {
			if h {
				b = append(b, h)
			}
		}
		if len(a) != len(b) {
			t.Errorf("%s received %d messages in the plain world and %d with the hidden acts", user, len(a), len(b))
		}
	}
	// Order too: the messages a player received are the same moves in the same
	// order, so compare the audiences of the visible tail of each world.
	tail := func(c *capture, n int) []Audience { return c.told[len(c.told)-n:] }
	if !reflect.DeepEqual(tail(plain, 3), tail(hidden, 3)) {
		t.Errorf("the visible deal, turn and show told different people:\n plain  %+v\n hidden %+v", tail(plain, 3), tail(hidden, 3))
	}
	for _, a := range hidden.told[:len(hidden.told)-3] {
		if a.Members || len(a.Users) != 0 {
			t.Errorf("a hidden act told players: %+v", a)
		}
	}
	// Directors-only messages are still sent, so Directors' screens follow.
	if len(hidden.told) <= len(plain.told) {
		t.Error("the hidden acts sent nothing at all")
	}
	if len(hidden.rings) != len(plain.rings) {
		t.Errorf("the hidden acts rang the bell: %d against %d", len(hidden.rings), len(plain.rings))
	}
}

// An ask tells the Directors on the bell; an answer tells the asker; a card
// taken back names itself only if it was face up.
func TestAsksAndTakeBacksReachTheBell(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Boons", KindHero, &SettingsInput{Arrives: ptr("up")})
	c := fx.card(deck, CreateCardInput{Name: "Favour", AskFirst: true, Copies: ptr(2)})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	_, err = fx.svc.Act(bg, camp, deck, res.CopyID, "play", fx.tam, ActInput{})
	fx.must(err)
	last := fx.cap.rings[len(fx.cap.rings)-1]
	if last.Kind != BellAsked || last.Message != "Tam asks to play Favour from Boons" || !reflect.DeepEqual(last.Users, []string{"u-dir"}) {
		t.Errorf("the ask rang %+v", last)
	}
	_, err = fx.svc.Act(bg, camp, deck, res.CopyID, "turn-down", fx.dir, ActInput{})
	fx.must(err)
	last = fx.cap.rings[len(fx.cap.rings)-1]
	if last.Kind != BellAnswered || last.Message != "Your Director turned down your ask to play Favour" || !reflect.DeepEqual(last.Users, []string{"u-tam"}) {
		t.Errorf("the answer rang %+v", last)
	}
	_, err = fx.svc.Act(bg, camp, deck, res.CopyID, "take-back", fx.dir, ActInput{})
	fx.must(err)
	last = fx.cap.rings[len(fx.cap.rings)-1]
	if last.Message != "A card was taken back from Boons: Favour" {
		t.Errorf("a face-up card taken back rang %q", last.Message)
	}
	// A card that arrived face down and was taken back unseen is not named.
	down := fx.deck("Downs", KindHero, &SettingsInput{Arrives: ptr("down")})
	dc := fx.card(down, CreateCardInput{Name: "Secretcard"})
	fx.choose(down, PlayerInput{UserID: "u-tam"})
	dr, err := fx.svc.DealCard(bg, camp, down, fx.dir, DealCardInput{CardID: dc, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	_, err = fx.svc.Act(bg, camp, down, dr.CopyID, "take-back", fx.dir, ActInput{})
	fx.must(err)
	last = fx.cap.rings[len(fx.cap.rings)-1]
	if last.Message != "A card was taken back from Downs" {
		t.Errorf("an unseen card taken back rang %q", last.Message)
	}
}

// A change to a deck is announced only when it changes something a player's
// view is built from, and then to the players who see the deck. A Director's
// next card, where a played card goes and a card's own edit tell nobody.
func TestADeckChangeTellsItsPlayersOnlyWhenTheyReadIt(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Boons", KindHero, &SettingsInput{Knows: ptr("nothing"), OthersSee: ptr("back")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	card := fx.card(deck, CreateCardInput{Name: "Ember", Copies: ptr(2)})
	version := func() int { return mustGet(t, fx, deck, fx.dir).(DirectorDeck).Version }

	fx.cap.told = nil
	_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(version()), NextCardID: patchOf(card)})
	fx.must(err)
	_, err = fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(version()), Settings: patchOf(SettingsInput{Fate: ptr(FateGone), Arrives: ptr("up")})})
	fx.must(err)
	_, err = fx.svc.UpdateCard(bg, camp, deck, card, fx.dir, UpdateCardInput{Words: patchOf("new words")})
	fx.must(err)
	if len(fx.cap.told) != 0 {
		t.Fatalf("a change no player reads was announced: %+v", fx.cap.told)
	}

	_, err = fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(version()), Settings: patchOf(SettingsInput{Knows: ptr("count")})})
	fx.must(err)
	if len(fx.cap.told) != 1 || !reflect.DeepEqual(fx.cap.told[0], Audience{Users: []string{"u-tam"}}) {
		t.Fatalf("a change to what players know told %+v", fx.cap.told)
	}

	// Deleting the deck tells the players who saw it.
	fx.cap.told = nil
	fx.must(fx.svc.Delete(bg, camp, deck, fx.dir))
	if len(fx.cap.told) != 1 || !reflect.DeepEqual(fx.cap.told[0], Audience{Users: []string{"u-tam"}}) {
		t.Fatalf("deleting the deck told %+v", fx.cap.told)
	}
}
