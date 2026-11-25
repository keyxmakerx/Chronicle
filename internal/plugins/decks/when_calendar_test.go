package decks

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// calendarFacts is the usual campaign with a main calendar: Nyx, a cycling
// moon on day 15 and full; Mote, a moon set by hand; a Bloodnight today; and
// night.
func calendarFacts() *fakeFacts {
	f := newFakeFacts()
	f.opts.Calendar = true
	f.opts.Moons = []string{"Nyx", "Mote", "Veil"}
	f.opts.EventKinds = []EventKindOption{{Slug: "holiday", Name: "Holiday"}}
	f.cal = &TodayFact{
		Moons: map[string]MoonFact{
			"nyx":  {Name: "Nyx", Day: 15, Words: []string{"Full Moon", "full"}},
			"mote": {Name: "Mote", Day: 0, Words: []string{"Blood Red"}},
			"veil": {Name: "Veil", Day: 3, Words: []string{"Waxing Crescent", "waxing", "crescent"}},
		},
		Events:  map[string]bool{"bloodnight": true, "holiday": true},
		DayPart: "night",
	}
	return f
}

func TestMoonMatches(t *testing.T) {
	cycling := MoonFact{Day: 24, Words: []string{"Waning Crescent", "waning", "crescent"}}
	byHand := MoonFact{Day: 0, Words: []string{"Blood Red"}}
	tests := []struct {
		name     string
		m        MoonFact
		phase    string
		from, to *int
		want     bool
	}{
		{"a phase word", cycling, "waning", nil, nil, true},
		{"a phase word in any case", cycling, " Crescent ", nil, nil, true},
		{"the phase's own name", cycling, "waning crescent", nil, nil, true},
		{"another phase", cycling, "full", nil, nil, false},
		{"a range holding the day", cycling, "", it(20), it(25), true},
		{"a range ending on the day", cycling, "", it(20), it(24), true},
		{"a range before the day", cycling, "", it(1), it(23), false},
		{"a range that wraps", cycling, "", it(23), it(1), true},
		{"a range that wraps past the day", MoonFact{Day: 10}, "", it(23), it(1), false},
		{"a range that wraps, early side", MoonFact{Day: 1}, "", it(23), it(1), true},
		{"a hand-set moon by its state", byHand, "blood red", nil, nil, true},
		{"a hand-set moon has no days", byHand, "", it(1), it(30), false},
		{"neither phase nor days", cycling, "", nil, nil, false},
	}
	for _, tt := range tests {
		if got := moonMatches(tt.m, tt.phase, tt.from, tt.to); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseWhenReadsTheCalendarTests(t *testing.T) {
	tests := []struct {
		name, raw, wantErr string
	}{
		{name: "a moon in a phase", raw: `[{"test":"moon","name":"Nyx","phase":"full","is":true}]`},
		{name: "a moon on days", raw: `[{"test":"moon","name":"Nyx","from":23,"to":1,"is":false,"open":true}]`},
		{name: "an event today", raw: `[{"test":"event","name":"Bloodnight","has":true}]`},
		{name: "the time of day", raw: `[{"test":"time","time":"night","is":true}]`},
		{name: "a moon names no page", raw: `[{"test":"moon","who":"any","name":"Nyx","phase":"full","is":true}]`, wantErr: `unknown key "who"`},
		{name: "an event has no is", raw: `[{"test":"event","name":"Bloodnight","is":true}]`, wantErr: `unknown key "is"`},
		{name: "a time has no name", raw: `[{"test":"time","name":"night","is":true}]`, wantErr: `unknown key "name"`},
		{name: "days are whole numbers", raw: `[{"test":"moon","name":"Nyx","from":"one","to":3,"is":true}]`, wantErr: `"from" has the wrong kind`},
	}
	for _, tt := range tests {
		got, err := ParseWhen([]byte(tt.raw))
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: err = %v, want %q", tt.name, err, tt.wantErr)
			}
			continue
		}
		if err != nil || len(got) != 1 {
			t.Errorf("%s: %+v, %v", tt.name, got, err)
		}
	}
}

