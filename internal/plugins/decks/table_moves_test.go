package decks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// setOpen marks a copy as shown to the table, the way a show would have.
func (fx *fixture) setOpen(copyID string, open bool) {
	fx.t.Helper()
	i := fx.repo.index(copyID)
	if i < 0 {
		fx.t.Fatalf("no copy %s", copyID)
	}
	fx.repo.copies[i].Open = open
}

// nudge sets the Director's next and last card through the deck update.
func (fx *fixture) nudge(deck string, next, last patch.Field[string]) error {
	_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{
		Version: patchOf(fx.repo.decks[deck].Version), NextCardID: next, LastCardID: last})
	return err
}

func (fx *fixture) cardOf(copyID string) string { return fx.repo.cards[fx.copy(copyID).CardID].Name }

func names(fx *fixture, ids []string) []string {
	var out []string
	for _, id := range ids {
		out = append(out, fx.cardOf(id))
	}
	return out
}

func TestLogActionNamesArePinned(t *testing.T) {
	want := map[string]string{
		ActDraw: "drawn", ActDeal: "dealt", ActTurn: "turned", ActShow: "shown", ActPlay: "played", ActAsk: "asked",
		ActApprove: "approved", ActTurnDown: "turned-down", ActTakeOff: "taken-off", ActDiscard: "discarded",
		ActTakeBack: "taken-back", ActPutBack: "put-back", ActKeep: "kept", ActShuffle: "shuffled", ActGather: "gathered",
	}
	for got, name := range want {
		if got != name {
			t.Errorf("log action %q, want %q", got, name)
		}
	}
	if len(want) != 15 {
		t.Errorf("%d log actions pinned, want 15", len(want))
	}
}

// ---- draw several, next and last ------------------------------------------

func TestNextAndLastCardsSteerTheDraw(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	a := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(2)})
	b := fx.card(deck, CreateCardInput{Name: "B"})
	c := fx.card(deck, CreateCardInput{Name: "C", Copies: ptr(2)})
	absent := patch.Absent[string]()
	picks := func() (string, string) { return fx.repo.decks[deck].NextCardID, fx.repo.decks[deck].LastCardID }

	// The next card is the first card of a draw and clears once taken; the
	// rest of the draw is random as ever (the fixed pick takes the first).
	fx.must(fx.nudge(deck, patchOf(c), absent))
	res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 3})
	fx.must(err)
	if got := strings.Join(names(fx, res.CopyIDs), ","); got != "C,A,A" {
		t.Errorf("drew %s, want C,A,A", got)
	}
	if n, _ := picks(); n != "" {
		t.Errorf("the next card stayed set after it was drawn: %q", n)
	}
	if _, err := fx.svc.Gather(bg, camp, deck, fx.dir); err != nil {
		t.Fatal(err)
	}

	// The last card is left out while anything else is in the pile, and is
	// drawn, and cleared, only when nothing else is left.
	fx.must(fx.nudge(deck, absent, patchOf(a)))
	res, err = fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 3})
	fx.must(err)
	if got := strings.Join(names(fx, res.CopyIDs), ","); got != "B,C,C" {
		t.Errorf("drew %s, want B,C,C", got)
	}
	if _, l := picks(); l != a {
		t.Errorf("the last card cleared before it was drawn")
	}
	res, err = fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 2})
	fx.must(err)
	if got := strings.Join(names(fx, res.CopyIDs), ","); got != "A,A" {
		t.Errorf("drew %s, want A,A", got)
	}
	if _, l := picks(); l != "" {
		t.Errorf("the last card stayed set after it was drawn: %q", l)
	}
	if _, err := fx.svc.Gather(bg, camp, deck, fx.dir); err != nil {
		t.Fatal(err)
	}

	// A next card with no copy in the pile waits; the draw is an ordinary one.
	fx.place(fx.copiesOf(b)[0], ZoneHand, "h-npc", false)
	fx.must(fx.nudge(deck, patchOf(b), absent))
	res, err = fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{})
	fx.must(err)
	if n, _ := picks(); n != b || fx.cardOf(res.CopyIDs[0]) != "A" {
		t.Errorf("next = %q, drew %s", n, fx.cardOf(res.CopyIDs[0]))
	}

	// A named deal ignores both nudges and clears neither.
	fx.must(fx.nudge(deck, absent, patchOf(c)))
	if _, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "table"}); err != nil {
		t.Fatal(err)
	}
	if n, l := picks(); n != b || l != c {
		t.Errorf("a named deal changed the nudges: next %q last %q", n, l)
	}
}

