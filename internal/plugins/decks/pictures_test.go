package decks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Saving a card refuses every picture the media side does not vouch for with
// one and the same message, so the refusal cannot be used to probe which ids
// exist in another campaign. A URL or a path never reaches the media side.
func TestCardPictureSaveRefusals(t *testing.T) {
	refused := []struct{ name, picture string }{
		{"a URL", "https://example.test/a.png"},
		{"a path", "../../etc/passwd"},
		{"a made-up id", "no-such-file"},
		{"a file of another campaign", "other-camp-file"},
		{"a non-image or page file", "pic-page"},
		{"a trashed file", "pic-trashed"},
		{"too long", "pic-" + strings.Repeat("a", 300)},
	}
	var first string
	for _, tt := range refused {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("D", KindEvent, nil)
			_, err := fx.svc.CreateCard(bg, camp, deck, fx.dir, CreateCardInput{Name: "A", Picture: tt.picture})
			if code(err) != http.StatusUnprocessableEntity {
				t.Fatalf("create: code %d (%v), want 422", code(err), err)
			}
			if first == "" {
				first = err.Error()
			} else if err.Error() != first {
				t.Errorf("refusal reads %q, other refusals read %q", err.Error(), first)
			}
			// The same on an edit.
			c := fx.card(deck, CreateCardInput{Name: "B"})
			_, err = fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Picture: patchOf(tt.picture)})
			if code(err) != http.StatusUnprocessableEntity || err.Error() != first {
				t.Errorf("update: %v, want the same refusal %q", err, first)
			}
		})
	}

	t.Run("a service without the media side refuses every picture", func(t *testing.T) {
		fx := newFixture(t)
		svc := NewService(fx.repo, fx.heroes, fakeMembers{})
		deck := fx.deck("D", KindEvent, nil)
		_, err := svc.CreateCard(bg, camp, deck, fx.dir, CreateCardInput{Name: "A", Picture: "pic-a"})
		if code(err) != http.StatusUnprocessableEntity {
			t.Errorf("code %d, want 422", code(err))
		}
	})

	t.Run("a usable picture is saved, kept through other edits, and cleared by null", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, nil)
		c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
		if got := fx.repo.cards[c].Picture; got != "pic-a" {
			t.Fatalf("stored picture %q", got)
		}
		// Words change, the picture is absent from the body: it is kept.
		_, err := fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Words: patchOf("new")})
		fx.must(err)
		if got := fx.repo.cards[c].Picture; got != "pic-a" {
			t.Errorf("an edit of the words changed the picture to %q", got)
		}
		// The file is trashed later: an edit of the words must still work.
		fx.repo.cards[c] = func() Card { x := fx.repo.cards[c]; x.Picture = "pic-trashed"; return x }()
		_, err = fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Words: patchOf("newer")})
		fx.must(err)
		// Naming the same stored picture again is not a change, so it is not re-checked.
		_, err = fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Picture: patchOf("pic-trashed")})
		fx.must(err)
		// Clearing is always allowed.
		_, err = fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Picture: patchOf("")})
		fx.must(err)
		if got := fx.repo.cards[c].Picture; got != "" {
			t.Errorf("picture after clearing = %q", got)
		}
	})
}

