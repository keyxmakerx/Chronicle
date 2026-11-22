package decks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func fl(v float64) *float64 { return &v }
func bl(v bool) *bool       { return &v }
func it(v int) *int         { return &v }

func TestParseWhenIsStrict(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr string
	}{
		{name: "empty string is none", raw: `""`},
		{name: "null is none", raw: `null`},
		{name: "nothing is none", raw: ``},
		{name: "empty list is none", raw: `[]`},
		{name: "a say test", raw: `[{"test":"say"}]`, want: 1},
		{name: "a number test", raw: `[{"test":"number","who":"any","field":"hp","op":"lte","value":3,"open":true}]`, want: 1},
		{name: "a tracker test", raw: `[{"test":"tracker","who":"holder","kind":"k","op":"gte","value":2}]`, want: 1},
		{name: "four tests are the most", raw: `[{"test":"say"},{"test":"say"},{"test":"say"},{"test":"say"}]`, want: 4},
		{name: "five tests are too many", raw: `[{"test":"say"},{"test":"say"},{"test":"say"},{"test":"say"},{"test":"say"}]`, wantErr: "at most 4"},
		{name: "not a list", raw: `{"test":"say"}`, wantErr: "must be a list"},
		{name: "a test with no kind", raw: `[{"who":"any"}]`, wantErr: "needs a \"test\""},
		{name: "an unknown kind", raw: `[{"test":"weather"}]`, wantErr: "unknown kind"},
		{name: "a key of another kind", raw: `[{"test":"tag","tag":1,"has":true,"field":"hp"}]`, wantErr: `unknown key "field"`},
		{name: "a typo is refused by name", raw: `[{"test":"number","who":"any","feild":"hp"}]`, wantErr: `unknown key "feild"`},
		{name: "a key on say", raw: `[{"test":"say","who":"any"}]`, wantErr: `unknown key "who"`},
		{name: "a wrong value type", raw: `[{"test":"number","who":"any","field":"hp","op":"lte","value":"three"}]`, wantErr: `"value" has the wrong kind`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWhen([]byte(tt.raw))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code(err) != http.StatusUnprocessableEntity {
					t.Fatalf("err = %v (%d), want one containing %q", err, code(err), tt.wantErr)
				}
				return
			}
			if err != nil || len(got) != tt.want {
				t.Fatalf("got %d tests, err %v; want %d", len(got), err, tt.want)
			}
		})
	}
}

// A body's "when" goes through the decoder, so a typo in a request is refused
// with the same words and never stored.
func TestWhenListDecodesFromARequestBody(t *testing.T) {
	var in CreateCardInput
	if err := json.Unmarshal([]byte(`{"name":"A","when":[{"test":"say","open":true}]}`), &in); err != nil || len(in.When) != 1 || !in.When[0].Open {
		t.Fatalf("decoded %+v, %v", in, err)
	}
	if err := json.Unmarshal([]byte(`{"name":"A","when":[{"test":"nope"}]}`), &in); err == nil {
		t.Fatal("an unknown kind decoded")
	}
}

