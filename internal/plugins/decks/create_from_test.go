package decks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// fakeTables is the campaign's rolling tables held in a map, in the order given.
type fakeTables struct {
	order  []string
	lines  map[string][]TableLine
	names  map[string]string
	failed error
}

func newFakeTables() *fakeTables {
	return &fakeTables{lines: map[string][]TableLine{}, names: map[string]string{}}
}

func (f *fakeTables) add(id, name string, lines ...TableLine) {
	f.order = append(f.order, id)
	f.names[id] = name
	f.lines[id] = lines
}

func (f *fakeTables) List(context.Context, string) ([]TableSummary, error) {
	if f.failed != nil {
		return nil, f.failed
	}
	var out []TableSummary
	for _, id := range f.order {
		out = append(out, TableSummary{ID: id, Name: f.names[id], Lines: len(f.lines[id])})
	}
	return out, nil
}

func (f *fakeTables) Lines(_ context.Context, _, id string) ([]TableLine, string, bool, error) {
	if f.failed != nil {
		return nil, "", false, f.failed
	}
	l, ok := f.lines[id]
	return l, f.names[id], ok, nil
}

// fromFixture is the usual campaign with facts and a rolling tables reader.
func fromFixture(t *testing.T) (*fixture, *fakeFacts, *fakeTables, *int) {
	fx := newFixture(t)
	f := newFakeFacts()
	tb := newFakeTables()
	pokes := new(int)
	fx.svc = NewService(fx.repo, fx.heroes, fx.members, WithNotifier(fx.cap), WithBell(fx.cap), WithPictures(fx.pics),
		WithRandom(func(int) int { return 0 }), WithFacts(f), WithMoments(memMoments{repo: fx.repo}), WithTables(tb),
		WithPoke(func(string) { *pokes++ }))
	return fx, f, tb, pokes
}

func (fx *fixture) cardsOf(deckID string) map[string]Card {
	out := map[string]Card{}
	for _, c := range fx.repo.cards {
		if c.DeckID == deckID {
			out[c.Name] = c
		}
	}
	return out
}

func (fx *fixture) pileOf(deckID string) int {
	n := 0
	for _, c := range fx.repo.copies {
		if c.DeckID == deckID && c.Zone == ZonePile {
			n++
		}
	}
	return n
}

func (fx *fixture) from(in CreateFromInput) (DirectorDeck, FromReport, error) {
	fx.t.Helper()
	v, rep, err := fx.svc.CreateFrom(nil2ctx(), camp, fx.dir, in)
	if err != nil {
		return DirectorDeck{}, rep, err
	}
	return v.(DirectorDeck), rep, nil
}

