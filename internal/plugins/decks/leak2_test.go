package decks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A copy that has been seen beside a face never comes back as a hidden card
// under the same id.
func TestReturnedCopyGetsAFreshID(t *testing.T) {
	paths := []struct {
		name string
		back func(fx *fixture, deck, copyID string) error
	}{
		{"take back", func(fx *fixture, deck, id string) error {
			_, err := fx.svc.Act(bg, camp, deck, id, "take-back", fx.dir, ActInput{})
			return err
		}},
		{"gather", func(fx *fixture, deck, _ string) error { _, err := fx.svc.Gather(bg, camp, deck, fx.dir); return err }},
		{"shuffle used back", func(fx *fixture, deck, id string) error {
			if _, err := fx.svc.Act(bg, camp, deck, id, "take-off", fx.dir, ActInput{}); err != nil {
				return err
			}
			_, err := fx.svc.Shuffle(bg, camp, deck, fx.dir)
			return err
		}},
		{"fate back", func(fx *fixture, deck, id string) error {
			if _, err := fx.svc.UpdateCard(bg, camp, deck, fx.cardID(deck, "A"), fx.dir, UpdateCardInput{Fate: patchOf(FateBack)}); err != nil {
				return err
			}
			_, err := fx.svc.Act(bg, camp, deck, id, "take-off", fx.dir, ActInput{})
			return err
		}},
		{"put back from shown", nil},
	}
	for _, tt := range paths {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("D", KindEvent, &SettingsInput{Arrives: ptr("down")})
			c := fx.card(deck, CreateCardInput{Name: "A", Words: "AWORDS"})
			seen := fx.copiesOf(c)[0]
			if tt.back == nil {
				fx.place(seen, ZoneShown, "", false)
				_, err := fx.svc.Act(bg, camp, deck, seen, "put-back", fx.dir, ActInput{})
				fx.must(err)
			} else {
				fx.place(seen, ZonePlay, "", true) // on the table, id and face public
				fx.must(tt.back(fx, deck, seen))
			}
			if got := fx.copiesOf(c)[0]; got == seen {
				t.Fatalf("the copy kept id %s through its return to the pile", seen)
			}
			if z := fx.copy(fx.copiesOf(c)[0]).Zone; z != ZonePile {
				t.Fatalf("copy is in %s, want the pile", z)
			}
			res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "hand", HeroID: "h-tam"})
			fx.must(err)
			if res.CopyID == seen {
				t.Errorf("the card was dealt face down under the id %s a player saw on its face", seen)
			}
			raw, _ := json.Marshal(mustGet(t, fx, deck, fx.tam))
			if strings.Contains(string(raw), seen) {
				t.Errorf("the old id is in a player's reply: %s", raw)
			}
		})
	}
}

func mustGet(t *testing.T, fx *fixture, deck string, v Viewer) DeckView {
	t.Helper()
	d, err := fx.svc.Get(bg, camp, deck, v)
	fx.must(err)
	return d
}

// The log reads by card name even after the copy's id changed.
func TestLogNamesTheCardAfterItsCopyChangedID(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	c := fx.card(deck, CreateCardInput{Name: "Ember"})
	id := fx.copiesOf(c)[0]
	fx.place(id, ZoneHand, "h-tam", true)
	_, err := fx.svc.Act(bg, camp, deck, id, "take-back", fx.dir, ActInput{})
	fx.must(err)
	view := mustGet(t, fx, deck, fx.dir).(DirectorDeck)
	if len(view.Log) != 1 || view.Log[0].CardName != "Ember" {
		t.Errorf("log = %+v", view.Log)
	}
}