func TestNextAndLastMustNameACardOfTheDeckAndClear(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	other := fx.deck("Other", KindEvent, nil)
	a := fx.card(deck, CreateCardInput{Name: "A"})
	foreign := fx.card(other, CreateCardInput{Name: "F"})
	absent := patch.Absent[string]()

	for name, id := range map[string]string{"another deck's card": foreign, "no such card": "nope", "not an id": "a b"} {
		for _, which := range []string{"next", "last"} {
			next, last := absent, absent
			if which == "next" {
				next = patchOf(id)
			} else {
				last = patchOf(id)
			}
			if err := fx.nudge(deck, next, last); code(err) != http.StatusUnprocessableEntity {
				t.Errorf("%s as %s: code %d, want 422", name, which, code(err))
			}
		}
	}
	fx.must(fx.nudge(deck, patchOf(a), patchOf(a)))
	d := fx.repo.decks[deck]
	if d.NextCardID != a || d.LastCardID != a {
		t.Fatalf("picks = %q %q", d.NextCardID, d.LastCardID)
	}
	// An update that names neither keeps both.
	_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(d.Version), Name: patchOf("Renamed")})
	fx.must(err)
	if d := fx.repo.decks[deck]; d.NextCardID != a || d.LastCardID != a {
		t.Errorf("a rename changed the picks: %q %q", d.NextCardID, d.LastCardID)
	}
	// An empty string and an explicit null both clear.
	fx.must(fx.nudge(deck, patchOf(""), absent))
	if d := fx.repo.decks[deck]; d.NextCardID != "" || d.LastCardID != a {
		t.Errorf("empty did not clear just the next card: %q %q", d.NextCardID, d.LastCardID)
	}
	var in UpdateDeckInput
	if err := json.Unmarshal([]byte(`{"version":`+itoa(fx.repo.decks[deck].Version)+`,"lastCardId":null}`), &in); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Update(bg, camp, deck, fx.dir, in); err != nil {
		t.Fatal(err)
	}
	if d := fx.repo.decks[deck]; d.LastCardID != "" {
		t.Errorf("null did not clear the last card: %q", d.LastCardID)
	}
	// Deleting the named card clears the field.
	fx.must(fx.nudge(deck, patchOf(a), patchOf(a)))
	if _, err := fx.svc.DeleteCard(bg, camp, deck, a, fx.dir); err != nil {
		t.Fatal(err)
	}
	if d := fx.repo.decks[deck]; d.NextCardID != "" || d.LastCardID != "" {
		t.Errorf("deleting the card left the picks: %q %q", d.NextCardID, d.LastCardID)
	}
	// Only a Director sets them.
	if _, err := fx.svc.Update(bg, camp, deck, fx.tam, UpdateDeckInput{Version: patchOf(fx.repo.decks[deck].Version), NextCardID: patchOf("x")}); code(err) != http.StatusNotFound {
		t.Errorf("a player's update: code %d", code(err))
	}
}

func TestPlayerDrawObeysNextAndLast(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, &SettingsInput{Draws: ptr("players"), Arrives: ptr("down")})
	fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(3)})
	y := fx.card(deck, CreateCardInput{Name: "Y"})
	fx.choose(deck, PlayerInput{UserID: "u-tam"})
	fx.must(fx.nudge(deck, patchOf(y), patch.Absent[string]()))
	res, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
	fx.must(err)
	if got := fx.copy(res.CopyIDs[0]); got.CardID != y || got.HolderEntityID != "h-tam" {
		t.Errorf("the player drew %+v, want the next card %s", got, y)
	}
	if fx.repo.decks[deck].NextCardID != "" {
		t.Error("a player's draw left the next card set")
	}
	// A player draws one card at a time.
	if _, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam", Count: 2}); code(err) != http.StatusUnprocessableEntity {
		t.Errorf("a player's draw of two: code %d, want 422", code(err))
	}
}

// What a player is sent does not change with the Director's nudges, or with
// cards the Director drew and put back.
func TestTwoWorldsNudgesAndLookedAtCardsAreInvisibleToPlayers(t *testing.T) {
	type world func(fx *fixture, deck string, cards map[string]string)
	run := func(t *testing.T, setup world) map[string]string {
		fx := newFixture(t)
		deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face"), Draws: ptr("players")})
		_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
			Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "Bane", Colour: "rose"}}})})
		fx.must(err)
		fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)}, PlayerInput{UserID: "u-scribe"})
		cards := map[string]string{
			"X": fx.card(deck, CreateCardInput{Name: "Xcard", KindKey: "boon", Words: "XWORDS", Copies: ptr(2), Picture: "pic-xx"}),
			"Y": fx.card(deck, CreateCardInput{Name: "Ycard", KindKey: "bane", Words: "YWORDS", Copies: ptr(2), Picture: "pic-yy"}),
			"Z": fx.card(deck, CreateCardInput{Name: "Zcard", KindKey: "boon", Words: "ZWORDS", Copies: ptr(3), Picture: "pic-zz"}),
		}
		_, err = fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: cards["Z"], To: "hand", HeroID: "h-tam"})
		fx.must(err)
		if setup != nil {
			setup(fx, deck, cards)
		}
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
			out[k] = strings.ReplaceAll(v, deck, "DECK")
		}
		return out
	}
	drewFive := func(back int) world {
		return func(fx *fixture, deck string, _ map[string]string) {
			res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 5})
			fx.must(err)
			for _, id := range res.CopyIDs[:back] {
				_, err := fx.svc.Act(bg, camp, deck, id, "put-back", fx.dir, ActInput{})
				fx.must(err)
			}
		}
	}
	drewOne := func(fx *fixture, deck string, _ map[string]string) {
		_, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{})
		fx.must(err)
	}
	nudged := func(fx *fixture, deck string, c map[string]string) {
		fx.must(fx.nudge(deck, patchOf(c["Y"]), patchOf(c["X"])))
	}
	scenarios := []struct {
		name  string
		a, b  world
		wantN int
	}{
		{"next and last set", nil, nudged, 0},
		{"drew five, put all five back", nil, drewFive(5), 0},
		{"drew five, put four back", drewOne, drewFive(4), 0},
		{"nudged and looked at", drewOne, func(fx *fixture, deck string, c map[string]string) { nudged(fx, deck, c); drewFive(4)(fx, deck, c) }, 0},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			a, b := run(t, sc.a), run(t, sc.b)
			if len(a) == 0 || len(a) != len(b) {
				t.Fatalf("worlds returned %d and %d replies", len(a), len(b))
			}
			for k := range a {
				if a[k] != b[k] {
					t.Errorf("%s differs between worlds:\n A: %s\n B: %s", k, a[k], b[k])
				}
				for _, bad := range []string{"nextCardId", "lastCardId", "XWORDS", "YWORDS", "ZWORDS", "pic-xx", "pic-yy", "pic-zz"} {
					if strings.Contains(a[k], bad) || strings.Contains(b[k], bad) {
						t.Errorf("%s names %q", k, bad)
					}
				}
			}
		})
	}
}

