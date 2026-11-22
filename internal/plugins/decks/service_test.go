package decks

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

var bg = context.Background()

func TestDrawsByDirectorAndEmptyPile(t *testing.T) {
	tests := []struct {
		name     string
		settings *SettingsInput
		used     int // copies moved to used before drawing
		pile     int
		wantCode int
		wantMsg  string
	}{
		{name: "draws from a pile", pile: 2},
		{name: "stop on empty", pile: 0, settings: &SettingsInput{Empty: ptr("stop")}, wantCode: http.StatusConflict, wantMsg: "Nothing left in this deck."},
		{name: "stop even with a used pile", pile: 0, used: 2, wantCode: http.StatusConflict, wantMsg: "Nothing left in this deck."},
		{name: "reshuffle with a used pile", pile: 0, used: 2, settings: &SettingsInput{Empty: ptr("reshuffle")}},
		{name: "reshuffle with nothing used", pile: 0, settings: &SettingsInput{Empty: ptr("reshuffle")}, wantCode: http.StatusConflict, wantMsg: "Nothing left in this deck."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("D", KindEvent, tt.settings)
			c := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(tt.pile + tt.used + 1)})
			ids := fx.copiesOf(c)
			// One copy sits in hand so the card has copies even when the pile is bare.
			fx.place(ids[0], ZoneHand, "h-npc", false)
			for i := 0; i < tt.used; i++ {
				fx.place(ids[1+i], ZoneUsed, "", true)
			}
			res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{})
			if tt.wantCode != 0 {
				if code(err) != tt.wantCode || !strings.Contains(err.Error(), tt.wantMsg) {
					t.Fatalf("err = %v, want %d %q", err, tt.wantCode, tt.wantMsg)
				}
				return
			}
			fx.must(err)
			if got := fx.copy(res.CopyID); got.Zone != ZoneShown {
				t.Errorf("drawn copy is %s, want shown", got.Zone)
			}
		})
	}
}

func TestDirectorDrawRules(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(3)})
	if _, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{HeroID: "h-tam"}); code(err) != http.StatusUnprocessableEntity {
		t.Errorf("a Director naming a hero: code %d, want 422", code(err))
	}
	if _, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{}); code(err) != http.StatusConflict {
		t.Errorf("drawing with a card already shown: code %d, want 409", code(err))
	}
}