// With knows = kinds a player is told how many cards of each kind they cannot
// see the face of, so dealing a card face down changes nothing they are told.
func TestKindCountsCountEveryUnseenCopy(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face")})
	_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
		Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "Bane", Colour: "rose"}}})})
	fx.must(err)
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	boon := fx.card(deck, CreateCardInput{Name: "B1", KindKey: "boon", Copies: ptr(2)})
	bane := fx.card(deck, CreateCardInput{Name: "N1", KindKey: "bane", Copies: ptr(1)})
	counts := func(v Viewer) string {
		pd := mustGet(t, fx, deck, v).(PlayerDeck)
		var parts []string
		for _, k := range pd.Kinds {
			parts = append(parts, k.KindKey+"="+itoa(k.Count))
		}
		return strings.Join(parts, ",")
	}
	before := counts(fx.tam)
	if before != "bane=1,boon=2" {
		t.Fatalf("counts = %s", before)
	}
	// Face down into anyone's hand, the pile loses a boon but the count holds.
	_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: boon, To: "hand", HeroID: "h-ilse"})
	fx.must(err)
	_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: bane, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	if got := counts(fx.tam); got != before {
		t.Errorf("after dealing face down: %s, want %s", got, before)
	}
	// Drawn to shown, played and used: the player cannot see any of them.
	res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{})
	fx.must(err)
	if got := counts(fx.tam); got != before {
		t.Errorf("after drawing to shown: %s, want %s", got, before)
	}
	_ = res
	// Only a face the player is shown changes a count: a card on the table.
	_, err = fx.svc.Act(bg, camp, deck, res.CopyID, "deal", fx.dir, ActInput{To: "table"})
	fx.must(err)
	if got := counts(fx.tam); got != "bane=1,boon=1" {
		t.Errorf("after a boon lands face up on the table: %s", got)
	}
}

// Copies in a player's reply are listed by when they arrived, then by id,
// never by which card they are.
func TestPlayerListsAreOrderedByArrival(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindParty, nil)
	zed := fx.card(deck, CreateCardInput{Name: "Zed"})
	aaa := fx.card(deck, CreateCardInput{Name: "Aaa"})
	mid := fx.card(deck, CreateCardInput{Name: "Mid"})
	// Arrival order is Mid, Zed, Aaa: the reverse of neither name nor card order.
	order := []string{fx.copiesOf(mid)[0], fx.copiesOf(zed)[0], fx.copiesOf(aaa)[0]}
	for _, id := range order {
		fx.place(id, ZoneHand, "h-tam", false)
	}
	for _, id := range order {
		fx.place(id, ZoneUsed, "", true)
	}
	for _, zone := range []string{ZoneHand, ZoneUsed} {
		for _, id := range order {
			fx.place(id, zone, map[string]string{ZoneHand: "h-tam", ZoneUsed: ""}[zone], zone == ZoneUsed)
		}
		pd := mustGet(t, fx, deck, fx.tam).(PlayerDeck)
		var got []string
		if zone == ZoneHand {
			for _, c := range pd.Hands[0].Hand {
				got = append(got, c.ID)
			}
		} else {
			for _, c := range pd.Used {
				got = append(got, c.ID)
			}
		}
		if strings.Join(got, ",") != strings.Join(order, ",") {
			t.Errorf("%s listed %v, want arrival order %v", zone, got, order)
		}
	}
	// Equal arrival times fall back to the id.
	same := fx.repo.copies[0].MovedAt
	for i := range fx.repo.copies {
		fx.repo.copies[i].MovedAt = same
	}
	pd := mustGet(t, fx, deck, fx.tam).(PlayerDeck)
	for i := 1; i < len(pd.Used); i++ {
		if pd.Used[i-1].ID > pd.Used[i].ID {
			t.Errorf("tied copies not ordered by id: %v", pd.Used)
		}
	}
}