// A player's own draw replies with exactly the same bytes whichever card the
// Director's next card made it take.
func TestPlayerDrawReplyIsTheSameWhetherOrNotANextCardIsSet(t *testing.T) {
	run := func(t *testing.T, withNext bool) string {
		fx := newFixture(t)
		deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), Draws: ptr("players")})
		_, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
			Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}, {Key: "bane", Name: "Bane", Colour: "rose"}}})})
		fx.must(err)
		fx.choose(deck, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(3)})
		fx.card(deck, CreateCardInput{Name: "Xcard", KindKey: "boon", Words: "XWORDS", Copies: ptr(2)})
		y := fx.card(deck, CreateCardInput{Name: "Ycard", KindKey: "bane", Words: "YWORDS", Fate: FateBack, AskFirst: true})
		if withNext {
			fx.must(fx.nudge(deck, patchOf(y), patch.Absent[string]()))
		}
		res, err := fx.svc.Draw(bg, camp, deck, fx.tam, DrawInput{HeroID: "h-tam"})
		fx.must(err)
		raw, _ := json.Marshal(res)
		out := strings.ReplaceAll(string(raw), res.CopyID, "FRESHCOPY")
		return strings.ReplaceAll(out, deck, "DECK")
	}
	if a, b := run(t, false), run(t, true); a != b {
		t.Errorf("the player's draw reply differs:\n without: %s\n with:    %s", a, b)
	}
}

func TestAnotherPutsThatCopyBackAndDrawsOne(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(8)})
	res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 5})
	fx.must(err)
	pick := res.CopyIDs[2]
	again, err := fx.svc.Act(bg, camp, deck, pick, "another", fx.dir, ActInput{})
	fx.must(err)
	if len(again.CopyIDs) != 1 || again.CopyIDs[0] == pick {
		t.Errorf("another replied %v after putting back %s", again.CopyIDs, pick)
	}
	if got := fx.repo.ZoneCountsFor(deck).Shown; got != 5 {
		t.Errorf("%d copies shown after another, want 5", got)
	}
	if _, ok, _ := fx.repo.GetCopy(bg, camp, deck, pick); ok {
		t.Errorf("the put-back copy kept its id")
	}
	dd := mustGet(t, fx, deck, fx.dir).(DirectorDeck)
	if len(dd.Log) < 2 || dd.Log[0].Action != ActDraw || dd.Log[1].Action != ActPutBack {
		t.Errorf("log = %+v, want a put-back then a draw", dd.Log[:2])
	}
	// deal works on each shown copy.
	if _, err := fx.svc.Act(bg, camp, deck, res.CopyIDs[0], "deal", fx.dir, ActInput{To: "table"}); err != nil {
		t.Errorf("deal of one of five shown: %v", err)
	}
}

// ---- turning is private, showing is a second step -------------------------