// A copy keeps the look, the rules and every card with what is printed on it,
// all in the pile, and leaves behind who may draw, the draw nudges, "the
// Director says so" and every copy's place.
func TestCopyKeepsLookRulesAndCardsAndDropsTheRest(t *testing.T) {
	fx, _, _, pokes := fromFixture(t)
	src := fx.deck("Iron omens", KindHero, &SettingsInput{Draws: ptr("players"), Knows: ptr("faces"), Fate: ptr("gone")})
	fx.must(func() error {
		_, err := fx.svc.Update(nil2ctx(), camp, src, fx.dir, UpdateDeckInput{Version: patchOf(1), Look: patchOf(LookInput{Border: ptr("metal"), Shape: ptr("small"),
			Back: &BackInput{Pattern: ptr("iron"), Words: ptr("OMENS")}, Kinds: []KindDef{{Key: "omen", Name: "Omen", Colour: "rose", Sign: "diamond"}}})})
		return err
	}())
	a := fx.card(src, CreateCardInput{Name: "The dragon stirs", KindKey: "omen", Words: "It wakes.", DirectorNote: "Only if they were loud.", Picture: "pic-1",
		Corner: "VII", Rare: true, Copies: ptr(3), Fate: "back", AskFirst: true})
	b := fx.card(src, CreateCardInput{Name: "A quiet road", Copies: ptr(2)})
	// What must not travel: chosen players, picks, said, and where copies lie.
	fx.choose(src, PlayerInput{UserID: "u-tam", DrawsLeft: ptr(2)})
	d := fx.repo.decks[src]
	d.NextCardID, d.LastCardID = a, b
	fx.repo.decks[src] = d
	c := fx.repo.cards[a]
	now := longAgo
	c.SaidAt = &now
	fx.repo.cards[a] = c
	fx.place(fx.copiesOf(a)[0], ZoneHand, "h-tam", true)
	fx.place(fx.copiesOf(a)[1], ZoneUsed, "", true)
	*pokes = 0

	got, rep, err := fx.from(CreateFromInput{Name: "Iron omens, again", Kind: KindParty, DeckID: src})
	fx.must(err)

	old := fx.repo.decks[src]
	nu := fx.repo.decks[got.ID]
	if nu.Look.Border != "metal" || nu.Look.Shape != "small" || nu.Look.Back.Pattern != "iron" || nu.Look.Back.Words != "OMENS" || len(nu.Look.Kinds) != 1 {
		t.Errorf("the look did not come: %+v", nu.Look)
	}
	// Who may draw stays behind with the chosen players; every other rule comes.
	want := old.Settings
	want.Draws = "directors"
	if nu.Settings != want {
		t.Errorf("the rules differ: %+v, want %+v", nu.Settings, want)
	}
	if nu.Kind != KindParty || nu.Name != "Iron omens, again" || nu.HolderEntityID != "" {
		t.Errorf("the new deck is not what was typed: %+v", nu)
	}
	if len(nu.Players) != 0 || nu.NextCardID != "" || nu.LastCardID != "" {
		t.Errorf("players or picks came along: %+v", nu)
	}
	cards := fx.cardsOf(got.ID)
	if len(cards) != 2 {
		t.Fatalf("%d cards, want 2", len(cards))
	}
	n := cards["The dragon stirs"]
	if n.ID == a || n.KindKey != "omen" || n.Words != "It wakes." || n.DirectorNote != "Only if they were loud." || n.Picture != "pic-1" || n.Corner != "VII" ||
		!n.Rare || n.Copies != 3 || n.Fate != "back" || !n.AskFirst {
		t.Errorf("the card did not come whole: %+v", n)
	}
	if n.SaidAt != nil || n.FitsSince != nil {
		t.Error("\"the Director says so\" came along")
	}
	if fx.pileOf(got.ID) != 5 {
		t.Errorf("%d copies in the pile, want all 5", fx.pileOf(got.ID))
	}
	for _, cp := range fx.repo.copies {
		if cp.DeckID == got.ID && (cp.Zone != ZonePile || cp.HolderEntityID != "") {
			t.Errorf("a copy of the new deck is not in the pile: %+v", cp)
		}
	}
	// The source is untouched.
	if fx.repo.cards[a].Copies != 3 || fx.copy(fx.copiesOf(a)[0]).Zone != ZoneHand || len(fx.repo.decks[src].Players) != 1 {
		t.Error("the source deck changed")
	}
	if len(rep.Notes) != 0 || rep.Made != 2 {
		t.Errorf("a clean copy reported %+v", rep)
	}
	if *pokes == 0 {
		t.Error("the moments watcher was not told")
	}
}

