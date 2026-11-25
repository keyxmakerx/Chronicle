package decks

import (
	"context"
	"strings"
	"testing"
)

// The AI reads and writes the calendar tests with the editor's words, and the
// card format names their keys and values from the server's own lists.
func TestAICardCalendarTests(t *testing.T) {
	fx := newFixture(t)
	newFactsFixture(t, fx, calendarFacts())
	r := &aiRig{t: t, fx: fx, on: true}
	r.ai = NewAI(fx.svc, aiPages{"tam": {"h-tam", "Tam"}}, func(context.Context, string) bool { return true })
	kinds := r.ai.Kinds()
	r.deck, r.card = kinds[0], kinds[1]
	fx.deck("Omens", KindEvent, nil)
	cases := []struct {
		name, summary, err string
		when               []any
	}{
		{name: "a moon, an event and the time of day",
			when: []any{yaml{"test": "moon", "name": "nyx", "phase": "Full", "is": true, "open": true},
				yaml{"test": "event", "name": "Bloodnight", "has": true}, yaml{"test": "time", "time": "night", "is": false}},
			summary: "adds Red Tide to Omens, 1 copy; comes up when Nyx is full (on the card) and Bloodnight is today (only you) and it is not night (only you)"},
		{name: "a moon on days that wrap", when: []any{yaml{"test": "moon", "name": "Nyx", "from": 23, "to": 1, "is": true}},
			summary: "adds Red Tide to Omens, 1 copy; comes up when Nyx is on day 23 to 1 of its cycle (only you)"},
		{name: "a moon the calendar lacks", when: []any{yaml{"test": "moon", "name": "Luna", "phase": "full", "is": true}},
			err: "test 1: the main calendar has no moon by that name"},
		{name: "a calendar test takes no who", when: []any{yaml{"test": "time", "who": "Tam", "time": "night", "is": true}},
			err: `test 1 (time) has an unknown key "who"`},
	}
	for _, tc := range cases {
		p := r.card.Plan(bg, camp, aiDirector, aiRec("card", "create", "Red Tide", map[string]any{"deck": "Omens", "when": tc.when}))
		if tc.err != "" {
			if !strings.Contains(p.Error, tc.err) {
				t.Errorf("%s: plan = %+v, want error %q", tc.name, p, tc.err)
			}
			continue
		}
		if p.Error != "" || p.Summary != tc.summary {
			t.Errorf("%s: plan = %+v, want %q", tc.name, p, tc.summary)
		}
	}
	doc := r.card.Doc()
	for _, w := range append(append([]OptionWord{}, moonPhases...), dayParts...) {
		if !strings.Contains(doc, w.Key) {
			t.Errorf("the card's format lacks %q", w.Key)
		}
	}
	for _, k := range []string{TestMoon, TestEvent, TestTime} {
		for _, key := range keysOf[k] {
			if !strings.Contains(doc, "`"+key+"`") {
				t.Errorf("the card's format lacks %s's key %q", k, key)
			}
		}
	}
}