func TestTurningIsPrivateAndShowingIsASecondStep(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), Arrives: ptr("down"), OthersSee: ptr("face")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
	x := fx.card(deck, CreateCardInput{Name: "Xcard", Words: "XWORDS", DirectorNote: "XNOTE"})
	fx.card(deck, CreateCardInput{Name: "Filler", Copies: ptr(2)})
	res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: x, To: "hand", HeroID: "h-tam"})
	fx.must(err)
	id := res.CopyID
	ilse := func() string {
		raw, _ := json.Marshal(mustGet(t, fx, deck, fx.ilse))
		return string(raw)
	}
	before := ilse()

	// A show of a card still face down is refused.
	if _, err := fx.svc.Act(bg, camp, deck, id, "show", fx.tam, ActInput{}); code(err) != http.StatusConflict {
		t.Errorf("showing a face-down card: code %d, want 409", code(err))
	}
	if _, err := fx.svc.Act(bg, camp, deck, id, "turn", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if got := ilse(); got != before {
		t.Errorf("a turn changed what the other player is sent:\n before: %s\n after:  %s", before, got)
	}
	if strings.Contains(ilse(), "XWORDS") {
		t.Error("the other player can read a card that was only turned")
	}
	// The holder sees the face and that it is not yet shown; a Director sees both marks.
	pd := mustGet(t, fx, deck, fx.tam).(PlayerDeck)
	if c := pd.Hands[0].Hand[0]; !c.FaceUp || c.Open || c.Face == nil || c.Face.Words != "XWORDS" {
		t.Errorf("holder's turned card = %+v", c)
	}
	dd := mustGet(t, fx, deck, fx.dir).(DirectorDeck)
	if c := dd.Copies[indexOfCopy(dd, id)]; !c.FaceUp || c.Open {
		t.Errorf("Director's copy after a turn = %+v", c)
	}

	// Only the holder (or a Director) shows it; another player cannot even see it.
	if _, err := fx.svc.Act(bg, camp, deck, id, "show", fx.ilse, ActInput{}); code(err) != http.StatusNotFound {
		t.Errorf("another player's show: code %d, want 404", code(err))
	}
	lines := len(fx.repo.logs)
	if _, err := fx.svc.Act(bg, camp, deck, id, "show", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if got := ilse(); got == before || !strings.Contains(got, "XWORDS") {
		t.Errorf("a show did not give the other player the face: %s", got)
	}
	pd = mustGet(t, fx, deck, fx.tam).(PlayerDeck)
	if c := pd.Hands[0].Hand[0]; !c.Open {
		t.Errorf("holder's shown card = %+v", c)
	}
	dd = mustGet(t, fx, deck, fx.dir).(DirectorDeck)
	if c := dd.Copies[indexOfCopy(dd, id)]; !c.FaceUp || !c.Open {
		t.Errorf("Director's copy after a show = %+v", c)
	}
	if len(fx.repo.logs) != lines+1 || fx.repo.logs[len(fx.repo.logs)-1].Action != ActShow {
		t.Errorf("a show wrote %d lines", len(fx.repo.logs)-lines)
	}
	// A player cannot undo it, and a repeat writes nothing.
	lines, announced := len(fx.repo.logs), len(fx.cap.got)
	for _, verb := range []string{"show", "turn"} {
		if _, err := fx.svc.Act(bg, camp, deck, id, verb, fx.tam, ActInput{}); code(err) != http.StatusConflict {
			t.Errorf("%s of a shown card: code %d, want 409", verb, code(err))
		}
	}
	if len(fx.repo.logs) != lines || len(fx.cap.got) != announced {
		t.Error("a refused repeat wrote a line or a notice")
	}
	// A Director turning it back down closes it again.
	if _, err := fx.svc.Act(bg, camp, deck, id, "turn", fx.dir, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if c := fx.copy(id); c.FaceUp || c.Open {
		t.Errorf("after a Director's turn down: %+v", c)
	}
	if got := ilse(); got != before {
		t.Errorf("a card turned back down is still readable: %s", got)
	}
}

func indexOfCopy(dd DirectorDeck, id string) int {
	for i, c := range dd.Copies {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// A copy that leaves the hand is no longer open, by every route out.
func TestOpenIsClearedWhenACopyLeavesTheHand(t *testing.T) {
	for _, verb := range []string{"play", "discard", "take-back"} {
		t.Run(verb, func(t *testing.T) {
			fx := newFixture(t)
			deck := fx.deck("D", KindHero, &SettingsInput{Fate: ptr(FatePlay), Discard: ptr(true)})
			c := fx.card(deck, CreateCardInput{Name: "A"})
			id := fx.copiesOf(c)[0]
			fx.place(id, ZoneHand, "h-tam", true)
			fx.setOpen(id, true)
			if _, err := fx.svc.Act(bg, camp, deck, id, verb, fx.dir, ActInput{}); err != nil {
				t.Fatal(err)
			}
			if got := fx.copy(fx.copiesOf(c)[0]); got.Open || got.Zone == ZoneHand {
				t.Errorf("after %s: %+v", verb, got)
			}
		})
	}
	t.Run("fate play stays on the table and not open", func(t *testing.T) {
		fx := newFixture(t)
		deck := fx.deck("D", KindHero, &SettingsInput{Fate: ptr(FatePlay)})
		c := fx.card(deck, CreateCardInput{Name: "A"})
		id := fx.copiesOf(c)[0]
		fx.place(id, ZoneHand, "h-tam", true)
		fx.setOpen(id, true)
		if _, err := fx.svc.Act(bg, camp, deck, id, "play", fx.tam, ActInput{}); err != nil {
			t.Fatal(err)
		}
		if got := fx.copy(id); got.Zone != ZonePlay || got.Open {
			t.Errorf("played copy = %+v", got)
		}
	})
}

// A card played in front of a hero is on the table: face up is enough for
// others to read it, whatever its open mark.
func TestPlayedCardInFrontOfAHeroIsReadableByOthers(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{OthersSee: ptr("face"), Fate: ptr(FatePlay)})
	fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
	c := fx.card(deck, CreateCardInput{Name: "A", Words: "AWORDS"})
	id := fx.copiesOf(c)[0]
	fx.place(id, ZoneHand, "h-tam", true)
	raw, _ := json.Marshal(mustGet(t, fx, deck, fx.ilse))
	if strings.Contains(string(raw), "AWORDS") {
		t.Fatal("a turned, unshown card is readable by another player")
	}
	if _, err := fx.svc.Act(bg, camp, deck, id, "play", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(mustGet(t, fx, deck, fx.ilse))
	if !strings.Contains(string(raw), "AWORDS") {
		t.Errorf("a played card is not readable by the table: %s", raw)
	}
}

// ---- the party's hand ------------------------------------------------------

func TestPartyHand(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Fortune", KindParty, &SettingsInput{Arrives: ptr("down")})
	omen := fx.card(deck, CreateCardInput{Name: "Omen", Words: "OMENWORDS", DirectorNote: "OMENNOTE", KindKey: ""})
	ask := fx.card(deck, CreateCardInput{Name: "Ask", Words: "ASKWORDS", AskFirst: true})
	res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: omen, To: "hand"})
	fx.must(err)
	omenID := res.CopyID
	if got := fx.copy(omenID); got.Zone != ZoneHand || got.HolderEntityID != "" || got.FaceUp {
		t.Fatalf("party hand copy = %+v", got)
	}
	// A shown card can be dealt to the party's hand too.
	shown, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{})
	fx.must(err)
	if _, err := fx.svc.Act(bg, camp, deck, shown.CopyID, "deal", fx.dir, ActInput{To: "hand"}); err != nil {
		t.Fatalf("deal from shown to the party hand: %v", err)
	}
	if fx.copy(shown.CopyID).Zone != ZoneHand {
		t.Error("the shown card did not reach the party hand")
	}

	for name, v := range map[string]Viewer{"tam": fx.tam, "ilse": fx.ilse, "scribe": fx.scribe} {
		pd := mustGet(t, fx, deck, v).(PlayerDeck)
		if len(pd.PartyHand) != 2 {
			t.Errorf("%s sees %d copies in the party's hand, want 2", name, len(pd.PartyHand))
		}
		raw, _ := json.Marshal(pd.PartyHand)
		for _, c := range pd.PartyHand {
			b, _ := json.Marshal(c)
			if string(b) != `{"id":"`+c.ID+`","deckId":"`+deck+`","faceUp":false,"waiting":false}` {
				t.Errorf("%s: face-down party hand entry = %s", name, b)
			}
		}
		if strings.Contains(string(raw), "OMENWORDS") || strings.Contains(string(raw), "OMENNOTE") {
			t.Errorf("%s reads a face-down party card", name)
		}
	}
	// A non-member sees nothing and cannot act on it.
	for name, v := range map[string]Viewer{"visitor": fx.visitor, "former": fx.former} {
		if _, err := fx.svc.Get(bg, camp, deck, v); code(err) != http.StatusNotFound {
			t.Errorf("%s: get answered %d, want 404", name, code(err))
		}
		if _, err := fx.svc.Act(bg, camp, deck, omenID, "turn", v, ActInput{}); code(err) != http.StatusNotFound {
			t.Errorf("%s: turn answered %d, want 404", name, code(err))
		}
	}
	// A member turns, and the party reads it; show has nothing to add there.
	if _, err := fx.svc.Act(bg, camp, deck, omenID, "turn", fx.scribe, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Act(bg, camp, deck, omenID, "show", fx.tam, ActInput{}); code(err) != http.StatusConflict {
		t.Errorf("show in the party hand: code %d, want 409", code(err))
	}
	raw, _ := json.Marshal(mustGet(t, fx, deck, fx.ilse))
	if !strings.Contains(string(raw), "OMENWORDS") || strings.Contains(string(raw), "OMENNOTE") {
		t.Errorf("another member should read the turned card and no note: %s", raw)
	}
	// A member plays from it: the default fate is used, and the party deck's
	// used pile is the party's to see.
	if _, err := fx.svc.Act(bg, camp, deck, omenID, "play", fx.tam, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if got := fx.copy(omenID); got.Zone != ZoneUsed {
		t.Errorf("played from the party hand: %+v", got)
	}
	// Ask-first still applies: the play waits for a Director.
	askID := fx.copiesOf(ask)[0]
	fx.place(askID, ZoneHand, "", true)
	if _, err := fx.svc.Act(bg, camp, deck, askID, "play", fx.ilse, ActInput{}); err != nil {
		t.Fatal(err)
	}
	if got := fx.copy(askID); got.Zone != ZoneHand || got.AskedAt == nil {
		t.Errorf("an ask-first play from the party hand = %+v", got)
	}
	if _, err := fx.svc.Act(bg, camp, deck, askID, "approve", fx.tam, ActInput{}); code(err) != http.StatusForbidden {
		t.Errorf("a member approving: code %d, want 403", code(err))
	}
	if _, err := fx.svc.Act(bg, camp, deck, askID, "approve", fx.dir, ActInput{}); err != nil {
		t.Fatal(err)
	}
}

func TestPartyHandDiscardFollowsTheDeckSetting(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		fx := newFixture(t)
		deck := fx.deck("Fortune", KindParty, &SettingsInput{Discard: ptr(allowed)})
		c := fx.card(deck, CreateCardInput{Name: "A"})
		id := fx.copiesOf(c)[0]
		fx.place(id, ZoneHand, "", true)
		_, err := fx.svc.Act(bg, camp, deck, id, "discard", fx.tam, ActInput{})
		switch {
		case allowed && err != nil:
			t.Errorf("discard allowed: %v", err)
		case !allowed && code(err) != http.StatusForbidden:
			t.Errorf("discard off: code %d, want 403", code(err))
		}
		// Face-down discards stay refused, as in a hero's hand.
		fx.place(id, ZoneHand, "", false)
		if _, err := fx.svc.Act(bg, camp, deck, id, "discard", fx.tam, ActInput{}); err == nil {
			t.Error("a member discarded a face-down party card")
		}
	}
}

func TestHandDealWithNoHeroIsOnlyForAPartyDeck(t *testing.T) {
	for _, kind := range []string{KindHero, KindEvent} {
		fx := newFixture(t)
		deck := fx.deck("D", kind, nil)
		c := fx.card(deck, CreateCardInput{Name: "A"})
		if _, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "hand"}); code(err) != http.StatusUnprocessableEntity {
			t.Errorf("%s deck: named deal to a hand with no hero: code %d, want 422", kind, code(err))
		}
		res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{})
		fx.must(err)
		if _, err := fx.svc.Act(bg, camp, deck, res.CopyID, "deal", fx.dir, ActInput{To: "hand"}); code(err) != http.StatusUnprocessableEntity {
			t.Errorf("%s deck: deal to a hand with no hero: code %d, want 422", kind, code(err))
		}
	}
}

