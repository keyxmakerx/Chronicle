package decks

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCompareHasATolerance(t *testing.T) {
	tests := []struct {
		op   string
		a, b float64
		want bool
	}{
		{"lte", 3, 3, true}, {"lte", 3.0000000001, 3, true}, {"lt", 3, 3, false}, {"lt", 2, 3, true},
		{"gte", 3, 3, true}, {"gt", 3, 3, false}, {"gt", 4, 3, true}, {"eq", 10, 30.0 / 3, true}, {"eq", 1, 2, false},
		{"huh", 1, 1, false},
	}
	for _, tt := range tests {
		if got := compare(tt.op, tt.a, tt.b); got != tt.want {
			t.Errorf("%v %s %v = %v", tt.a, tt.op, tt.b, got)
		}
	}
}

// judgeOf builds a judge over the facts for a deck.
func judgeOf(t *testing.T, f *fakeFacts, d Deck) *judge {
	t.Helper()
	fx := newFixture(t)
	newFactsFixture(t, fx, f)
	return fx.svc.(*service).judgeFor(bg, camp, d)
}

func TestJudgeTests(t *testing.T) {
	f := newFakeFacts()
	f.sheet("h-tam", map[string]float64{"hp": 5, "hpmax": 20, "stamina": 30})
	f.sheet("h-ilse", map[string]float64{"hp": 18, "hpmax": 20})
	f.trks["h-tam"] = []TrackerFact{{KindID: "k-doom", Name: "Doom", Value: 3}}
	f.tags["h-tam"] = []TagFact{{ID: 7, Name: "Cursed"}}
	f.lays["h-ilse"] = []LayerFact{{Name: "Revenant", Source: "drawsteel:revenant"}}
	hero := Deck{Kind: KindHero, HolderEntityID: "h-tam"}
	j := judgeOf(t, f, hero)

	tests := []struct {
		name string
		test WhenTest
		want bool
	}{
		{"number below a value", WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Value: fl(5)}, true},
		{"number above a value fails", WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "gt", Value: fl(5)}, false},
		{"a quarter of max", WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Of: "hpmax", Share: "quarter"}, true},
		{"a quarter of max is exact", WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "eq", Of: "hpmax", Share: "quarter"}, true},
		{"half of max fails for Ilse", WhenTest{Test: TestNumber, Who: "h-ilse", Field: "hp", Op: "lte", Of: "hpmax", Share: "half"}, false},
		{"holder is the deck's hero", WhenTest{Test: TestNumber, Who: WhoHolder, Field: "stamina", Op: "gte", Value: fl(30)}, true},
		{"a field with no value is missing", WhenTest{Test: TestNumber, Who: "h-ilse", Field: "stamina", Op: "gte", Value: fl(0)}, false},
		{"a page with no sheet never holds", WhenTest{Test: TestNumber, Who: "h-npc", Field: "hp", Op: "gte", Value: fl(0)}, false},
		{"half of another field", WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Of: "stamina", Share: "half"}, true},
		{"a share of a field that is missing cannot be compared", WhenTest{Test: TestNumber, Who: "h-ilse", Field: "hp", Op: "lte", Of: "stamina", Share: "half"}, false},
		{"tracker at or above", WhenTest{Test: TestTracker, Who: "h-tam", Kind: "k-doom", Op: "gte", Value: fl(3)}, true},
		{"tracker below fails", WhenTest{Test: TestTracker, Who: "h-tam", Kind: "k-doom", Op: "lt", Value: fl(3)}, false},
		{"a tracker not on the page does not hold", WhenTest{Test: TestTracker, Who: "h-ilse", Kind: "k-doom", Op: "gte", Value: fl(0)}, false},
		{"has a tag", WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(true)}, true},
		{"has not a tag", WhenTest{Test: TestTag, Who: "h-ilse", Tag: it(7), Has: bl(false)}, true},
		{"has the tag, wanted not", WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(false)}, false},
		{"layer by name, any case", WhenTest{Test: TestLayer, Who: "h-ilse", Name: "revenant", On: bl(true)}, true},
		{"layer by source", WhenTest{Test: TestLayer, Who: "h-ilse", Name: "drawsteel:revenant", On: bl(true)}, true},
		{"layer off", WhenTest{Test: TestLayer, Who: "h-tam", Name: "Revenant", On: bl(false)}, true},
		{"layer wanted but absent", WhenTest{Test: TestLayer, Who: "h-tam", Name: "Revenant", On: bl(true)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := j.fits(Card{When: []WhenTest{tt.test}})
			if got.OK != tt.want {
				t.Errorf("fits = %v (%s), want %v", got.OK, got.Why, tt.want)
			}
		})
	}
}