func TestValidateCalendarTests(t *testing.T) {
	tests := []struct {
		name    string
		in      WhenTest
		want    WhenTest
		wantErr string
	}{
		{name: "a moon in a phase, named in any case", in: WhenTest{Test: TestMoon, Name: " nyx ", Phase: "Full", Is: bl(true)},
			want: WhenTest{Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(true)}},
		{name: "a moon in a phase's own name", in: WhenTest{Test: TestMoon, Name: "Mote", Phase: " Blood Red ", Is: bl(false)},
			want: WhenTest{Test: TestMoon, Name: "Mote", Phase: "Blood Red", Is: bl(false)}},
		{name: "a moon on days that wrap", in: WhenTest{Test: TestMoon, Name: "Nyx", From: it(23), To: it(1), Is: bl(true)},
			want: WhenTest{Test: TestMoon, Name: "Nyx", From: it(23), To: it(1), Is: bl(true)}},
		{name: "a moon the calendar does not have", in: WhenTest{Test: TestMoon, Name: "Luna", Phase: "full", Is: bl(true)}, wantErr: "no moon by that name"},
		{name: "a moon needs is", in: WhenTest{Test: TestMoon, Name: "Nyx", Phase: "full"}, wantErr: `needs "name", "is"`},
		{name: "a moon needs a name", in: WhenTest{Test: TestMoon, Phase: "full", Is: bl(true)}, wantErr: "the moon's name"},
		{name: "a moon not both phase and days", in: WhenTest{Test: TestMoon, Name: "Nyx", Phase: "full", From: it(1), To: it(2), Is: bl(true)}, wantErr: "not both"},
		{name: "a moon not neither", in: WhenTest{Test: TestMoon, Name: "Nyx", Is: bl(true)}, wantErr: "not both and not neither"},
		{name: "a moon needs both days", in: WhenTest{Test: TestMoon, Name: "Nyx", From: it(3), Is: bl(true)}, wantErr: "days of the moon's cycle"},
		{name: "a day of zero", in: WhenTest{Test: TestMoon, Name: "Nyx", From: it(0), To: it(3), Is: bl(true)}, wantErr: "days of the moon's cycle"},
		{name: "a moon names no page", in: WhenTest{Test: TestMoon, Who: WhoAny, Name: "Nyx", Phase: "full", Is: bl(true)}, wantErr: `takes no "who"`},
		{name: "an event", in: WhenTest{Test: TestEvent, Name: " Bloodnight ", Has: bl(false)},
			want: WhenTest{Test: TestEvent, Name: "Bloodnight", Has: bl(false)}},
		{name: "an event not on the calendar yet", in: WhenTest{Test: TestEvent, Name: "The Eclipse", Has: bl(true)},
			want: WhenTest{Test: TestEvent, Name: "The Eclipse", Has: bl(true)}},
		{name: "an event needs has", in: WhenTest{Test: TestEvent, Name: "Bloodnight"}, wantErr: `needs "name" and "has"`},
		{name: "an event needs a name", in: WhenTest{Test: TestEvent, Name: "  ", Has: bl(true)}, wantErr: "kind of event"},
		{name: "a long event name", in: WhenTest{Test: TestEvent, Name: strings.Repeat("e", 81), Has: bl(true)}, wantErr: "too long"},
		{name: "a time of day", in: WhenTest{Test: TestTime, Time: "Dusk", Is: bl(true)}, want: WhenTest{Test: TestTime, Time: "dusk", Is: bl(true)}},
		{name: "an unknown time of day", in: WhenTest{Test: TestTime, Time: "noon", Is: bl(true)}, wantErr: "dawn, day, dusk or night"},
		{name: "a time needs is", in: WhenTest{Test: TestTime, Time: "day"}, wantErr: "dawn, day, dusk or night"},
		{name: "say takes no moon key", in: WhenTest{Test: TestSay, Is: bl(true)}, wantErr: `"say" takes no other keys`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			newFactsFixture(t, fx, calendarFacts())
			got, err := fx.svc.ValidateWhen(bg, camp, Deck{Kind: KindEvent}, []WhenTest{tt.in})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code(err) != http.StatusUnprocessableEntity {
					t.Fatalf("err = %v (%d), want one containing %q", err, code(err), tt.wantErr)
				}
				return
			}
			if err != nil || len(got) != 1 {
				t.Fatalf("got %+v, %v", got, err)
			}
			if encodeWhen(got) != encodeWhen([]WhenTest{tt.want}) {
				t.Errorf("stored %s, want %s", encodeWhen(got), encodeWhen([]WhenTest{tt.want}))
			}
		})
	}
}

