package calendar

import (
	"reflect"
	"strings"
	"testing"
)

func ruleOf(id, raw string) StructureRule {
	return StructureRule{EventID: id, Name: "Rule " + id, Raw: raw}
}

func TestPlanStructureEdit_Repeats(t *testing.T) {
	const (
		monthRule  = `{"match":[{"kind":"month","month":1},{"kind":"weekday","weekday":5}]}`
		monthsRule = `{"match":[{"kind":"months","months":[2,3]}],"every":2,"offset_days":1}`
		moonRule   = `{"match":[{"kind":"moon_phase","moon_id":7,"phase":"full"}]}`
		seasonRule = `{"match":[{"kind":"season_start","season_id":3}]}`
		anySeason  = `{"match":[{"kind":"season_start"}]}`
	)
	swap := func(e *StructureEdit) { e.Months = []MonthInput{e.Months[2], e.Months[1], e.Months[0]} }
	dropBeta := func(e *StructureEdit) { e.Months = []MonthInput{e.Months[0], e.Months[2]} }

	tests := []struct {
		name      string
		rules     []StructureRule
		overrides []StructureOverride
		change    func(e *StructureEdit)

		updates       map[string]string
		counts        [6]int // rules updated, rules removed month, rules removed moon/season, overrides moved, removed, replaced
		notesHave     []string
		notesHaveNone []string
	}{
		{
			name:    "a swapped pair rewrites a month rule and moves a skip and a move override",
			rules:   []StructureRule{ruleOf("a", monthRule), ruleOf("b", moonRule)},
			change:  swap,
			updates: map[string]string{"a": `{"match":[{"kind":"month","month":3},{"kind":"weekday","weekday":5}]}`},
			overrides: []StructureOverride{
				{EventID: "a", Year: 4, Month: 1, Day: 2},
				{EventID: "a", Year: 4, Month: 3, Day: 2, NewMonth: intPtr(1)},
			},
			counts:    [6]int{1, 0, 0, 2, 0, 0},
			notesHave: []string{"1 repeat rule follow the months they name", "2 one-off changes to repeats (skipped or moved dates) follow their month"},
		},
		{
			name:  "a months list is rewritten and de-duplicated, keeping every and offset",
			rules: []StructureRule{ruleOf("m", monthsRule)},
			change: func(e *StructureEdit) {
				e.Months = []MonthInput{e.Months[0], e.Months[2], e.Months[1]}
			},
			updates: map[string]string{"m": `{"match":[{"kind":"months","months":[3,2]}],"every":2,"offset_days":1}`},
			counts:  [6]int{1, 0, 0, 0, 0, 0},
		},
		{
			name:   "a removed month is left as written and named, never rewritten",
			rules:  []StructureRule{ruleOf("a", `{"match":[{"kind":"month","month":2}]}`), ruleOf("g", `{"match":[{"kind":"month","month":3}]}`)},
			change: dropBeta,
			// Only Gamma's rule follows its month (3 to 2); Beta's stays.
			updates:   map[string]string{"g": `{"match":[{"kind":"month","month":2}]}`},
			counts:    [6]int{1, 1, 0, 0, 0, 0},
			notesHave: []string{`The repeat rule of "Rule a" names Beta, which is removed; the rule is left as written and will match the month now in that place.`},
		},
		{
			name:      "a removed last month matches nothing",
			rules:     []StructureRule{ruleOf("g", `{"match":[{"kind":"month","month":3}]}`)},
			change:    func(e *StructureEdit) { e.Months = e.Months[:2] },
			counts:    [6]int{0, 1, 0, 0, 0, 0},
			notesHave: []string{"names Gamma, which is removed; the rule is left as written and will match nothing there."},
		},
		{
			name: "overrides in a removed month stay and are counted; a moved one claiming the date deletes the other",
			overrides: []StructureOverride{
				{EventID: "a", Year: 4, Month: 2, Day: 9},                      // Beta removed: stays
				{EventID: "a", Year: 4, Month: 2, Day: 5},                      // Beta removed: displaced by Gamma 5
				{EventID: "a", Year: 4, Month: 3, Day: 5},                      // Gamma moves to 2nd
				{EventID: "a", Year: 4, Month: 1, Day: 1, NewMonth: intPtr(3)}, // move target follows Gamma
				{EventID: "a", Year: 4, Month: 1, Day: 2, NewMonth: intPtr(2)}, // move target in the removed month
			},
			change:    dropBeta,
			counts:    [6]int{0, 0, 0, 2, 2, 1},
			notesHave: []string{"1 one-off change to a repeat (skipped or moved date) in removed months will be deleted", "2 one-off changes to repeats (skipped or moved dates) sit in removed months"},
		},
		{
			name:      "a removed moon names the rule that repeats by it",
			rules:     []StructureRule{ruleOf("b", moonRule), ruleOf("c", seasonRule)},
			change:    func(e *StructureEdit) { e.Moons = nil },
			counts:    [6]int{0, 0, 1, 0, 0, 0},
			notesHave: []string{`"Rule b" repeats by the moon Luna you're removing, so it will match nothing.`},
		},
		{
			name:      "a removed season names its rule, but a rule for any season is untouched",
			rules:     []StructureRule{ruleOf("c", seasonRule), ruleOf("d", anySeason)},
			change:    func(e *StructureEdit) { e.Seasons = nil },
			counts:    [6]int{0, 0, 1, 0, 0, 0},
			notesHave: []string{`"Rule c" repeats by the season Warm you're removing`},
		},
		{
			name:          "an unreadable stored rule is left alone",
			rules:         []StructureRule{ruleOf("x", `{"match":[{"kind":"month","month":1}],"bogus":1}`)},
			change:        swap,
			counts:        [6]int{},
			notesHaveNone: []string{"repeat rule"},
		},
		{
			name:      "no structure change touches nothing",
			rules:     []StructureRule{ruleOf("a", monthRule)},
			change:    func(*StructureEdit) {},
			overrides: []StructureOverride{{EventID: "a", Year: 4, Month: 1, Day: 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := structureFixture()
			edit := editFrom(cal)
			tt.change(&edit)
			plan := planStructureEdit(cal, nil, nil, tt.rules, tt.overrides, edit)
			p := plan.preview

			got := map[string]string{}
			for _, u := range plan.ruleUpdates {
				got[u.EventID] = u.JSON
			}
			want := tt.updates
			if want == nil {
				want = map[string]string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("rule updates = %v, want %v", got, want)
			}
			counts := [6]int{p.RulesUpdated, p.RulesRemovedMonth, p.RulesRemovedRef, p.OverridesMoved, p.OverridesRemoved, p.OverridesReplaced}
			if counts != tt.counts {
				t.Errorf("counts (rules updated, removed month, removed moon/season, overrides moved, removed, replaced) = %v, want %v", counts, tt.counts)
			}
			notes := strings.Join(p.OtherNotes, "\n")
			for _, h := range tt.notesHave {
				if !strings.Contains(notes, h) {
					t.Errorf("notes %q missing %q", p.OtherNotes, h)
				}
			}
			for _, h := range tt.notesHaveNone {
				if strings.Contains(notes, h) {
					t.Errorf("notes %q should not contain %q", p.OtherNotes, h)
				}
			}
		})
	}
}

func TestStructureFingerprint_CoversRulesAndOverrides(t *testing.T) {
	cal := structureFixture()
	rules := []StructureRule{ruleOf("a", `{"match":[{"kind":"month","month":1}]}`)}
	overrides := []StructureOverride{{EventID: "a", Year: 4, Month: 1, Day: 2}}
	base := structureFingerprint(cal, nil, nil, rules, overrides)

	changed := map[string]string{
		"rule text":      structureFingerprint(cal, nil, nil, []StructureRule{ruleOf("a", `{"match":[{"kind":"month","month":2}]}`)}, overrides),
		"rule removed":   structureFingerprint(cal, nil, nil, nil, overrides),
		"override moved": structureFingerprint(cal, nil, nil, rules, []StructureOverride{{EventID: "a", Year: 4, Month: 2, Day: 2}}),
		"override added": structureFingerprint(cal, nil, nil, rules, append(append([]StructureOverride{}, overrides...), StructureOverride{EventID: "a", Year: 5, Month: 1, Day: 2, NewMonth: intPtr(3)})),
	}
	for name, fp := range changed {
		if fp == base {
			t.Errorf("fingerprint did not change when %s changed", name)
		}
	}
	if again := structureFingerprint(cal, nil, nil, rules, overrides); again != base {
		t.Error("fingerprint is not stable for the same input")
	}
}