func TestPlayerDraws(t *testing.T) {
	two := 2
	zero := 0
	tests := []struct {
		name       string
		kind       string
		settings   *SettingsInput
		chosen     []PlayerInput
		actor      func(*fixture) Viewer
		hero       string
		pile       int
		wantCode   int
		wantZone   string
		wantFace   *bool
		wantLeft   []int // draws left after each of the attempts, -1 for unlimited
		attempts   int
		lastCode   int
		leftAfter  *int
		leftIsNull bool
	}{
		{name: "players draw when allowed", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam", DrawsLeft: &two}},
			actor: tamOf, hero: "h-tam", pile: 3, attempts: 1, wantZone: ZoneHand, wantFace: ptr(false), leftAfter: ptr(1)},
		{name: "arrives up", kind: KindHero, settings: &SettingsInput{Draws: ptr("players"), Arrives: ptr("up")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: tamOf, hero: "h-tam", pile: 3, attempts: 1, wantZone: ZoneHand, wantFace: ptr(true), leftIsNull: true},
		{name: "no limit never runs out", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: tamOf, hero: "h-tam", pile: 5, attempts: 4, wantZone: ZoneHand, leftIsNull: true},
		{name: "the limit stops the third draw", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam", DrawsLeft: &two}},
			actor: tamOf, hero: "h-tam", pile: 5, attempts: 3, lastCode: http.StatusForbidden, leftAfter: ptr(0)},
		{name: "zero draws left", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam", DrawsLeft: &zero}},
			actor: tamOf, hero: "h-tam", pile: 5, attempts: 1, lastCode: http.StatusForbidden, leftAfter: ptr(0)},
		{name: "an empty pile spends nothing", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam", DrawsLeft: &two}},
			actor: tamOf, hero: "h-tam", pile: 0, attempts: 1, lastCode: http.StatusConflict, leftAfter: &two},
		{name: "directors draw is the default", kind: KindHero, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: tamOf, hero: "h-tam", pile: 3, attempts: 1, lastCode: http.StatusForbidden},
		{name: "not chosen on a hero deck is a 404", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: ilseOf, hero: "h-ilse", pile: 3, attempts: 1, lastCode: http.StatusNotFound},
		{name: "a member of a party deck who is not chosen is refused", kind: KindParty, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: ilseOf, hero: "h-ilse", pile: 3, attempts: 1, lastCode: http.StatusForbidden},
		{name: "into someone else's hero", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: tamOf, hero: "h-ilse", pile: 3, attempts: 1, lastCode: http.StatusForbidden},
		{name: "into no hero", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: tamOf, hero: "", pile: 3, attempts: 1, lastCode: http.StatusForbidden},
		{name: "a visitor draws nothing", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: visitorOf, hero: "h-tam", pile: 3, attempts: 1, lastCode: http.StatusNotFound},
		{name: "a former member draws nothing", kind: KindHero, settings: &SettingsInput{Draws: ptr("players")}, chosen: []PlayerInput{{UserID: "u-tam"}},
			actor: formerOf, hero: "h-tam", pile: 3, attempts: 1, lastCode: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("D", tt.kind, tt.settings)
			if tt.pile > 0 {
				fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(tt.pile)})
			} else {
				fx.card(deck, CreateCardInput{Name: "A"})
				fx.place(fx.copiesOf(fx.cardID(deck, "A"))[0], ZoneGone, "", true)
			}
			fx.choose(deck, tt.chosen...)
			var last error
			var res *ActionResult
			for i := 0; i < tt.attempts; i++ {
				res, last = fx.svc.Draw(bg, camp, deck, tt.actor(fx), DrawInput{HeroID: tt.hero})
			}
			if code(last) != tt.lastCode {
				t.Fatalf("last attempt code = %d (%v), want %d", code(last), last, tt.lastCode)
			}
			if last == nil {
				got := fx.copy(res.CopyID)
				if got.Zone != tt.wantZone || got.HolderEntityID != tt.hero || got.DealtBy != "u-tam" {
					t.Errorf("copy = %+v", got)
				}
				if tt.wantFace != nil && got.FaceUp != *tt.wantFace {
					t.Errorf("faceUp = %v", got.FaceUp)
				}
			}
			players := fx.repo.decks[deck].Players
			if tt.leftAfter != nil && (players[0].DrawsLeft == nil || *players[0].DrawsLeft != *tt.leftAfter) {
				t.Errorf("draws left = %v, want %d", players[0].DrawsLeft, *tt.leftAfter)
			}
			if tt.leftIsNull && players[0].DrawsLeft != nil {
				t.Errorf("draws left = %d, want no limit", *players[0].DrawsLeft)
			}
		})
	}
}

func (fx *fixture) cardID(deckID, name string) string {
	for _, c := range fx.repo.cards {
		if c.DeckID == deckID && c.Name == name {
			return c.ID
		}
	}
	fx.t.Fatalf("no card %q", name)
	return ""
}

func TestDealCardPicksOnlyThatCard(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindHero, nil)
	a := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(2)})
	b := fx.card(deck, CreateCardInput{Name: "B", Copies: ptr(2)})
	for i := 0; i < 2; i++ {
		res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: b, To: "hand", HeroID: "h-tam"})
		fx.must(err)
		if got := fx.copy(res.CopyID); got.CardID != b || got.Zone != ZoneHand || got.HolderEntityID != "h-tam" {
			t.Errorf("dealt %+v", got)
		}
	}
	if _, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: b, To: "table"}); code(err) != http.StatusConflict {
		t.Errorf("no copy left: code %d, want 409", code(err))
	}
	if _, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: a, To: "table"}); err != nil {
		t.Errorf("the other card is still there: %v", err)
	}
	if _, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: "nope", To: "table"}); code(err) != http.StatusNotFound {
		t.Errorf("unknown card: code %d", code(err))
	}
	// A shown copy does not block a named deal.
	if _, err := fx.svc.DealCard(bg, camp, deck, fx.tam, DealCardInput{CardID: a, To: "table"}); code(err) == 0 {
		t.Error("a player dealt a card")
	}
	if _, err := fx.svc.DealCard(bg, camp, deck, fx.visitor, DealCardInput{CardID: a, To: "table"}); code(err) != http.StatusNotFound {
		t.Errorf("visitor: code %d, want 404", code(err))
	}
}