// A moon renamed on the calendar breaks the test that names it; a calendar
// that cannot be read breaks nothing, since nothing is known to be gone.
func TestBrokenMoonTests(t *testing.T) {
	fx := newFixture(t)
	f := calendarFacts()
	newFactsFixture(t, fx, f)
	tests := []WhenTest{{Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(true)}, {Test: TestTime, Time: "night", Is: bl(true)}}
	f.opts.Moons = []string{"Nyx the Pale"}
	if got := fx.svc.(*service).BrokenTests(bg, camp, Deck{}, tests); !got[0] || got[1] {
		t.Errorf("a renamed moon: broken = %v", got)
	}
	f.opts.Calendar, f.opts.Moons = false, nil
	if got := fx.svc.(*service).BrokenTests(bg, camp, Deck{}, tests); got[0] || got[1] {
		t.Errorf("no calendar: broken = %v", got)
	}
}

func TestJudgeCalendarTests(t *testing.T) {
	tests := []struct {
		name string
		test WhenTest
		want bool
	}{
		{"a full moon", WhenTest{Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(true)}, true},
		{"not new", WhenTest{Test: TestMoon, Name: "NYX", Phase: "new", Is: bl(false)}, true},
		{"full, wanted not", WhenTest{Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(false)}, false},
		{"on its days", WhenTest{Test: TestMoon, Name: "Nyx", From: it(14), To: it(16), Is: bl(true)}, true},
		{"off its days", WhenTest{Test: TestMoon, Name: "Nyx", From: it(1), To: it(7), Is: bl(true)}, false},
		{"a hand-set moon's state", WhenTest{Test: TestMoon, Name: "Mote", Phase: "Blood Red", Is: bl(true)}, true},
		{"a moon not on the calendar is missing", WhenTest{Test: TestMoon, Name: "Luna", Phase: "full", Is: bl(true)}, false},
		{"a missing moon does not hold its not form either", WhenTest{Test: TestMoon, Name: "Luna", Phase: "full", Is: bl(false)}, false},
		{"an event today", WhenTest{Test: TestEvent, Name: "bloodnight", Has: bl(true)}, true},
		{"a kind of event today", WhenTest{Test: TestEvent, Name: "Holiday", Has: bl(true)}, true},
		{"an event not today", WhenTest{Test: TestEvent, Name: "Harvest", Has: bl(true)}, false},
		{"no Harvest today", WhenTest{Test: TestEvent, Name: "Harvest", Has: bl(false)}, true},
		{"night", WhenTest{Test: TestTime, Time: "night", Is: bl(true)}, true},
		{"not day", WhenTest{Test: TestTime, Time: "day", Is: bl(false)}, true},
		{"dawn", WhenTest{Test: TestTime, Time: "dawn", Is: bl(true)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := judgeOf(t, calendarFacts(), Deck{Kind: KindEvent})
			if got := j.fits(Card{When: []WhenTest{tt.test}}); got.OK != tt.want {
				t.Errorf("fits = %v (%s), want %v", got.OK, got.Why, tt.want)
			}
		})
	}
}