// A face carries its card's picture, a face-down copy carries nothing of it,
// and a Director's card has it.
func TestFaceCarriesPictureAndFaceDownCarriesNone(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, &SettingsInput{Arrives: ptr("down")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a", Copies: ptr(2)})
	ids := fx.copiesOf(c)
	fx.place(ids[0], ZoneHand, "h-tam", true)
	fx.place(ids[1], ZoneHand, "h-tam", false)

	pd := mustGet(t, fx, deck, fx.tam).(PlayerDeck)
	var up, down *PlayerCopy
	for i := range pd.Hands[0].Hand {
		cp := &pd.Hands[0].Hand[i]
		if cp.ID == ids[0] {
			up = cp
		} else {
			down = cp
		}
	}
	if up == nil || up.Face == nil || up.Face.Picture != "pic-a" {
		t.Fatalf("the face-up copy's face = %+v, want picture pic-a", up)
	}
	raw, _ := json.Marshal(down)
	if strings.Contains(string(raw), "pic-a") || strings.Contains(string(raw), "picture") || down.Face != nil {
		t.Errorf("the face-down copy carries picture data: %s", raw)
	}
	dd := mustGet(t, fx, deck, fx.dir).(DirectorDeck)
	if len(dd.Cards) != 1 || dd.Cards[0].Picture != "pic-a" {
		t.Errorf("the Director's card = %+v", dd.Cards)
	}
}

// seen is CardPictureSeenBy for the fixture's one picture.
func (fx *fixture) seen(v Viewer, picture string) bool {
	fx.t.Helper()
	ok, err := fx.svc.CardPictureSeenBy(bg, camp, picture, v)
	fx.must(err)
	return ok
}

// The picture of a card is open to a player exactly while a copy of the card
// is face up to them, through the whole life of the copy.
func TestCardPictureFollowsTheCardsFace(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{Arrives: ptr("down")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
	fx.card(deck, CreateCardInput{Name: "B", Picture: "pic-b"})

	if fx.seen(fx.tam, "pic-a") {
		t.Fatal("a card in the pile shows its picture")
	}
	id := fx.copiesOf(c)[0]
	fx.place(id, ZoneHand, "h-tam", true)
	if !fx.seen(fx.tam, "pic-a") {
		t.Fatal("a face-up card in the player's own hand hides its picture")
	}
	if fx.seen(fx.tam, "pic-b") {
		t.Error("another card's picture opened with it")
	}
	if fx.seen(fx.ilse, "pic-a") {
		t.Error("another player's face-up card opened to the wrong hero (the deck is not theirs)")
	}

	// Back to the pile and dealt face down (the copy gets a fresh id).
	_, err := fx.svc.Act(bg, camp, deck, id, "take-back", fx.dir, ActInput{})
	fx.must(err)
	_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	if fx.seen(fx.tam, "pic-a") {
		t.Fatal("a face-down card shows its picture")
	}
	// They turn it over.
	fresh := fx.copiesOf(c)[0]
	_, err = fx.svc.Act(bg, camp, deck, fresh, "turn", fx.tam, ActInput{})
	fx.must(err)
	if !fx.seen(fx.tam, "pic-a") {
		t.Fatal("turning the card over did not open its picture to the holder")
	}
	// Played to the used pile of an event deck: nobody reads it any more.
	_, err = fx.svc.Act(bg, camp, deck, fresh, "play", fx.tam, ActInput{})
	fx.must(err)
	if fx.seen(fx.tam, "pic-a") {
		t.Error("a card in a non-party used pile still shows its picture")
	}
}

// Other heroes' cards: a turned card is private, a shown one opens only where
// the deck lets others see faces.
func TestCardPictureOfAnotherHerosCard(t *testing.T) {
	for _, tt := range []struct {
		others    string
		afterShow bool
	}{{"face", true}, {"back", false}, {"nothing", false}} {
		t.Run(tt.others, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("Omens", KindEvent, &SettingsInput{Arrives: ptr("down"), OthersSee: ptr(tt.others)})
			fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
			c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
			_, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "hand", HeroID: "h-tam"})
			fx.must(err)
			id := fx.copiesOf(c)[0]
			if fx.seen(fx.ilse, "pic-a") {
				t.Fatal("a face-down card in another hero's hand shows its picture")
			}
			_, err = fx.svc.Act(bg, camp, deck, id, "turn", fx.tam, ActInput{})
			fx.must(err)
			if !fx.seen(fx.tam, "pic-a") || fx.seen(fx.ilse, "pic-a") {
				t.Fatal("a turned card must open to its holder only")
			}
			_, err = fx.svc.Act(bg, camp, deck, id, "show", fx.tam, ActInput{})
			fx.must(err)
			if got := fx.seen(fx.ilse, "pic-a"); got != tt.afterShow {
				t.Errorf("after show, othersSee=%s: other player sees picture = %v, want %v", tt.others, got, tt.afterShow)
			}
		})
	}
}

// The rest of the places a face can be read: the table, the party's hand, a
// party deck's used pile, and a deck that lets players know every face.
func TestCardPictureOtherPlaces(t *testing.T) {
	t.Run("on the table, face up, any member and even an event deck", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, nil)
		c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
		id := fx.copiesOf(c)[0]
		if fx.seen(fx.scribe, "pic-a") {
			t.Fatal("pile card shown")
		}
		fx.place(id, ZonePlay, "", false)
		if fx.seen(fx.scribe, "pic-a") {
			t.Error("a face-down card on the table shows its picture")
		}
		fx.place(id, ZonePlay, "", true)
		if !fx.seen(fx.scribe, "pic-a") || !fx.seen(fx.tam, "pic-a") {
			t.Error("a face-up table card hides its picture from a member")
		}
		if fx.seen(fx.former, "pic-a") || fx.seen(fx.visitor, "pic-a") {
			t.Error("a former member or visitor sees a table card's picture")
		}
	})
	t.Run("the party's hand", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("Party", KindParty, nil)
		c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
		id := fx.copiesOf(c)[0]
		fx.place(id, ZoneHand, "", false)
		if fx.seen(fx.tam, "pic-a") {
			t.Error("a face-down card in the party's hand shows its picture")
		}
		fx.place(id, ZoneHand, "", true)
		if !fx.seen(fx.tam, "pic-a") || !fx.seen(fx.ilse, "pic-a") {
			t.Error("a face-up card in the party's hand hides its picture from a member")
		}
	})
	t.Run("a used pile only on a party deck", func(t *testing.T) {
		for kind, want := range map[string]bool{KindParty: true, KindEvent: false} {
			fx := newFixture(t)
			deck := fx.deck("D", kind, nil)
			fx.choose(deck, PlayerInput{UserID: "u-tam"})
			c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
			fx.place(fx.copiesOf(c)[0], ZoneUsed, "", true)
			if got := fx.seen(fx.tam, "pic-a"); got != want {
				t.Errorf("%s deck used pile: seen = %v, want %v", kind, got, want)
			}
		}
	})
	t.Run("knows faces shows every card of a listed deck, and only to those it is listed to", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, &SettingsInput{Knows: ptr("faces")})
		fx.choose(deck, PlayerInput{UserID: "u-tam"})
		fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
		if !fx.seen(fx.tam, "pic-a") {
			t.Error("a chosen player of a knows-faces deck cannot see a pile card's picture")
		}
		if fx.seen(fx.ilse, "pic-a") {
			t.Error("a player the deck is not listed to sees the picture")
		}
	})
	t.Run("directors always, outsiders never", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, nil)
		fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a"})
		if !fx.seen(fx.dir, "pic-a") {
			t.Error("a Director cannot see a card's picture")
		}
		if fx.seen(fx.former, "pic-a") || fx.seen(fx.visitor, "pic-a") {
			t.Error("a non-member sees a card's picture")
		}
	})
	t.Run("a file that is no card's picture is not seen", func(t *testing.T) {
		fx := newFixture(t)
		fx.card(fx.deck("D", KindEvent, nil), CreateCardInput{Name: "A", Picture: "pic-a"})
		if fx.seen(fx.tam, "pic-other") {
			t.Error("an unknown file reads as seen")
		}
	})
}