func TestPartyDeckKeepsItsKindWhileThePartyHoldsCards(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Fortune", KindParty, nil)
	c := fx.card(deck, CreateCardInput{Name: "A"})
	res, err := fx.svc.DealCard(bg, camp, deck, fx.dir, DealCardInput{CardID: c, To: "hand"})
	fx.must(err)
	if _, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patch.Of(1), Kind: patch.Of(KindEvent)}); code(err) != http.StatusConflict {
		t.Fatalf("changing the kind with a card in the party's hand: code %d, want 409", code(err))
	}
	if _, err := fx.svc.Act(bg, camp, deck, res.CopyID, "take-back", fx.dir, ActInput{}); err != nil {
		t.Fatalf("take back: %v", err)
	}
	if _, err := fx.svc.Update(bg, camp, deck, fx.dir, UpdateDeckInput{Version: patch.Of(1), Kind: patch.Of(KindEvent)}); err != nil {
		t.Errorf("changing the kind once the party's hand is empty: %v", err)
	}
}

// ---- keep for good ---------------------------------------------------------

type keepWorld struct {
	*fixture
	src, dst string
	ember    string // card id in src
	copyID   string
}

func newKeepWorld(t *testing.T, dstKinds []KindDef) *keepWorld {
	fx := newFixture(t)
	w := &keepWorld{fixture: fx}
	w.src = fx.deck("Src", KindEvent, nil)
	w.dst = fx.deck("Dst", KindEvent, nil)
	_, err := fx.svc.Update(bg, camp, w.src, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{
		Kinds: []KindDef{{Key: "boon", Name: "Boon", Colour: "teal"}}})})
	fx.must(err)
	if dstKinds != nil {
		_, err = fx.svc.Update(bg, camp, w.dst, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{Kinds: dstKinds})})
		fx.must(err)
	}
	w.ember = fx.card(w.src, CreateCardInput{Name: "Ember", KindKey: "boon", Words: "EMBERWORDS", DirectorNote: "EMBERNOTE",
		Corner: "IV", Rare: true, Fate: FateGone, AskFirst: true, When: WhenList{{Test: TestSay}}, Copies: ptr(2)})
	w.copyID = fx.copiesOf(w.ember)[0]
	fx.place(w.copyID, ZoneHand, "h-tam", true)
	// Setting the looks up announced them; the tests count what a keep tells.
	w.cap.got = nil
	return w
}