// A missing fact is a test that does not hold, in its plain and its not form:
// no calendar, a calendar plugin that fails, events that can't be read and a
// time of day the calendar can't say.
func TestCalendarTestsOnMissingFactsDoNotHold(t *testing.T) {
	all := []WhenTest{
		{Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(true)}, {Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(false)},
		{Test: TestEvent, Name: "Bloodnight", Has: bl(true)}, {Test: TestEvent, Name: "Bloodnight", Has: bl(false)},
		{Test: TestTime, Time: "night", Is: bl(true)}, {Test: TestTime, Time: "night", Is: bl(false)},
	}
	worlds := map[string]func(f *fakeFacts){
		"no calendar":                  func(f *fakeFacts) { f.cal = nil },
		"the calendar fails":           func(f *fakeFacts) { f.errs["today"] = errors.New("calendar down") },
		"no events, no time, no moons": func(f *fakeFacts) { f.cal = &TodayFact{} },
	}
	for name, set := range worlds {
		for _, test := range all {
			f := calendarFacts()
			set(f)
			j := judgeOf(t, f, Deck{Kind: KindEvent})
			if got := j.fits(Card{When: []WhenTest{test}}); got.OK || got.Why == "" {
				t.Errorf("%s: %s held (%q)", name, encodeWhen([]WhenTest{test}), got.Why)
			}
		}
	}
}

func TestCalendarSentences(t *testing.T) {
	fx := newFixture(t)
	f := calendarFacts()
	// Veil is hidden from players: their calendar does not show it.
	f.playerCal = &TodayFact{Moons: map[string]MoonFact{"nyx": f.cal.Moons["nyx"]}, Events: map[string]bool{}, DayPart: "night"}
	newFactsFixture(t, fx, f)
	s := fx.svc.(*service)
	dir := s.newNamer(bg, camp, Deck{}, fx.dir)
	tests := []struct {
		in   WhenTest
		want string
	}{
		{WhenTest{Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(true)}, "Nyx is full"},
		{WhenTest{Test: TestMoon, Name: "Nyx", Phase: "first-quarter", Is: bl(false)}, "Nyx is not at its first quarter"},
		{WhenTest{Test: TestMoon, Name: "Mote", Phase: "Blood Red", Is: bl(true)}, "Mote is Blood Red"},
		{WhenTest{Test: TestMoon, Name: "Nyx", From: it(3), To: it(5), Is: bl(true)}, "Nyx is on day 3 to 5 of its cycle"},
		{WhenTest{Test: TestMoon, Name: "Nyx", From: it(7), To: it(7), Is: bl(false)}, "Nyx is not on day 7 of its cycle"},
		{WhenTest{Test: TestEvent, Name: "Bloodnight", Has: bl(true)}, "Bloodnight is today"},
		{WhenTest{Test: TestEvent, Name: "Bloodnight", Has: bl(false)}, "Bloodnight is not today"},
		{WhenTest{Test: TestTime, Time: "night", Is: bl(true)}, "it is night"},
		{WhenTest{Test: TestTime, Time: "dawn", Is: bl(false)}, "it is not dawn"},
		{WhenTest{Test: TestMoon, Name: "Veil", Phase: "crescent", Is: bl(true)}, "Veil is a crescent"},
	}
	for _, tt := range tests {
		if got := dir.sentence(tt.in); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.in, got, tt.want)
		}
	}
	// A player reads a moon the calendar hides from them as "a moon", as a
	// hidden page reads "a page".
	hidden := []WhenTest{{Test: TestMoon, Name: "Veil", Phase: "crescent", Is: bl(true), Open: true}, {Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(true), Open: true}}
	if got := s.newNamer(bg, camp, Deck{}, fx.tam).printedLine(hidden); got != "When a moon is a crescent and Nyx is full." {
		t.Errorf("the player reads %q", got)
	}
	if got := s.newNamer(bg, camp, Deck{}, fx.dir).printedLine(hidden); !strings.Contains(got, "Veil") {
		t.Errorf("the Director reads %q", got)
	}
}