func TestShuffleAndGather(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	c := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(5)})
	ids := fx.copiesOf(c)
	fx.place(ids[0], ZoneUsed, "", true)
	fx.place(ids[1], ZoneUsed, "", true)
	fx.place(ids[2], ZoneHand, "h-tam", true)
	fx.place(ids[3], ZoneGone, "", true)
	fx.place(ids[4], ZoneShown, "", false)
	zones := func() string {
		var z []string
		for _, id := range fx.copiesOf(c) {
			z = append(z, fx.copy(id).Zone)
		}
		sortStrings(z)
		return strings.Join(z, ",")
	}

	if _, err := fx.svc.Shuffle(bg, camp, deck, fx.ilse); code(err) != http.StatusNotFound {
		t.Errorf("player shuffle on an unlisted deck: code %d, want 404", code(err))
	}
	// Tam holds a copy, so the deck is theirs to see but not to shuffle.
	if _, err := fx.svc.Shuffle(bg, camp, deck, fx.tam); code(err) != http.StatusForbidden {
		t.Errorf("player shuffle on a deck they hold a card of: code %d, want 403", code(err))
	}
	if _, err := fx.svc.Shuffle(bg, camp, deck, fx.dir); err != nil {
		t.Fatal(err)
	}
	if got := zones(); got != "gone,hand,pile,pile,shown" {
		t.Errorf("after shuffle: %s", got)
	}
	// The copies that came back have new ids; the ones that did not keep theirs.
	after := fx.copiesOf(c)
	if after[0] == ids[0] || after[1] == ids[1] || after[2] != ids[2] || after[3] != ids[3] || after[4] != ids[4] {
		t.Errorf("ids after shuffle: %v, were %v", after, ids)
	}
	if _, err := fx.svc.Gather(bg, camp, deck, fx.dir); err != nil {
		t.Fatal(err)
	}
	for _, id := range fx.copiesOf(c) {
		if got := fx.copy(id); got.Zone != ZonePile || got.HolderEntityID != "" || got.FaceUp {
			t.Errorf("after gather %+v", got)
		}
	}
	for i, id := range fx.copiesOf(c) {
		if id == ids[i] {
			t.Errorf("copy %d kept its id through a gather", i)
		}
	}
	// A deck a player can see but may not act on is a 403.
	party := fx.deck("P", KindParty, nil)
	if _, err := fx.svc.Gather(bg, camp, party, fx.tam); code(err) != http.StatusForbidden {
		t.Errorf("player gather on a party deck: code %d, want 403", code(err))
	}
	if len(fx.cap.got) != 2 {
		t.Errorf("announced %d bulk moves, want 2", len(fx.cap.got))
	}
}