// Each test is passed through the one check against the new deck: a holder
// test follows the new hero, and one the new deck cannot hold is left off its
// card, never stored, and reported with the card's name.
func TestCopyChecksEachTestAgainstTheNewDeck(t *testing.T) {
	holder := WhenTest{Test: "number", Who: "holder", Field: "hp", Op: "lte", Value: fl(3), Open: true}
	page := WhenTest{Test: "number", Who: "h-tam", Field: "hp", Op: "lte", Value: fl(3)}
	say := WhenTest{Test: "say"}
	goneTag := WhenTest{Test: "tag", Who: "any", Tag: it(99), Has: bl(true)}
	liveTag := WhenTest{Test: "tag", Who: "any", Tag: it(7), Has: bl(true)}
	tests := []struct {
		name     string
		kind     string
		holder   string
		when     []WhenTest
		wantKept int
		wantLeft int
	}{
		{"a holder test follows the new hero", KindHero, "h-ilse", []WhenTest{holder}, 1, 0},
		{"a holder test onto a party deck is left off", KindParty, "", []WhenTest{holder}, 0, 1},
		{"a holder test onto an events deck is left off", KindEvent, "", []WhenTest{holder}, 0, 1},
		{"a holder test onto a hero deck with no hero is left off", KindHero, "", []WhenTest{holder}, 0, 1},
		{"a test on one named page needs no holder", KindParty, "", []WhenTest{page}, 1, 0},
		{"a test whose tag is gone is left off", KindEvent, "", []WhenTest{goneTag}, 0, 1},
		{"one test left off keeps the others", KindParty, "", []WhenTest{holder, liveTag, say, goneTag}, 2, 2},
		{"say tests always come", KindEvent, "", []WhenTest{say}, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx, f, _, _ := fromFixture(t)
			_ = f
			src := fx.deck("Source", KindHero, nil)
			fx.repo.decks[src] = withHolder(fx.repo.decks[src], "h-tam")
			// Stored straight in, as a list is after a tag has since been deleted.
			id := fx.card(src, CreateCardInput{Name: "Waits"})
			c0 := fx.repo.cards[id]
			c0.When = tt.when
			fx.repo.cards[id] = c0
			got, rep, err := fx.from(CreateFromInput{Name: "Copy", Kind: tt.kind, HolderEntityID: tt.holder, DeckID: src})
			fx.must(err)
			c := fx.cardsOf(got.ID)["Waits"]
			if len(c.When) != tt.wantKept {
				t.Errorf("%d tests kept, want %d: %+v", len(c.When), tt.wantKept, c.When)
			}
			left := 0
			for _, l := range rep.TestsLeftOff {
				if l.Card != "Waits" {
					t.Errorf("reported on %q", l.Card)
				}
				left += l.Count
			}
			if left != tt.wantLeft {
				t.Errorf("%d reported left off, want %d", left, tt.wantLeft)
			}
			if (tt.wantLeft > 0) != (len(rep.Notes) > 0) {
				t.Errorf("notes %v for %d left off", rep.Notes, tt.wantLeft)
			}
			// Whatever was stored passes the check on the new deck, as stored.
			nd := fx.repo.decks[got.ID]
			if _, err := fx.svc.ValidateWhen(nil2ctx(), camp, nd, c.When); err != nil {
				t.Errorf("a stored test does not pass the check: %v", err)
			}
			if c.WhenOpen != derivedOpen(c.When) {
				t.Error("when_open is not derived from what was kept")
			}
		})
	}
}

func withHolder(d Deck, holder string) Deck { d.HolderEntityID = holder; return d }

// The sentence says how many were left off and on which cards.
func TestCopyReportSaysWhichCardsLostTests(t *testing.T) {
	fx, _, _, _ := fromFixture(t)
	src := fx.deck("Source", KindHero, nil)
	fx.repo.decks[src] = withHolder(fx.repo.decks[src], "h-tam")
	h := WhenTest{Test: "number", Who: "holder", Field: "hp", Op: "lte", Value: fl(3)}
	for name, w := range map[string][]WhenTest{"Low": {h, h}, "Lower": {h}} {
		id := fx.card(src, CreateCardInput{Name: name})
		c := fx.repo.cards[id]
		c.When = w
		fx.repo.cards[id] = c
	}
	fx.card(src, CreateCardInput{Name: "Plain"})
	_, rep, err := fx.from(CreateFromInput{Name: "Copy", Kind: KindEvent, DeckID: src})
	fx.must(err)
	if len(rep.Notes) != 1 || !strings.Contains(rep.Notes[0], "3 conditions were left off") ||
		!strings.Contains(rep.Notes[0], "Low") || !strings.Contains(rep.Notes[0], "Lower") || strings.Contains(rep.Notes[0], "Plain") {
		t.Errorf("the sentence: %v", rep.Notes)
	}
}

// A picture that can no longer be used is left off, said, and the card stays.
func TestCopyLeavesOffAPictureThatCannotBeUsed(t *testing.T) {
	fx, _, _, _ := fromFixture(t)
	src := fx.deck("Source", KindEvent, nil)
	id := fx.card(src, CreateCardInput{Name: "Pictured", Picture: "pic-ok"})
	c := fx.repo.cards[id]
	c.Picture = "pic-trashed"
	fx.repo.cards[id] = c
	got, rep, err := fx.from(CreateFromInput{Name: "Copy", Kind: KindEvent, DeckID: src})
	fx.must(err)
	if fx.cardsOf(got.ID)["Pictured"].Picture != "" || len(rep.PicturesLeftOff) != 1 || len(rep.Notes) != 1 {
		t.Errorf("picture: %q, report %+v", fx.cardsOf(got.ID)["Pictured"].Picture, rep)
	}
}