// A player's repeat turn is refused before anything is written.
func TestRepeatedTurnWritesNothing(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindHero, nil)
	c := fx.card(deck, CreateCardInput{Name: "A"})
	id := fx.copiesOf(c)[0]
	fx.place(id, ZoneHand, "h-tam", false)
	if _, err := fx.svc.Act(bg, camp, deck, id, "turn", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	lines := len(fx.repo.logs)
	announced := len(fx.cap.got)
	for i := 0; i < 50; i++ {
		if _, err := fx.svc.Act(bg, camp, deck, id, "turn", fx.tam, ActInput{}); code(err) != http.StatusConflict {
			t.Fatalf("repeat turn: code %d, want 409", code(err))
		}
	}
	if len(fx.repo.logs) != lines || len(fx.cap.got) != announced {
		t.Errorf("repeat turns wrote %d lines and %d notices", len(fx.repo.logs)-lines, len(fx.cap.got)-announced)
	}
	if !fx.copy(id).FaceUp {
		t.Error("the card went back down")
	}
	// A Director may turn it back.
	if _, err := fx.svc.Act(bg, camp, deck, id, "turn", fx.dir, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if fx.copy(id).FaceUp {
		t.Error("the Director's turn did not take it down")
	}
}

// From the same starting state, deal card X face down in one world and card Y
// in another. What every player is sent is the same bytes in both, apart from
// the fresh copy's id. X and Y differ in kind, words, fate and ask-first.
func TestTwoWorldsPlayersCannotTellWhichCardWasDealtDown(t *testing.T) {
	run := func(t *testing.T, deal string, steps func(fx *fixture, deck, cardID string)) map[string]string {
		fx := newFixture(t)
		deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face"), Draws: ptr("players")})
		_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
			Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "Bane", Colour: "rose"}}})})
		fx.must(err)
		fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)}, PlayerInput{UserID: "u-scribe"})
		x := fx.card(deck, CreateCardInput{Name: "Xcard", KindKey: "boon", Words: "XWORDS", DirectorNote: "XNOTE", Fate: FateUsed, Picture: "pic-xx"})
		y := fx.card(deck, CreateCardInput{Name: "Ycard", KindKey: "bane", Words: "YWORDS", DirectorNote: "YNOTE", Fate: FateBack, AskFirst: true, WhenOpen: true, Picture: "pic-yy"})
		fx.card(deck, CreateCardInput{Name: "Filler", KindKey: "boon", Copies: ptr(2)})
		cardID := map[string]string{"X": x, "Y": y}[deal]
		res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: cardID, To: "hand", HeroID: "h-tam"})
		fx.must(err)
		if steps != nil {
			steps(fx, deck, cardID)
		}
		dealt := res.CopyID
		out := map[string]string{}
		for name, v := range map[string]Viewer{"tam": fx.tam, "ilse": fx.ilse, "scribe": fx.scribe} {
			list, err := fx.svc.List(bg, camp, v)
			fx.must(err)
			raw, _ := json.Marshal(list)
			out[name+"/list"] = string(raw)
			if d, err := fx.svc.Get(bg, camp, deck, v); err == nil {
				raw, _ := json.Marshal(d)
				out[name+"/deck"] = string(raw)
			}
			table, err := fx.svc.Table(bg, camp, v)
			fx.must(err)
			raw, _ = json.Marshal(table)
			out[name+"/table"] = string(raw)
		}
		for k, v := range out {
			// The deck's own id is random per run; the fresh copy's id is the
			// one thing that is meant to differ.
			out[k] = strings.ReplaceAll(strings.ReplaceAll(v, dealt, "FRESHCOPY"), deck, "DECK")
		}
		return out
	}
	scenarios := map[string]func(fx *fixture, deck, cardID string){
		"just dealt": nil,
		"returned and dealt again": func(fx *fixture, deck, cardID string) {
			id := fx.copiesOf(cardID)[0]
			_, err := fx.svc.Act(bg, camp, deck, id, "take-back", fx.dir, ActInput{})
			fx.must(err)
			_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: cardID, To: "hand", HeroID: "h-tam"})
			fx.must(err)
		},
	}
	for name, steps := range scenarios {
		t.Run(name, func(t *testing.T) {
			// The second scenario deals again, so the id to hide is the last one dealt.
			a, b := run(t, "X", steps), run(t, "Y", steps)
			for k := range a {
				if a[k] != b[k] {
					t.Errorf("%s differs between worlds:\n X: %s\n Y: %s", k, a[k], b[k])
				}
			}
			if len(a) == 0 || len(a) != len(b) {
				t.Errorf("worlds returned %d and %d replies", len(a), len(b))
			}
			for _, k := range []string{"tam/deck", "ilse/deck", "tam/table"} {
				for _, bad := range []string{"XWORDS", "YWORDS", "XNOTE", "YNOTE", "Xcard", "Ycard", "pic-xx", "pic-yy"} {
					if strings.Contains(a[k], bad) || strings.Contains(b[k], bad) {
						t.Errorf("%s names %q", k, bad)
					}
				}
			}
		})
	}
}