func TestJudgeSayAndNoTests(t *testing.T) {
	j := judgeOf(t, newFakeFacts(), Deck{})
	if !j.fits(Card{}).OK {
		t.Error("a card with no test must fit")
	}
	if j.fits(Card{When: []WhenTest{{Test: TestSay}}}).OK {
		t.Error("an unsaid card fits")
	}
	now := time.Now()
	if !j.fits(Card{When: []WhenTest{{Test: TestSay}}, SaidAt: &now}).OK {
		t.Error("a said card does not fit")
	}
}

func TestEveryTestMustHold(t *testing.T) {
	f := newFakeFacts()
	f.sheet("h-tam", map[string]float64{"hp": 5})
	f.tags["h-tam"] = []TagFact{{ID: 7}}
	j := judgeOf(t, f, Deck{})
	hp := WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Value: fl(5)}
	tag := WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(true)}
	if !j.fits(Card{When: []WhenTest{hp, tag}}).OK {
		t.Error("two holding tests must fit")
	}
	bad := WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(false)}
	got := j.fits(Card{When: []WhenTest{hp, bad}})
	if got.OK || !strings.Contains(got.Why, "has the tag") {
		t.Errorf("one failing test must fail the card: %+v", got)
	}
	if !strings.Contains(got.Reading.Text, "5") {
		t.Errorf("the reading is the first number test: %+v", got.Reading)
	}
}

// Every "any" test of one card has to hold for the same hero, and the first by
// name is the subject.
func TestAnyTestsHoldForTheSameHero(t *testing.T) {
	f := newFakeFacts()
	f.sheet("h-tam", map[string]float64{"hp": 2})
	f.sheet("h-ilse", map[string]float64{"hp": 18})
	f.tags["h-ilse"] = []TagFact{{ID: 7}}
	j := judgeOf(t, f, Deck{})
	low := WhenTest{Test: TestNumber, Who: WhoAny, Field: "hp", Op: "lte", Value: fl(3)}
	cursed := WhenTest{Test: TestTag, Who: WhoAny, Tag: it(7), Has: bl(true)}
	if got := j.fits(Card{When: []WhenTest{low}}); !got.OK || got.Subject != "h-tam" {
		t.Errorf("low alone: %+v", got)
	}
	if got := j.fits(Card{When: []WhenTest{cursed}}); !got.OK || got.Subject != "h-ilse" {
		t.Errorf("cursed alone: %+v", got)
	}
	// One hero is low and another is cursed; no hero is both.
	if got := j.fits(Card{When: []WhenTest{low, cursed}}); got.OK {
		t.Errorf("two different heroes satisfied one card: %+v", got)
	}
	f.tags["h-tam"] = []TagFact{{ID: 7}}
	j = judgeOf(t, f, Deck{})
	if got := j.fits(Card{When: []WhenTest{low, cursed}}); !got.OK || got.Subject != "h-tam" {
		t.Errorf("one hero is both: %+v", got)
	}
}

func TestAnyWithNoPartyDoesNotHold(t *testing.T) {
	fx := newFixture(t)
	fx.heroes.pages = map[string]Hero{}
	f := newFakeFacts()
	newFactsFixture(t, fx, f)
	j := fx.svc.(*service).judgeFor(bg, camp, Deck{})
	if j.fits(Card{When: []WhenTest{{Test: TestTag, Who: WhoAny, Tag: it(7), Has: bl(false)}}}).OK {
		t.Error("a test about any hero held with no hero")
	}
}