// Anything but a deck of this campaign is the same 404 as a deck that is not
// there, and nothing is made.
func TestCopyOfADeckOfAnotherCampaignIsTheSame404(t *testing.T) {
	fx, _, _, _ := fromFixture(t)
	fx.repo.decks["foreign"] = Deck{ID: "foreign", CampaignID: "camp-2", Name: "Theirs", Kind: KindEvent, Settings: defaultSettings(KindEvent), Look: defaultLook(), Version: 1}
	var bodies []string
	for _, id := range []string{"foreign", "no-such-deck", "bad id!", "../x"} {
		_, _, err := fx.from(CreateFromInput{Name: "Copy", Kind: KindEvent, DeckID: id})
		if code(err) != http.StatusNotFound {
			t.Errorf("%q: %v (%d), want 404", id, err, code(err))
			continue
		}
		bodies = append(bodies, err.Error())
	}
	for _, b := range bodies {
		if b != bodies[0] {
			t.Errorf("the 404s differ: %q vs %q", b, bodies[0])
		}
	}
	if n, _ := fx.repo.CountDecks(nil2ctx(), camp); n != 0 {
		t.Errorf("%d decks were made", n)
	}
}

func TestCreateFromRefusals(t *testing.T) {
	fx, _, tb, _ := fromFixture(t)
	src := fx.deck("Source", KindEvent, nil)
	tb.add("loot", "Loot", TableLine{Name: "Gold", Weight: 1})
	tests := []struct {
		name string
		v    Viewer
		in   CreateFromInput
		want int
	}{
		{"a player", fx.tam, CreateFromInput{Name: "X", Kind: KindEvent, DeckID: src}, http.StatusForbidden},
		{"a scribe", fx.scribe, CreateFromInput{Name: "X", Kind: KindEvent, TableID: "loot"}, http.StatusForbidden},
		{"no source", fx.dir, CreateFromInput{Name: "X", Kind: KindEvent}, http.StatusUnprocessableEntity},
		{"two sources", fx.dir, CreateFromInput{Name: "X", Kind: KindEvent, DeckID: src, TableID: "loot"}, http.StatusUnprocessableEntity},
		{"no name", fx.dir, CreateFromInput{Name: "  ", Kind: KindEvent, DeckID: src}, http.StatusUnprocessableEntity},
		{"a made-up kind", fx.dir, CreateFromInput{Name: "X", Kind: "nope", DeckID: src}, http.StatusUnprocessableEntity},
		{"a name already used", fx.dir, CreateFromInput{Name: "Source", Kind: KindEvent, DeckID: src}, http.StatusConflict},
		{"a hero page that is not in the campaign", fx.dir, CreateFromInput{Name: "X", Kind: KindHero, HolderEntityID: "h-nobody", TableID: "loot"}, http.StatusUnprocessableEntity},
		{"a hero page on an events deck", fx.dir, CreateFromInput{Name: "X", Kind: KindEvent, HolderEntityID: "h-tam", DeckID: src}, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := len(fx.repo.decks)
			_, _, err := fx.svc.CreateFrom(nil2ctx(), camp, tt.v, tt.in)
			if code(err) != tt.want {
				t.Fatalf("%v (%d), want %d", err, code(err), tt.want)
			}
			if len(fx.repo.decks) != before {
				t.Error("a deck was made")
			}
		})
	}
}