func (w *keepWorld) keep(v Viewer, target string) (*ActionResult, error) {
	return w.svc.Act(bg, camp, w.src, w.copyID, "keep", v, ActInput{TargetDeckID: target})
}

func (w *keepWorld) dstCard(name string) (Card, bool) {
	for _, c := range w.repo.cards {
		if c.DeckID == w.dst && c.Name == name {
			return c, true
		}
	}
	return Card{}, false
}

func TestKeepCopiesTheCardIntoAnotherDeck(t *testing.T) {
	w := newKeepWorld(t, []KindDef{{Key: "boon", Name: "Gift", Colour: "moss"}})
	if _, err := w.keep(w.dir, w.dst); err != nil {
		t.Fatal(err)
	}
	got, ok := w.dstCard("Ember")
	if !ok {
		t.Fatal("the card was not copied")
	}
	if got.Words != "EMBERWORDS" || got.DirectorNote != "EMBERNOTE" || got.Corner != "IV" || !got.Rare || got.Fate != FateGone ||
		!got.AskFirst || got.KindKey != "boon" || got.Copies != 1 || got.Picture != "" {
		t.Errorf("kept card = %+v", got)
	}
	// The condition was written against the source deck and is not carried.
	if len(got.When) != 0 {
		t.Errorf("the condition was carried over: %+v", got)
	}
	copies := w.repo.ZoneCountsFor(w.dst)
	if copies.Pile != 1 || copies.Shown+copies.Hand+copies.Play+copies.Used+copies.Gone != 0 {
		t.Errorf("target zones = %+v, want one copy in the pile", copies)
	}
	if c := w.copy(w.copyID); c.Zone != ZoneGone || c.Open || c.HolderEntityID != "" {
		t.Errorf("source copy = %+v, want gone", c)
	}
	if dd := mustGet(t, w.fixture, w.src, w.dir).(DirectorDeck); dd.Log[0].Action != ActKeep || dd.Log[0].CardName != "Ember" {
		t.Errorf("log = %+v", dd.Log)
	}
	if len(w.cap.got) != 1 {
		t.Errorf("announced %v", w.cap.got)
	}
	// The source card still has both of its copies; one is gone.
	if w.repo.cards[w.ember].Copies != 2 {
		t.Errorf("source card copies = %d", w.repo.cards[w.ember].Copies)
	}
}