// A player's own draw judges the calendar as that player sees it: a printed
// test on a moon hidden from them keeps the card out whatever the moon does,
// and a hidden event is not today for them. The Director's draw reads it all.
func TestAPlayersDrawJudgesTheCalendarAsThePlayerSeesIt(t *testing.T) {
	fx := newFixture(t)
	f := calendarFacts()
	f.playerCal = &TodayFact{Moons: map[string]MoonFact{"nyx": f.cal.Moons["nyx"]}, Events: map[string]bool{}, DayPart: "night"}
	newFactsFixture(t, fx, f)
	s := fx.svc.(*service)
	deck := fx.deck("Omens", KindEvent, &SettingsInput{Draws: ptr("players")})
	open := func(t WhenTest) WhenList { t.Open = true; return WhenList{t} }
	veil := fx.card(deck, CreateCardInput{Name: "Veil", When: open(WhenTest{Test: TestMoon, Name: "Veil", Phase: "crescent", Is: bl(true)})})
	notVeil := fx.card(deck, CreateCardInput{Name: "NotVeil", When: open(WhenTest{Test: TestMoon, Name: "Veil", Phase: "full", Is: bl(false)})})
	nyx := fx.card(deck, CreateCardInput{Name: "Nyx", When: open(WhenTest{Test: TestMoon, Name: "Nyx", Phase: "full", Is: bl(true)})})
	quiet := fx.card(deck, CreateCardInput{Name: "Quiet", When: open(WhenTest{Test: TestEvent, Name: "Bloodnight", Has: bl(false)})})
	cards, err := fx.repo.ListCards(bg, camp, deck)
	fx.must(err)
	d := Deck{ID: deck, CampaignID: camp, Kind: KindEvent}

	dir := s.fittingCards(bg, camp, d, cards)
	if !dir[veil] || !dir[notVeil] || !dir[nyx] || dir[quiet] {
		t.Errorf("the Director's draw: %v", dir)
	}
	f.todayAs = nil
	pl := s.eligibleForPlayer(bg, camp, d, cards, fx.tam)
	if pl[veil] || pl[notVeil] || !pl[nyx] || !pl[quiet] {
		t.Errorf("the player's draw: %v", pl)
	}
	for _, v := range f.todayAs {
		if v.Director || v.UserID != "u-tam" {
			t.Errorf("the player's draw read the calendar as %+v", v)
		}
	}
}

func TestWhenOptionsOfferTheCalendar(t *testing.T) {
	fx := newFixture(t)
	newFactsFixture(t, fx, calendarFacts())
	o, err := fx.svc.WhenOptions(bg, camp, fx.dir)
	fx.must(err)
	if !o.Calendar || len(o.Moons) != 3 || len(o.EventKinds) != 1 || o.EventKinds[0].Slug != "holiday" {
		t.Errorf("options: calendar %v, moons %v, kinds %v", o.Calendar, o.Moons, o.EventKinds)
	}
	if len(o.DayParts) != 4 || len(o.MoonPhases) != 8 {
		t.Errorf("words: %v %v", o.DayParts, o.MoonPhases)
	}
	kinds := map[string]bool{}
	for _, k := range o.TestKinds {
		kinds[k.Key] = true
	}
	for _, k := range []string{TestMoon, TestEvent, TestTime} {
		if !kinds[k] {
			t.Errorf("the add menu has no %q", k)
		}
	}
	bare := newFixture(t)
	o, err = bare.svc.WhenOptions(bg, camp, bare.dir)
	bare.must(err)
	if o.Moons == nil || o.EventKinds == nil || o.Calendar {
		t.Errorf("without facts the lists must be empty, not null: %+v", o.WhenOptions)
	}
}