func TestValidateWhen(t *testing.T) {
	tests := []struct {
		name    string
		deck    string // hero deck for h-tam when "hero"
		in      WhenTest
		wantErr string
	}{
		{name: "say", in: WhenTest{Test: TestSay}},
		{name: "say takes no other key", in: WhenTest{Test: TestSay, Who: "any"}, wantErr: `"say" takes no other keys`},
		{name: "number against a value", in: WhenTest{Test: TestNumber, Who: WhoAny, Field: "hp", Op: "lte", Value: fl(3)}},
		{name: "number against a share of another field", in: WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Of: "hpmax", Share: "half"}},
		{name: "a share defaults to all", in: WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lt", Of: "hpmax"}},
		{name: "number needs a field and an op", in: WhenTest{Test: TestNumber, Who: WhoAny, Value: fl(1)}, wantErr: "needs \"field\""},
		{name: "number needs one comparison", in: WhenTest{Test: TestNumber, Who: WhoAny, Field: "hp", Op: "lte"}, wantErr: "either"},
		{name: "number not both comparisons", in: WhenTest{Test: TestNumber, Who: WhoAny, Field: "hp", Op: "lte", Value: fl(1), Of: "hpmax"}, wantErr: "either"},
		{name: "a share goes with of", in: WhenTest{Test: TestNumber, Who: WhoAny, Field: "hp", Op: "lte", Value: fl(1), Share: "half"}, wantErr: `"share" goes with "of"`},
		{name: "a field the page does not have", in: WhenTest{Test: TestNumber, Who: "h-tam", Field: "mana", Op: "lte", Value: fl(1)}, wantErr: "not a number field"},
		{name: "an unknown share", in: WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Of: "hpmax", Share: "most"}, wantErr: "unknown share"},
		{name: "a huge number", in: WhenTest{Test: TestNumber, Who: WhoAny, Field: "hp", Op: "lte", Value: fl(1e15)}, wantErr: "not a usable number"},
		{name: "who is needed", in: WhenTest{Test: TestNumber, Field: "hp", Op: "lte", Value: fl(1)}, wantErr: `"who" is needed`},
		{name: "a page outside the campaign", in: WhenTest{Test: TestTag, Who: "h-elsewhere", Tag: it(7), Has: bl(true)}, wantErr: "not in this campaign"},
		{name: "holder on a deck with no hero", in: WhenTest{Test: TestTag, Who: WhoHolder, Tag: it(7), Has: bl(true)}, wantErr: "only for a hero deck"},
		{name: "a tracker", in: WhenTest{Test: TestTracker, Who: WhoAny, Kind: "k-doom", Op: "gte", Value: fl(3)}},
		{name: "a tracker value is whole", in: WhenTest{Test: TestTracker, Who: WhoAny, Kind: "k-doom", Op: "gte", Value: fl(2.5)}, wantErr: "whole number"},
		{name: "an unknown tracker kind", in: WhenTest{Test: TestTracker, Who: WhoAny, Kind: "k-nope", Op: "gte", Value: fl(3)}, wantErr: "tracker kind does not exist"},
		{name: "a tag", in: WhenTest{Test: TestTag, Who: WhoAny, Tag: it(7), Has: bl(false)}},
		{name: "an unknown tag", in: WhenTest{Test: TestTag, Who: WhoAny, Tag: it(99), Has: bl(true)}, wantErr: "tag does not exist"},
		{name: "a tag test needs has", in: WhenTest{Test: TestTag, Who: WhoAny, Tag: it(7)}, wantErr: `needs "tag" and "has"`},
		{name: "a layer", in: WhenTest{Test: TestLayer, Who: WhoAny, Name: "  Vampire Scion ", On: bl(true)}},
		{name: "a layer needs a name", in: WhenTest{Test: TestLayer, Who: WhoAny, Name: " ", On: bl(true)}, wantErr: "layer's name"},
		{name: "a long layer name", in: WhenTest{Test: TestLayer, Who: WhoAny, Name: strings.Repeat("n", 81), On: bl(true)}, wantErr: "too long"},
		{name: "an unknown kind", in: WhenTest{Test: "weather"}, wantErr: "unknown kind"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newFixture(t)
			newFactsFixture(t, fx, newFakeFacts())
			got, err := fx.svc.ValidateWhen(bg, camp, Deck{Kind: KindEvent}, []WhenTest{tt.in})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || code(err) != http.StatusUnprocessableEntity {
					t.Fatalf("err = %v (%d), want one containing %q", err, code(err), tt.wantErr)
				}
				if !strings.Contains(err.Error(), "test 1:") {
					t.Errorf("the message does not say which test: %v", err)
				}
				return
			}
			if err != nil || len(got) != 1 {
				t.Fatalf("got %+v, %v", got, err)
			}
			if tt.in.Test == TestLayer && got[0].Name != "Vampire Scion" {
				t.Errorf("the layer name was not trimmed: %q", got[0].Name)
			}
			if tt.name == "a share defaults to all" && got[0].Share != "all" {
				t.Errorf("share = %q", got[0].Share)
			}
		})
	}
}