func TestKeepDropsAKindTheTargetDoesNotHave(t *testing.T) {
	w := newKeepWorld(t, nil)
	if _, err := w.keep(w.dir, w.dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.dstCard("Ember"); got.KindKey != "" {
		t.Errorf("kind = %q, want none", got.KindKey)
	}
}

func TestKeepMergesIntoACardOfTheSameName(t *testing.T) {
	w := newKeepWorld(t, nil)
	mine := w.card(w.dst, CreateCardInput{Name: "Ember", Words: "TARGETWORDS", Copies: ptr(2)})
	if _, err := w.keep(w.dir, w.dst); err != nil {
		t.Fatal(err)
	}
	got := w.repo.cards[mine]
	if got.Copies != 3 || got.Words != "TARGETWORDS" || got.Rare {
		t.Errorf("merged card = %+v, want 3 copies and the target's own fields", got)
	}
	if n := len(w.copiesOf(mine)); n != 3 {
		t.Errorf("%d copy rows, want 3", n)
	}
	if _, ok := w.dstCard("Ember"); !ok || len(w.repo.cards) != 2 {
		t.Errorf("a second card was made: %d cards", len(w.repo.cards))
	}
}

func TestKeepFromEveryAllowedPlace(t *testing.T) {
	for _, tt := range []struct {
		zone, holder string
		faceUp       bool
		want         int
	}{
		{ZoneShown, "", false, 0}, {ZoneHand, "h-tam", false, 0}, {ZonePlay, "h-tam", true, 0}, {ZonePlay, "", true, 0},
		{ZonePile, "", false, http.StatusConflict}, {ZoneUsed, "", true, http.StatusConflict}, {ZoneGone, "", true, http.StatusConflict},
	} {
		w := newKeepWorld(t, nil)
		w.place(w.copyID, tt.zone, tt.holder, tt.faceUp)
		_, err := w.keep(w.dir, w.dst)
		if code(err) != tt.want {
			t.Errorf("keep from %s: code %d (%v), want %d", tt.zone, code(err), err, tt.want)
		}
		if tt.want != 0 {
			if _, ok := w.dstCard("Ember"); ok || len(w.cap.got) != 0 {
				t.Errorf("a refused keep from %s changed something", tt.zone)
			}
		}
	}
}

func TestKeepTargets(t *testing.T) {
	w := newKeepWorld(t, nil)
	w.repo.decks["elsewhere"] = Deck{ID: "elsewhere", CampaignID: "camp-2", Name: "Elsewhere", Kind: KindEvent}
	for name, target := range map[string]string{"the same deck": w.src, "none": "", "unknown": "nope", "another campaign": "elsewhere"} {
		if _, err := w.keep(w.dir, target); code(err) != http.StatusUnprocessableEntity {
			t.Errorf("%s: code %d, want 422", name, code(err))
		}
	}
	if len(w.cap.got) != 0 || w.copy(w.copyID).Zone != ZoneHand {
		t.Error("a refused keep changed something")
	}
}

func TestKeepIsForDirectorsOnly(t *testing.T) {
	w := newKeepWorld(t, nil)
	for name, tt := range map[string]struct {
		v    Viewer
		want int
	}{"the holder": {w.tam, http.StatusForbidden}, "another player": {w.ilse, http.StatusNotFound},
		"a scribe": {w.scribe, http.StatusNotFound}, "a visitor": {w.visitor, http.StatusNotFound}, "a former member": {w.former, http.StatusNotFound}} {
		if _, err := w.keep(tt.v, w.dst); code(err) != tt.want {
			t.Errorf("%s: code %d, want %d", name, code(err), tt.want)
		}
	}
	if _, ok := w.dstCard("Ember"); ok || w.copy(w.copyID).Zone != ZoneHand {
		t.Error("a refused keep changed something")
	}
}

func TestKeepEnforcesTheTargetDecksLimits(t *testing.T) {
	for _, tt := range []struct {
		name  string
		fill  func(w *keepWorld)
		wantM string
	}{
		{"two hundred cards", func(w *keepWorld) {
			for i := 0; i < MaxCardsPerDeck; i++ {
				w.card(w.dst, CreateCardInput{Name: "C" + itoa(i)})
			}
		}, "200 cards"},
		{"six hundred copies", func(w *keepWorld) {
			for i := 0; i < 30; i++ {
				w.card(w.dst, CreateCardInput{Name: "C" + itoa(i), Copies: ptr(20)})
			}
		}, "600 copies in all"},
		{"twenty copies of the card", func(w *keepWorld) {
			w.card(w.dst, CreateCardInput{Name: "Ember", Copies: ptr(MaxCopiesPerCard)})
		}, "20 copies of this card"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := newKeepWorld(t, nil)
			tt.fill(w)
			cards, copies := len(w.repo.cards), len(w.repo.copies)
			zones := w.repo.ZoneCountsFor(w.dst)
			_, err := w.keep(w.dir, w.dst)
			if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), tt.wantM) {
				t.Fatalf("err = %v, want 422 naming %q", err, tt.wantM)
			}
			if len(w.repo.cards) != cards || len(w.repo.copies) != copies || w.repo.ZoneCountsFor(w.dst) != zones ||
				w.copy(w.copyID).Zone != ZoneHand || len(w.cap.got) != 0 {
				t.Error("a refused keep changed something")
			}
			if dd := mustGet(t, w.fixture, w.src, w.dir).(DirectorDeck); len(dd.Log) != 0 {
				t.Errorf("a refused keep wrote the log: %+v", dd.Log)
			}
		})
	}
}

// ---- rare ------------------------------------------------------------------

func TestRareIsPartOfTheFaceAndNeverSentFaceDown(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{Knows: ptr("kinds"), OthersSee: ptr("face")})
	fx.choose(deck, PlayerInput{UserID: "u-tam"}, PlayerInput{UserID: "u-ilse"})
	down := fx.card(deck, CreateCardInput{Name: "Down", Words: "DOWNWORDS", Rare: true, Copies: ptr(2)})
	up := fx.card(deck, CreateCardInput{Name: "Up", Words: "UPWORDS", Rare: true})
	plain := fx.card(deck, CreateCardInput{Name: "Plain"})
	fx.place(fx.copiesOf(down)[0], ZoneHand, "h-tam", false)
	fx.place(fx.copiesOf(down)[1], ZoneHand, "h-ilse", false)
	fx.place(fx.copiesOf(plain)[0], ZoneHand, "h-ilse", false)

	// With only face-down rare cards in sight, "rare" appears nowhere.
	for name, v := range map[string]Viewer{"tam": fx.tam, "ilse": fx.ilse} {
		raw, _ := json.Marshal(mustGet(t, fx, deck, v))
		if strings.Contains(string(raw), "rare") {
			t.Errorf("%s is sent a rare mark with only face-down cards: %s", name, raw)
		}
		list, _ := json.Marshal(mustList(t, fx.svc, v))
		if strings.Contains(string(list), "rare") {
			t.Errorf("%s's list carries a rare mark: %s", name, list)
		}
	}
	// A face-up rare card carries it, in the face.
	fx.place(fx.copiesOf(up)[0], ZoneHand, "h-tam", true)
	pd := mustGet(t, fx, deck, fx.tam).(PlayerDeck)
	var seen bool
	for _, c := range pd.Hands[0].Hand {
		if c.Face != nil && c.Face.Name == "Up" {
			seen = c.Face.Rare
		}
		if c.Face == nil {
			if b, _ := json.Marshal(c); strings.Contains(string(b), "rare") {
				t.Errorf("a face-down entry carries rare: %s", b)
			}
		}
	}
	if !seen {
		t.Error("a face-up rare card is not marked rare")
	}
	// The table carries it with the face.
	fx.place(fx.copiesOf(down)[0], ZonePlay, "", true)
	tv, err := fx.svc.Table(bg, camp, fx.scribe)
	fx.must(err)
	if !tv.Decks[0].Cards[0].Face.Rare {
		t.Error("a rare card on the table is not marked rare")
	}
	// A Director sees it on the card.
	dd := mustGet(t, fx, deck, fx.dir).(DirectorDeck)
	for _, c := range dd.Cards {
		if c.Rare != (c.Name != "Plain") {
			t.Errorf("Director's card %s rare = %v", c.Name, c.Rare)
		}
	}
}