// A fact that cannot be read is a missing fact: a test on it does not hold, not
// even a test that would hold of an empty page.
func TestAFailingSourceNeverMakesACardFit(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name string
		key  string
		err  error
		test WhenTest
	}{
		{"numbers", "numbers:h-tam", boom, WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "gte", Value: fl(0)}},
		{"trackers", "trackers:h-tam", boom, WhenTest{Test: TestTracker, Who: "h-tam", Kind: "k-doom", Op: "lte", Value: fl(99)}},
		{"tags", "tags:h-tam", boom, WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(false)}},
		{"layers", "layers:h-tam", boom, WhenTest{Test: TestLayer, Who: "h-tam", Name: "X", On: bl(false)}},
		{"unavailable numbers", "numbers:h-tam", ErrUnavailable, WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "gte", Value: fl(0)}},
		{"unavailable tags", "tags:h-tam", ErrUnavailable, WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeFacts()
			f.sheet("h-tam", map[string]float64{"hp": 1})
			f.fail(tc.key, tc.err)
			j := judgeOf(t, f, Deck{})
			if got := j.fits(Card{When: []WhenTest{tc.test}}); got.OK {
				t.Errorf("a test on a source that failed held: %+v", got)
			}
		})
	}
	// With no facts wired at all, nothing but "say" can hold.
	fx := newFixture(t)
	j := fx.svc.(*service).judgeFor(bg, camp, Deck{})
	if j.fits(Card{When: []WhenTest{{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(false)}}}).OK {
		t.Error("a tag test held with no facts wired")
	}
}

func TestEachFactIsReadOncePerRun(t *testing.T) {
	f := newFakeFacts()
	f.sheet("h-tam", map[string]float64{"hp": 1})
	j := judgeOf(t, f, Deck{})
	c := Card{When: []WhenTest{{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Value: fl(5)}}}
	for i := 0; i < 10; i++ {
		j.fits(c)
	}
	if f.reads != 1 {
		t.Errorf("the page's numbers were read %d times", f.reads)
	}
}

func TestFittingSetsByWhoAsks(t *testing.T) {
	fx := newFixture(t)
	f := newFakeFacts()
	f.tags["h-tam"] = []TagFact{{ID: 7}}
	newFactsFixture(t, fx, f)
	s := fx.svc.(*service)
	holds := WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(true)}
	fails := WhenTest{Test: TestTag, Who: "h-tam", Tag: it(7), Has: bl(false)}
	holdsOpen, failsOpen := holds, fails
	holdsOpen.Open, failsOpen.Open = true, true
	cards := []Card{
		{ID: "plain"},
		{ID: "hidden-holds", When: []WhenTest{holds}},
		{ID: "hidden-fails", When: []WhenTest{fails}},
		{ID: "open-holds", When: []WhenTest{holdsOpen}},
		{ID: "open-fails", When: []WhenTest{failsOpen}},
		{ID: "mixed", When: []WhenTest{holdsOpen, holds}},
		// Printed, and it holds, but about a page a player cannot open.
		{ID: "open-vault", When: []WhenTest{{Test: TestTag, Who: "h-vault", Tag: it(7), Has: bl(false), Open: true}}},
	}
	dir := s.fittingCards(bg, camp, Deck{}, cards)
	for id, want := range map[string]bool{"plain": true, "hidden-holds": true, "hidden-fails": false, "open-holds": true, "open-fails": false, "mixed": true, "open-vault": true} {
		if dir[id] != want {
			t.Errorf("Director set: %s = %v, want %v", id, dir[id], want)
		}
	}
	// A player's own draw never reaches a card with a test the Director kept.
	// Nor one whose printed test is about a page they cannot open.
	pl := s.eligibleForPlayer(bg, camp, Deck{}, cards, fx.tam)
	for id, want := range map[string]bool{"plain": true, "hidden-holds": false, "hidden-fails": false, "open-holds": true, "open-fails": false, "mixed": false, "open-vault": false} {
		if pl[id] != want {
			t.Errorf("player set: %s = %v, want %v", id, pl[id], want)
		}
	}
	if s.fittingCards(bg, camp, Deck{}, []Card{{ID: "a"}}) != nil || s.eligibleForPlayer(bg, camp, Deck{}, []Card{{ID: "a"}}, fx.tam) != nil {
		t.Error("a deck with no tests must not be restricted")
	}
	withTests := s.fittingWithTests(bg, camp, Deck{}, cards)
	if _, ok := withTests["plain"]; ok || len(withTests) != 4 {
		t.Errorf("fittingWithTests = %v", withTests)
	}
}

// What a player's draw reads must not even touch a hidden test's facts.
func TestAPlayersEligibilityNeverReadsAHiddenTestsFacts(t *testing.T) {
	fx := newFixture(t)
	f := newFakeFacts()
	newFactsFixture(t, fx, f)
	s := fx.svc.(*service)
	hidden := WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "gte", Value: fl(0)}
	s.eligibleForPlayer(bg, camp, Deck{}, []Card{{ID: "h", When: []WhenTest{hidden}}}, fx.tam)
	if f.reads != 0 {
		t.Errorf("a hidden test's facts were read %d times for a player", f.reads)
	}
}