func TestCopiesReconciliation(t *testing.T) {
	tests := []struct {
		name  string
		zones []string // starting zone of each copy
		want  int
		// afterZones is the sorted zones left, or wantErr is set.
		wantZones string
		wantErr   string
	}{
		{name: "raise adds pile copies", zones: []string{ZonePile, ZoneHand}, want: 4, wantZones: "hand,pile,pile,pile"},
		{name: "lower takes from the pile first", zones: []string{ZonePile, ZonePile, ZoneUsed, ZoneGone}, want: 2, wantZones: "gone,used"},
		{name: "then used", zones: []string{ZonePile, ZoneUsed, ZoneUsed, ZoneGone, ZoneHand}, want: 2, wantZones: "gone,hand"},
		{name: "then gone", zones: []string{ZoneUsed, ZoneGone, ZoneGone, ZoneHand}, want: 2, wantZones: "gone,hand"},
		{name: "never from hand or play", zones: []string{ZoneHand, ZonePlay, ZonePile}, want: 1, wantErr: "2 copies of this card are held in hands or in play"},
		{name: "one held", zones: []string{ZoneHand, ZonePile}, want: 0 + 1, wantZones: "hand"},
		{name: "refuse naming the count of one", zones: []string{ZoneHand, ZoneHand}, want: 1, wantErr: "2 copies of this card are held"},
		{name: "same number changes nothing", zones: []string{ZoneHand, ZonePile}, want: 2, wantZones: "hand,pile"},
		{name: "above the limit", zones: []string{ZonePile}, want: MaxCopiesPerCard + 1, wantErr: "copies must be between 1 and 20"},
		{name: "below one", zones: []string{ZonePile}, want: 0, wantErr: "copies must be between 1 and 20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("D", KindEvent, nil)
			c := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(len(tt.zones))})
			for i, id := range fx.copiesOf(c) {
				holder := ""
				if tt.zones[i] == ZoneHand || tt.zones[i] == ZonePlay {
					holder = "h-tam"
				}
				fx.place(id, tt.zones[i], holder, true)
			}
			_, err := fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Copies: patch.Of(tt.want)})
			var after []string
			for _, id := range fx.copiesOf(c) {
				after = append(after, fx.copy(id).Zone)
			}
			sortStrings(after)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				if got := len(after); got != len(tt.zones) {
					t.Errorf("a refused change altered the copies: %d left of %d", got, len(tt.zones))
				}
				return
			}
			fx.must(err)
			if got := strings.Join(after, ","); got != tt.wantZones {
				t.Errorf("zones = %s, want %s", got, tt.wantZones)
			}
			if fx.repo.cards[c].Copies != tt.want {
				t.Errorf("stored copies = %d, want %d", fx.repo.cards[c].Copies, tt.want)
			}
		})
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func TestDeletingCardOrDeckRemovesCopiesEverywhere(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	a := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(3)})
	fx.card(deck, CreateCardInput{Name: "B", Copies: ptr(2)})
	ids := fx.copiesOf(a)
	fx.place(ids[0], ZoneHand, "h-tam", true)
	fx.place(ids[1], ZonePlay, "", true)
	if _, err := fx.svc.DeleteCard(bg, camp, deck, a, fx.dir); err != nil {
		t.Fatal(err)
	}
	if len(fx.repo.copies) != 2 {
		t.Errorf("%d copies left, want 2 (card B)", len(fx.repo.copies))
	}
	if err := fx.svc.Delete(bg, camp, deck, fx.dir); err != nil {
		t.Fatal(err)
	}
	if len(fx.repo.copies) != 0 || len(fx.repo.cards) != 0 {
		t.Error("deleting the deck left copies or cards behind")
	}
}

func TestLimits(t *testing.T) {
	t.Run("sixty decks", func(t *testing.T) {
		fx := newFixture(t)
		for i := 0; i < MaxDecksPerCampaign; i++ {
			fx.deck("Deck "+itoa(i), KindEvent, nil)
		}
		_, err := fx.svc.Create(bg, camp, fx.dir, CreateDeckInput{Name: "One more", Kind: KindEvent})
		if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "at most 60 decks") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("two hundred cards", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, nil)
		for i := 0; i < MaxCardsPerDeck; i++ {
			fx.card(deck, CreateCardInput{Name: "C" + itoa(i)})
		}
		_, err := fx.svc.CreateCard(bg, camp, deck, fx.dir, CreateCardInput{Name: "extra"})
		if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "at most 200 cards") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("six hundred copies", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, nil)
		for i := 0; i < 30; i++ {
			fx.card(deck, CreateCardInput{Name: "C" + itoa(i), Copies: ptr(20)})
		}
		_, err := fx.svc.CreateCard(bg, camp, deck, fx.dir, CreateCardInput{Name: "extra"})
		if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "at most 600 copies") {
			t.Errorf("create err = %v", err)
		}
		c := fx.cardID(deck, "C0")
		fx.repo.cards[c] = func() Card { x := fx.repo.cards[c]; x.Copies = 19; return x }()
		fx.repo.copies = fx.repo.copies[1:] // keep rows and card in step for the check
		_, err = fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Copies: patch.Of(20)})
		fx.must(err)
		_, err = fx.svc.UpdateCard(bg, camp, deck, fx.cardID(deck, "C1"), fx.dir, UpdateCardInput{Copies: patch.Of(20)})
		fx.must(err)
	})
}