// A table: weights become copies (rounded, at least 1, at most 20), a line with
// no name or no weight is skipped, a line with day conditions becomes a card
// without them and the page says so in the brief's words.
func TestFromATableMakesCardsFromLines(t *testing.T) {
	fx, _, tb, _ := fromFixture(t)
	tb.add("omens", "Omens",
		TableLine{Name: "Low", Brief: "A low one", Weight: 0.4},
		TableLine{Name: "High", Brief: "A high one", Weight: 2.6},
		TableLine{Name: "Never", Weight: 0},
		TableLine{Name: "   ", Brief: "no name", Weight: 3},
		TableLine{Name: "Plain", Weight: 1},
		TableLine{Name: "Huge", Weight: 100},
		TableLine{Name: "Full moon", Brief: "Only at night", Weight: 1, HasDays: true},
		TableLine{Name: "Harvest", Weight: 2, HasDays: true},
		TableLine{Name: "Midwinter", Weight: 1, HasDays: true},
	)
	got, rep, err := fx.from(CreateFromInput{Name: "Omens deck", Kind: KindEvent, TableID: "omens"})
	fx.must(err)
	cards := fx.cardsOf(got.ID)
	want := map[string]int{"Low": 1, "High": 3, "Plain": 1, "Huge": MaxCopiesPerCard, "Full moon": 1, "Harvest": 2, "Midwinter": 1}
	if len(cards) != len(want) {
		t.Fatalf("%d cards, want %d: %v", len(cards), len(want), cards)
	}
	for name, copies := range want {
		if c, ok := cards[name]; !ok || c.Copies != copies {
			t.Errorf("%s: %+v, want %d copies", name, c, copies)
		}
	}
	if cards["Low"].Words != "A low one" {
		t.Errorf("the brief is not the card's words: %q", cards["Low"].Words)
	}
	for name, c := range cards {
		if len(c.When) != 0 || c.WhenOpen {
			t.Errorf("%s stored a test: %+v", name, c.When)
		}
	}
	nu := fx.repo.decks[got.ID]
	if fmt.Sprint(nu.Look) != fmt.Sprint(defaultLook()) {
		t.Errorf("the deck is not in the Plain look: %+v", nu.Look)
	}
	if rep.Skipped != 2 || rep.DayLines != 3 || rep.NotMade != 0 || rep.Made != 7 {
		t.Errorf("report %+v", rep)
	}
	day := "3 lines only come up on certain days. Cards can't wait for a day yet, so those cards come up any time."
	found := false
	for _, n := range rep.Notes {
		found = found || n == day
	}
	if !found {
		t.Errorf("the day sentence is missing: %v", rep.Notes)
	}
	if fx.pileOf(got.ID) != 1+3+1+MaxCopiesPerCard+1+2+1 {
		t.Errorf("%d copies in the pile", fx.pileOf(got.ID))
	}
}

func TestFromATableDayConditionSentenceInTheSingular(t *testing.T) {
	fx, _, tb, _ := fromFixture(t)
	tb.add("t", "T", TableLine{Name: "A", Weight: 1, HasDays: true}, TableLine{Name: "B", Weight: 1})
	_, rep, err := fx.from(CreateFromInput{Name: "T deck", Kind: KindEvent, TableID: "t"})
	fx.must(err)
	if len(rep.Notes) != 1 || rep.Notes[0] != "1 line only comes up on certain days. Cards can't wait for a day yet, so that card comes up any time." {
		t.Errorf("%v", rep.Notes)
	}
}

// More lines than a deck can hold: the first up to the limit are made and the
// rest are counted.
func TestFromATableOverTheLimitSaysHowManyWereLeft(t *testing.T) {
	tests := []struct {
		name      string
		lines     int
		weight    float64
		wantCards int
		wantLeft  int
	}{
		{"more lines than cards a deck may hold", 250, 1, MaxCardsPerDeck, 50},
		{"more copies than a deck may hold", 40, 20, MaxCopiesPerDeck / 20, 40 - MaxCopiesPerDeck/20},
		{"exactly the card limit", MaxCardsPerDeck, 1, MaxCardsPerDeck, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx, _, tb, _ := fromFixture(t)
			var lines []TableLine
			for i := 0; i < tt.lines; i++ {
				lines = append(lines, TableLine{Name: fmt.Sprintf("Line %03d", i), Weight: tt.weight})
			}
			tb.add("big", "Big", lines...)
			got, rep, err := fx.from(CreateFromInput{Name: "Big deck", Kind: KindEvent, TableID: "big"})
			fx.must(err)
			if n := len(fx.cardsOf(got.ID)); n != tt.wantCards {
				t.Errorf("%d cards, want %d", n, tt.wantCards)
			}
			if rep.NotMade != tt.wantLeft {
				t.Errorf("%d left, want %d", rep.NotMade, tt.wantLeft)
			}
			if _, ok := fx.cardsOf(got.ID)["Line 000"]; !ok {
				t.Error("the first line was not taken")
			}
			if (tt.wantLeft > 0) != (len(rep.Notes) > 0) {
				t.Errorf("notes: %v", rep.Notes)
			}
			if fx.pileOf(got.ID) > MaxCopiesPerDeck {
				t.Errorf("%d copies pass the limit", fx.pileOf(got.ID))
			}
		})
	}
}