// Two copies of one card: one face down and one face up. The picture is open.
func TestCardPictureOpenWhileAnyCopyIsFaceUp(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, &SettingsInput{Arrives: ptr("down")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	c := fx.card(deck, CreateCardInput{Name: "A", Picture: "pic-a", Copies: ptr(2)})
	ids := fx.copiesOf(c)
	fx.place(ids[0], ZoneHand, "h-tam", false)
	fx.place(ids[1], ZoneHand, "h-tam", true)
	if !fx.seen(fx.tam, "pic-a") {
		t.Error("the picture is closed while one copy is face up")
	}
	fx.place(ids[1], ZoneHand, "h-tam", false)
	if fx.seen(fx.tam, "pic-a") {
		t.Error("the picture is open with both copies face down")
	}
}

func TestCardPictureListingHelpers(t *testing.T) {
	fx := newFixture(t)
	d1 := fx.deck("One", KindEvent, nil)
	d2 := fx.deck("Two", KindEvent, nil)
	fx.card(d1, CreateCardInput{Name: "A", Picture: "pic-a"})
	fx.card(d1, CreateCardInput{Name: "B"})
	fx.card(d2, CreateCardInput{Name: "C", Picture: "pic-a"})
	fx.card(d2, CreateCardInput{Name: "D", Picture: "pic-d"})

	ids, err := fx.svc.CardPictureIDs(bg, camp)
	fx.must(err)
	if strings.Join(ids, ",") != "pic-a,pic-d" {
		t.Errorf("picture ids = %v", ids)
	}
	if ok, err := fx.svc.IsCardPicture(bg, camp, "pic-a"); err != nil || !ok {
		t.Errorf("IsCardPicture(pic-a) = %v, %v", ok, err)
	}
	if ok, _ := fx.svc.IsCardPicture(bg, camp, "pic-none"); ok {
		t.Error("IsCardPicture(pic-none) = true")
	}
	if ok, _ := fx.svc.IsCardPicture(bg, "other-camp", "pic-a"); ok {
		t.Error("a card picture of one campaign is reported in another")
	}
	uses, err := fx.svc.CardPictureUses(bg, camp, "pic-a", fx.dir)
	fx.must(err)
	if len(uses) != 2 {
		t.Errorf("uses for a Director = %+v, want 2", uses)
	}
	for _, v := range []Viewer{fx.tam, fx.scribe, fx.former, fx.visitor} {
		if uses, _ := fx.svc.CardPictureUses(bg, camp, "pic-a", v); len(uses) != 0 {
			t.Errorf("uses for %q = %+v, want none", v.UserID, uses)
		}
	}
}

// Keep for good copies the picture id into the target deck's new card.
func TestKeepCarriesThePictureID(t *testing.T) {
	fx := newFixture(t)
	from := fx.deck("From", KindEvent, nil)
	to := fx.deck("To", KindEvent, nil)
	c := fx.card(from, CreateCardInput{Name: "Relic", Picture: "pic-relic"})
	id := fx.copiesOf(c)[0]
	fx.place(id, ZoneHand, "h-tam", true)
	_, err := fx.svc.Act(bg, camp, from, id, "keep", fx.dir, ActInput{TargetDeckID: to})
	fx.must(err)
	var kept *Card
	for _, card := range fx.repo.cards {
		if card.DeckID == to && card.Name == "Relic" {
			card := card
			kept = &card
		}
	}
	if kept == nil || kept.Picture != "pic-relic" {
		t.Fatalf("kept card = %+v, want picture pic-relic", kept)
	}
}