func TestCardRules(t *testing.T) {
	tests := []struct {
		name    string
		in      CreateCardInput
		wantErr string
	}{
		{name: "plain card", in: CreateCardInput{Name: "A"}},
		{name: "name needed", in: CreateCardInput{Name: "  "}, wantErr: "needs a name"},
		{name: "name length", in: CreateCardInput{Name: strings.Repeat("n", 121)}, wantErr: "too long"},
		{name: "words length", in: CreateCardInput{Name: "A", Words: strings.Repeat("w", 2001)}, wantErr: "too long"},
		{name: "words keep line breaks", in: CreateCardInput{Name: "A", Words: "one\ntwo"}},
		{name: "note length", in: CreateCardInput{Name: "A", DirectorNote: strings.Repeat("w", 2001)}, wantErr: "too long"},
		{name: "corner length", in: CreateCardInput{Name: "A", Corner: strings.Repeat("c", 13)}, wantErr: "too long"},
		{name: "kind must exist in the deck", in: CreateCardInput{Name: "A", KindKey: "boon"}, wantErr: "not one of the deck's kinds"},
		{name: "picture refused for now", in: CreateCardInput{Name: "A", Picture: "x.png"}, wantErr: "picture yet"},
		{name: "condition refused for now", in: CreateCardInput{Name: "A", When: `{"x":1}`}, wantErr: "condition yet"},
		{name: "when_open is stored", in: CreateCardInput{Name: "A", WhenOpen: true}},
		{name: "bad fate", in: CreateCardInput{Name: "A", Fate: "burn"}, wantErr: "fate must be one of"},
		{name: "copies over 20", in: CreateCardInput{Name: "A", Copies: ptr(21)}, wantErr: "between 1 and 20"},
		{name: "zero copies", in: CreateCardInput{Name: "A", Copies: ptr(0)}, wantErr: "between 1 and 20"},
		{name: "control character", in: CreateCardInput{Name: "A\x00"}, wantErr: "not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("D", KindEvent, nil)
			_, err := fx.svc.CreateCard(bg, camp, deck, fx.dir, tt.in)
			if tt.wantErr != "" {
				if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			fx.must(err)
		})
	}
	t.Run("kinds and duplicate names", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, nil)
		_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patch.Of(1),
			Look: patch.Of(LookInput{Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}}})})
		fx.must(err)
		fx.card(deck, CreateCardInput{Name: "A", KindKey: "boon"})
		if _, err := fx.svc.CreateCard(bg, camp, deck, fx.dir, CreateCardInput{Name: "A"}); code(err) != http.StatusConflict {
			t.Errorf("duplicate name: code %d, want 409", code(err))
		}
		// A kind a card still uses cannot be dropped from the look.
		_, err = fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patch.Of(2), Look: patch.Of(LookInput{Kinds: []KindDef{}})})
		if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "still uses the kind") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestDeckPartialUpdateAndVersion(t *testing.T) {
	fx := newFixture(t)
	d, err := fx.svc.Create(bg, camp, fx.dir, CreateDeckInput{Name: "Fortune", Kind: KindHero, HolderEntityID: "h-tam",
		Settings: &SettingsInput{Draws: ptr("players"), Fate: ptr("gone")}, Look: &LookInput{Border: ptr("metal")}})
	fx.must(err)
	id := d.(DirectorDeck).ID

	// Absent keys preserve, a value replaces.
	v, err := fx.svc.Update(bg, camp, id, fx.dir, UpdateDeckInput{Version: patch.Of(1), Name: patch.Of("Fortunes")})
	fx.must(err)
	got := v.(DirectorDeck)
	if got.Name != "Fortunes" || got.HolderEntityID != "h-tam" || got.Settings.Draws != "players" || got.Settings.Fate != "gone" || got.Look.Border != "metal" || got.Version != 2 {
		t.Errorf("after rename: %+v", got)
	}
	// A settings change names only the keys it changes.
	v, err = fx.svc.Update(bg, camp, id, fx.dir, UpdateDeckInput{Version: patch.Of(2), Settings: patch.Of(SettingsInput{Discard: ptr(true)})})
	fx.must(err)
	got = v.(DirectorDeck)
	if !got.Settings.Discard || got.Settings.Draws != "players" || got.Name != "Fortunes" {
		t.Errorf("after settings: %+v", got)
	}
	// An explicit null clears the hero page; null on a name preserves it.
	var in UpdateDeckInput
	if err := json.Unmarshal([]byte(`{"version":3,"holderEntityId":null,"name":null}`), &in); err != nil {
		t.Fatal(err)
	}
	v, err = fx.svc.Update(bg, camp, id, fx.dir, in)
	fx.must(err)
	got = v.(DirectorDeck)
	if got.HolderEntityID != "" || got.Name != "Fortunes" {
		t.Errorf("after null: %+v", got)
	}
	// Stale and missing versions.
	if _, err := fx.svc.Update(bg, camp, id, fx.dir, UpdateDeckInput{Version: patch.Of(1), Name: patch.Of("X")}); code(err) != http.StatusConflict {
		t.Errorf("stale version: code %d, want 409", code(err))
	}
	if _, err := fx.svc.Update(bg, camp, id, fx.dir, UpdateDeckInput{Name: patch.Of("X")}); code(err) != http.StatusUnprocessableEntity {
		t.Errorf("missing version: code %d, want 422", code(err))
	}
	if _, err := fx.svc.Update(bg, camp, id, fx.tam, UpdateDeckInput{Version: patch.Of(4), Name: patch.Of("X")}); code(err) != http.StatusNotFound {
		t.Errorf("player on an unlisted deck: code %d, want 404", code(err))
	}
	// Create rules.
	for _, tt := range []struct {
		name string
		in   CreateDeckInput
		want int
	}{
		{"duplicate name", CreateDeckInput{Name: "Fortunes", Kind: KindEvent}, http.StatusConflict},
		{"no name", CreateDeckInput{Kind: KindEvent}, http.StatusUnprocessableEntity},
		{"bad kind", CreateDeckInput{Name: "Z", Kind: "solo"}, http.StatusUnprocessableEntity},
		{"holder on a party deck", CreateDeckInput{Name: "Z", Kind: KindParty, HolderEntityID: "h-tam"}, http.StatusUnprocessableEntity},
		{"holder outside the campaign", CreateDeckInput{Name: "Z", Kind: KindHero, HolderEntityID: "h-x"}, http.StatusUnprocessableEntity},
		{"bad setting", CreateDeckInput{Name: "Z", Kind: KindHero, Settings: &SettingsInput{Draws: ptr("x")}}, http.StatusUnprocessableEntity},
		{"player creating", CreateDeckInput{Name: "Z", Kind: KindEvent}, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viewer := fx.dir
			if tt.name == "player creating" {
				viewer = fx.tam
			}
			if _, err := fx.svc.Create(bg, camp, viewer, tt.in); code(err) != tt.want {
				t.Errorf("code = %d (%v), want %d", code(err), err, tt.want)
			}
		})
	}
}