func TestValidateWhenOnAHeroDeckAcceptsHolder(t *testing.T) {
	fx := newFixture(t)
	newFactsFixture(t, fx, newFakeFacts())
	d := Deck{Kind: KindHero, HolderEntityID: "h-tam"}
	if _, err := fx.svc.ValidateWhen(bg, camp, d, []WhenTest{{Test: TestTag, Who: WhoHolder, Tag: it(7), Has: bl(true)}}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateWhenWithoutFactsAllowsOnlySay(t *testing.T) {
	fx := newFixture(t)
	if _, err := fx.svc.ValidateWhen(bg, camp, Deck{}, []WhenTest{{Test: TestSay}}); err != nil {
		t.Fatal(err)
	}
	_, err := fx.svc.ValidateWhen(bg, camp, Deck{}, []WhenTest{{Test: TestTag, Who: WhoAny, Tag: it(7), Has: bl(true)}})
	if err == nil || !strings.Contains(err.Error(), "only \"say\" is") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateWhenIsAllOrNothingAndLimited(t *testing.T) {
	fx := newFixture(t)
	newFactsFixture(t, fx, newFakeFacts())
	_, err := fx.svc.ValidateWhen(bg, camp, Deck{}, []WhenTest{{Test: TestSay}, {Test: TestTag, Who: WhoAny, Tag: it(99), Has: bl(true)}})
	if err == nil || !strings.Contains(err.Error(), "test 2:") {
		t.Fatalf("err = %v", err)
	}
	five := make([]WhenTest, 5)
	for i := range five {
		five[i] = WhenTest{Test: TestSay}
	}
	if _, err := fx.svc.ValidateWhen(bg, camp, Deck{}, five); err == nil {
		t.Fatal("five tests were accepted")
	}
}

func TestSentenceWriter(t *testing.T) {
	fx := newFixture(t)
	newFactsFixture(t, fx, newFakeFacts())
	s := fx.svc.(*service)
	d := Deck{Kind: KindHero, HolderEntityID: "h-tam"}
	n := s.newNamer(bg, camp, d, fx.dir)
	tests := []struct {
		in   WhenTest
		want string
	}{
		{WhenTest{Test: TestSay}, "the Director says so"},
		{WhenTest{Test: TestNumber, Who: "h-tam", Field: "hp", Op: "lte", Of: "hpmax", Share: "half"}, "Tam's Hit points is at or below half of Max hit points"},
		{WhenTest{Test: TestNumber, Who: WhoAny, Field: "stamina", Op: "gte", Value: fl(4)}, "any hero's Stamina is at or above 4"},
		{WhenTest{Test: TestNumber, Who: WhoHolder, Field: "hp", Op: "lt", Of: "hpmax", Share: "all"}, "Tam's Hit points is below Max hit points"},
		{WhenTest{Test: TestTracker, Who: "h-ilse", Kind: "k-doom", Op: "eq", Value: fl(2)}, "Ilse's Doom is exactly 2"},
		{WhenTest{Test: TestTag, Who: WhoAny, Tag: it(7), Has: bl(true)}, "any hero has the tag Cursed"},
		{WhenTest{Test: TestTag, Who: WhoAny, Tag: it(7), Has: bl(false)}, "any hero does not have the tag Cursed"},
		{WhenTest{Test: TestLayer, Who: "h-tam", Name: "Revenant", On: bl(false)}, "Tam is not Revenant"},
		{WhenTest{Test: TestTag, Who: "h-gone", Tag: it(99), Has: bl(true)}, "a page has the tag a tag"},
	}
	for _, tt := range tests {
		if got := n.sentence(tt.in); got != tt.want {
			t.Errorf("%+v: %q, want %q", tt.in, got, tt.want)
		}
	}
	line := n.printedLine([]WhenTest{{Test: TestSay, Open: true}, {Test: TestTag, Who: WhoAny, Tag: it(8), Has: bl(true)}, {Test: TestTag, Who: WhoAny, Tag: it(7), Has: bl(true), Open: true}})
	if line != "When the Director says so and any hero has the tag Cursed." {
		t.Errorf("printed line = %q", line)
	}
	if strings.Contains(line, "spy") {
		t.Error("a hidden test was printed")
	}
	if n.printedLine([]WhenTest{{Test: TestSay}}) != "" {
		t.Error("a card with only hidden tests printed a line")
	}
}

// A player is told the name of a hero only when they may open the page.
func TestPrintedLineNamesOnlyHeroesThePlayerMayOpen(t *testing.T) {
	fx := newFixture(t)
	f := newFakeFacts()
	newFactsFixture(t, fx, f)
	s := fx.svc.(*service)
	test := []WhenTest{{Test: TestTag, Who: "h-vault", Tag: it(7), Has: bl(true), Open: true}}
	if got := s.newNamer(bg, camp, Deck{}, fx.dir).printedLine(test); !strings.Contains(got, "Hidden") {
		t.Errorf("the Director reads %q", got)
	}
	got := s.newNamer(bg, camp, Deck{}, fx.tam).printedLine(test)
	if strings.Contains(got, "Hidden") || got != "When a page has the tag Cursed." {
		t.Errorf("the player reads %q", got)
	}
}

func TestDerivedOpenAndHidden(t *testing.T) {
	if derivedOpen(nil) || hasHidden(nil) {
		t.Error("no tests is neither open nor hidden")
	}
	mixed := []WhenTest{{Test: TestSay, Open: true}, {Test: TestSay}}
	if !derivedOpen(mixed) || !hasHidden(mixed) {
		t.Error("a mixed list is both")
	}
	if hasHidden([]WhenTest{{Test: TestSay, Open: true}}) {
		t.Error("an all-printed list has no hidden test")
	}
}

func TestEncodeDecodeWhen(t *testing.T) {
	in := []WhenTest{{Test: TestNumber, Who: WhoAny, Field: "hp", Op: "lte", Value: fl(3), Open: true}}
	got := decodeWhen(encodeWhen(in))
	if len(got) != 1 || got[0].Field != "hp" || *got[0].Value != 3 || !got[0].Open {
		t.Fatalf("round trip = %+v", got)
	}
	if encodeWhen(nil) != "" || decodeWhen("") != nil || decodeWhen("{not json") != nil {
		t.Error("none, empty and unreadable must all read as no tests")
	}
}