// Lines the cards table would call one name make one card whose copies add
// up: the table compares names without regard to case or accents, so a second
// card would be refused and the whole deck with it.
func TestFromATableMergesLinesThatShareAName(t *testing.T) {
	tests := []struct {
		name   string
		second string
	}{
		{"another case", "wolf"},
		{"an accent", "Wölf"},
		{"an accent written as two marks", "Wo\u0308lf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx, _, tb, _ := fromFixture(t)
			tb.add("t", "T", TableLine{Name: "Wolf", Weight: 2}, TableLine{Name: tt.second, Weight: 3}, TableLine{Name: "Bear", Weight: 1})
			got, rep, err := fx.from(CreateFromInput{Name: "T deck", Kind: KindEvent, TableID: "t"})
			fx.must(err)
			cards := fx.cardsOf(got.ID)
			if len(cards) != 2 || cards["Wolf"].Copies != 5 || rep.Merged != 1 || rep.LinesUsed != 3 || len(rep.Notes) != 1 {
				t.Errorf("cards %v, report %+v", cards, rep)
			}
		})
	}
}

// The sentence for a full deck counts lines, not cards: lines of one name
// became one card, and saying the smaller number would leave lines unaccounted
// for.
func TestTheFullDeckSentenceCountsLines(t *testing.T) {
	got := FromReport{Made: 2, LinesUsed: 3, NotMade: 4}.sentences()
	want := "A deck can't hold the whole table. The first 3 lines were made into cards and the last 4 lines were left out."
	if len(got) != 1 || got[0] != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNameKeyFoldsWhatTheCardsTableFolds(t *testing.T) {
	tests := []struct{ a, b string }{
		{"Cafe", "CAFÉ"},
		{"Straße", "strasse"},
		{"Ash on the wind", "ash on the wind"},
	}
	for _, tt := range tests {
		if nameKey(tt.a) != nameKey(tt.b) {
			t.Errorf("%q and %q are one name to the cards table and two here", tt.a, tt.b)
		}
	}
	if nameKey("Wolf") == nameKey("Wolves") {
		t.Error("two different names share a key")
	}
}

func TestFromATableRefusals(t *testing.T) {
	fx, _, tb, _ := fromFixture(t)
	tb.add("empty", "Empty", TableLine{Name: "", Weight: 1}, TableLine{Name: "Zero", Weight: 0})
	tests := []struct {
		name  string
		table string
		want  int
	}{
		{"a table that is not there", "nope", http.StatusNotFound},
		{"an id that cannot be a table", "../Loot", http.StatusNotFound},
		{"a table with nothing usable", "empty", http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		_, _, err := fx.from(CreateFromInput{Name: "X", Kind: KindEvent, TableID: tt.table})
		if code(err) != tt.want {
			t.Errorf("%s: %v (%d), want %d", tt.name, err, code(err), tt.want)
		}
	}
	if len(fx.repo.decks) != 0 {
		t.Error("a deck was made")
	}
}

// With no reader of rolling tables bound the page says there are none and a
// create names the reason, whether or not the id is real.
func TestWithoutRollingTablesTheOptionSaysSoAndACreateIsRefused(t *testing.T) {
	fx := newFixture(t)
	ch, err := fx.svc.RollingTables(nil2ctx(), camp, fx.dir)
	fx.must(err)
	if ch.Available || len(ch.Tables) != 0 {
		t.Errorf("%+v", ch)
	}
	_, _, err = fx.from(CreateFromInput{Name: "X", Kind: KindEvent, TableID: "loot"})
	if code(err) != http.StatusUnprocessableEntity || !strings.Contains(err.Error(), "Rolling tables are not available") {
		t.Errorf("%v (%d)", err, code(err))
	}
	if _, err := fx.svc.RollingTables(nil2ctx(), camp, fx.tam); err == nil {
		t.Error("a player listed the tables")
	}
}

// A reader that fails reads as none to offer, not as a broken page.
func TestARollingTablesReaderThatFailsReadsAsNone(t *testing.T) {
	fx, _, tb, _ := fromFixture(t)
	tb.add("t", "T", TableLine{Name: "A", Weight: 1})
	ch, err := fx.svc.RollingTables(nil2ctx(), camp, fx.dir)
	fx.must(err)
	if !ch.Available || len(ch.Tables) != 1 || ch.Tables[0].Lines != 1 {
		t.Errorf("%+v", ch)
	}
	tb.failed = errors.New("down")
	ch, err = fx.svc.RollingTables(nil2ctx(), camp, fx.dir)
	if err != nil || ch.Available {
		t.Errorf("%+v, %v", ch, err)
	}
	if _, _, err := fx.from(CreateFromInput{Name: "X", Kind: KindEvent, TableID: "t"}); code(err) != http.StatusInternalServerError {
		t.Errorf("a failing read: %v (%d)", err, code(err))
	}
}