func TestCardPartialUpdate(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	c := fx.card(deck, CreateCardInput{Name: "A", Words: "words", DirectorNote: "note", Copies: ptr(3), Fate: FateGone, AskFirst: true})
	if _, err := fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Name: patch.Of("B")}); err != nil {
		t.Fatal(err)
	}
	got := fx.repo.cards[c]
	if got.Name != "B" || got.Words != "words" || got.DirectorNote != "note" || got.Copies != 3 || got.Fate != FateGone || !got.AskFirst {
		t.Errorf("a rename changed more: %+v", got)
	}
	if _, err := fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Fate: patch.Of("")}); err != nil {
		t.Fatal(err)
	}
	if fx.repo.cards[c].Fate != "" {
		t.Error("an empty fate means the deck's default")
	}
	if _, err := fx.svc.UpdateCard(bg, camp, deck, "missing", fx.dir, UpdateCardInput{}); code(err) != http.StatusNotFound {
		t.Errorf("missing card: code %d", code(err))
	}
	if _, err := fx.svc.UpdateCard(bg, camp, deck, c, fx.tam, UpdateCardInput{}); code(err) != http.StatusNotFound {
		t.Errorf("player on an unlisted deck: code %d, want 404", code(err))
	}
}

func TestChosenPlayers(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	set := func(in ...PlayerInput) error {
		_, err := fx.svc.SetPlayers(bg, camp, deck, fx.dir, SetPlayersInput{Version: fx.repo.decks[deck].Version, Players: in})
		return err
	}
	for _, tt := range []struct {
		name string
		in   []PlayerInput
		want int
	}{
		{"a Director cannot be chosen", []PlayerInput{{UserID: "u-dir"}}, http.StatusUnprocessableEntity},
		{"a stranger cannot be chosen", []PlayerInput{{UserID: "u-nobody"}}, http.StatusUnprocessableEntity},
		{"twice", []PlayerInput{{UserID: "u-tam"}, {UserID: "u-tam"}}, http.StatusUnprocessableEntity},
		{"negative draws", []PlayerInput{{UserID: "u-tam", DrawsLeft: ptr(-1)}}, http.StatusUnprocessableEntity},
		{"too many draws", []PlayerInput{{UserID: "u-tam", DrawsLeft: ptr(MaxDraws + 1)}}, http.StatusUnprocessableEntity},
		{"valid", []PlayerInput{{UserID: "u-tam", DrawsLeft: ptr(3)}, {UserID: "u-scribe"}}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := code(set(tt.in...)); got != tt.want {
				t.Errorf("code = %d, want %d", got, tt.want)
			}
		})
	}
	if _, err := fx.svc.SetPlayers(bg, camp, deck, fx.dir, SetPlayersInput{Version: 1}); code(err) != http.StatusConflict {
		t.Errorf("stale version: code %d, want 409", code(err))
	}
	if _, err := fx.svc.SetPlayers(bg, camp, deck, fx.tam, SetPlayersInput{Version: fx.repo.decks[deck].Version}); code(err) != http.StatusForbidden {
		t.Errorf("a chosen player setting players: code %d, want 403", code(err))
	}
	if _, err := fx.svc.SetPlayers(bg, camp, deck, fx.ilse, SetPlayersInput{Version: fx.repo.decks[deck].Version}); code(err) != http.StatusNotFound {
		t.Errorf("an unlisted player setting players: code %d, want 404", code(err))
	}
	// Saving again keeps a grant's original time.
	before := longAgo.Add(time.Hour)
	d := fx.repo.decks[deck]
	d.Players[0].GrantedAt = before
	fx.repo.decks[deck] = d
	fx.must(set(PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)}))
	if got := fx.repo.decks[deck].Players[0].GrantedAt; !got.Equal(before) {
		t.Errorf("grant re-dated from %v to %v", before, got)
	}
}