func TestRarePartialUpdate(t *testing.T) {
	fx := newFixture(t)
	deck := fx.deck("D", KindEvent, nil)
	c := fx.card(deck, CreateCardInput{Name: "A", Words: "words", Rare: true, Copies: ptr(2)})
	if _, err := fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Words: patchOf("new")}); err != nil {
		t.Fatal(err)
	}
	if !fx.repo.cards[c].Rare {
		t.Error("a words-only edit cleared the rare flag")
	}
	if _, err := fx.svc.UpdateCard(bg, camp, deck, c, fx.dir, UpdateCardInput{Rare: patchOf(false)}); err != nil {
		t.Fatal(err)
	}
	if got := fx.repo.cards[c]; got.Rare || got.Words != "new" || got.Copies != 2 {
		t.Errorf("after clearing rare: %+v", got)
	}
}

// A Director's draw larger than the pile takes what is there instead of
// rolling back as if the pile were empty.
func TestDirectorDrawLargerThanThePileTakesWhatIsThere(t *testing.T) {
	setup := func() (*fixture, string, string, string) {
		fx := newFixture(t)
		deck := fx.deck("D", KindEvent, nil)
		a := fx.card(deck, CreateCardInput{Name: "A", Copies: ptr(2)})
		b := fx.card(deck, CreateCardInput{Name: "B"})
		return fx, deck, a, b
	}
	absent := patch.Absent[string]()
	tests := []struct {
		name  string
		setup func(fx *fixture, deck, a, b string)
		want  string
		// nudge is the card the field is expected to hold afterwards, by name.
		nextAfter, lastAfter string
	}{
		{name: "nothing set", want: "A,A,B"},
		{name: "next set", setup: func(fx *fixture, deck, a, b string) { fx.must(fx.nudge(deck, patchOf(b), absent)) },
			want: "B,A,A"},
		{name: "last set with other cards present", setup: func(fx *fixture, deck, a, b string) {
			fx.must(fx.nudge(deck, absent, patchOf(a)))
		}, want: "B,A,A"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx, deck, a, b := setup()
			if tt.setup != nil {
				tt.setup(fx, deck, a, b)
			}
			res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 5})
			fx.must(err)
			if got := strings.Join(names(fx, res.CopyIDs), ","); got != tt.want {
				t.Errorf("drew %s, want %s", got, tt.want)
			}
			if len(res.CopyIDs) != 3 {
				t.Errorf("the reply lists %d copies, want the 3 the pile held", len(res.CopyIDs))
			}
			d := fx.repo.decks[deck]
			if d.NextCardID != "" || d.LastCardID != "" {
				t.Errorf("a drawn pick stayed set: next %q last %q", d.NextCardID, d.LastCardID)
			}
		})
	}

	t.Run("an empty pile is still the empty answer", func(t *testing.T) {
		fx, deck, _, _ := setup()
		_, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 3})
		fx.must(err)
		_, err = fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 2})
		if code(err) != http.StatusConflict || !strings.Contains(err.Error(), "Nothing left") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("never more than five shown", func(t *testing.T) {
		shownIn := func(fx *fixture, deck string) int {
			n := 0
			for _, c := range fx.repo.copies {
				if c.DeckID == deck && c.Zone == ZoneShown {
					n++
				}
			}
			return n
		}
		// Three shown and a pile of three: the third copy would make six, so
		// the whole draw is refused.
		fx, deck, _, _ := setup()
		held := fx.card(deck, CreateCardInput{Name: "C", Copies: ptr(3)})
		for _, id := range fx.copiesOf(held) {
			fx.place(id, ZoneShown, "", false)
		}
		_, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 5})
		if code(err) != http.StatusConflict || !strings.Contains(err.Error(), "At most 5") {
			t.Fatalf("err = %v", err)
		}
		if n := shownIn(fx, deck); n != 3 {
			t.Errorf("%d shown after a refused draw, want 3", n)
		}
		// Four shown and a pile of one: the draw takes one and reaches five.
		fx = newFixture(t)
		deck = fx.deck("D", KindEvent, nil)
		fx.card(deck, CreateCardInput{Name: "B"})
		held = fx.card(deck, CreateCardInput{Name: "C", Copies: ptr(4)})
		for _, id := range fx.copiesOf(held) {
			fx.place(id, ZoneShown, "", false)
		}
		res, err := fx.svc.Draw(bg, camp, deck, fx.dir, DrawInput{Count: 5})
		fx.must(err)
		if len(res.CopyIDs) != 1 || shownIn(fx, deck) != 5 {
			t.Errorf("drew %d, %d shown; want 1 drawn and 5 shown", len(res.CopyIDs), shownIn(fx, deck))
		}
	})
}