// A grant made before the member's present stay counts for nothing.
func TestStaleGrantFromAnEarlierStayIsIgnored(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Secret", KindEvent, nil)
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	if _, err := fx.svc.Get(bg, camp, deck, fx.tam); err != nil {
		t.Fatalf("chosen player: %v", err)
	}
	members := fakeMembers{list: []Member{
		{UserID: "u-dir", Role: 3, Director: true, JoinedAt: longAgo},
		{UserID: "u-tam", Role: 1, JoinedAt: time.Now().Add(time.Hour)},
	}}
	svc := NewService(fx.repo, fx.heroes, members)
	if _, err := svc.Get(bg, camp, deck, fx.tam); code(err) != http.StatusNotFound {
		t.Errorf("after rejoining: code %d, want 404", code(err))
	}
}

func TestTable(t *testing.T) {
	fx := newFixture(t)
	event := fx.deck("Omens", KindEvent, nil)
	c := fx.card(event, CreateCardInput{Name: "Eclipse", Words: "The sun goes dark.", DirectorNote: "DIRNOTE"})
	ids := fx.copiesOf(fx.card(event, CreateCardInput{Name: "Down", Words: "DOWNWORDS", Copies: ptr(2)}))
	fx.place(fx.copiesOf(c)[0], ZonePlay, "", true)
	fx.place(ids[0], ZonePlay, "", false)
	fx.place(ids[1], ZonePlay, "h-tam", true)
	for _, v := range []Viewer{fx.dir, fx.tam, fx.scribe} {
		tv, err := fx.svc.Table(bg, camp, v)
		fx.must(err)
		raw, _ := json.Marshal(tv)
		s := string(raw)
		if !strings.Contains(s, "Eclipse") || !strings.Contains(s, "The sun goes dark.") {
			t.Errorf("%s: the table card is missing: %s", v.UserID, s)
		}
		for _, bad := range []string{"DIRNOTE", "DOWNWORDS", "directorNote"} {
			if strings.Contains(s, bad) {
				t.Errorf("%s: table leaks %q: %s", v.UserID, bad, s)
			}
		}
	}
	for _, v := range []Viewer{fx.former, fx.visitor} {
		if _, err := fx.svc.Table(bg, camp, v); code(err) != http.StatusNotFound {
			t.Errorf("%q: code %d, want 404", v.UserID, code(err))
		}
	}
}
