package fieldrules

import "testing"

func fp(f float64) *float64 { return &f }

func TestRuleGives(t *testing.T) {
	tests := []struct {
		name  string
		rows  []RuleRow
		value any
		want  string
	}{
		{"is exact", []RuleRow{{Is: "gold", Gives: "A"}}, "gold", "A"},
		{"is ignores case", []RuleRow{{Is: "Gold", Gives: "A"}}, "gOLD", "A"},
		{"is ignores surrounding space", []RuleRow{{Is: " gold ", Gives: "A"}}, "  gold", "A"},
		{"numeric string vs float row", []RuleRow{{Is: "2", Gives: "A"}}, 2.0, "A"},
		{"numeric string vs decimal string", []RuleRow{{Is: "2", Gives: "A"}}, "2.0", "A"},
		{"decimal row vs integer float", []RuleRow{{Is: "2.0", Gives: "A"}}, 2.0, "A"},
		{"decimal row vs integer string", []RuleRow{{Is: "2.0", Gives: "A"}}, "2", "A"},
		{"different numbers do not match", []RuleRow{{Is: "2", Gives: "A"}}, 3.0, ""},
		{"fractional float", []RuleRow{{Is: "2.5", Gives: "A"}}, 2.5, "A"},
		{"bool value", []RuleRow{{Is: "true", Gives: "A"}}, true, "A"},
		{"from only inclusive", []RuleRow{{From: fp(5), Gives: "A"}}, 5.0, "A"},
		{"from only below", []RuleRow{{From: fp(5), Gives: "A"}}, 4.9, ""},
		{"from only far above", []RuleRow{{From: fp(5), Gives: "A"}}, 1e9, "A"},
		{"to only inclusive", []RuleRow{{To: fp(5), Gives: "A"}}, "5", "A"},
		{"to only above", []RuleRow{{To: fp(5), Gives: "A"}}, 5.1, ""},
		{"to only far below", []RuleRow{{To: fp(5), Gives: "A"}}, -100.0, "A"},
		{"both bounds inside", []RuleRow{{From: fp(1), To: fp(3), Gives: "A"}}, 2.0, "A"},
		{"both bounds lower edge", []RuleRow{{From: fp(1), To: fp(3), Gives: "A"}}, 1.0, "A"},
		{"both bounds upper edge", []RuleRow{{From: fp(1), To: fp(3), Gives: "A"}}, 3.0, "A"},
		{"both bounds outside", []RuleRow{{From: fp(1), To: fp(3), Gives: "A"}}, 3.01, ""},
		{"range skips non-numeric value", []RuleRow{{From: fp(1), Gives: "A"}}, "abc", ""},
		{"range works on numeric string", []RuleRow{{From: fp(1), To: fp(3), Gives: "A"}}, "2", "A"},
		{"first matching row wins", []RuleRow{
			{From: fp(0), To: fp(10), Gives: "low"},
			{From: fp(5), To: fp(20), Gives: "mid"},
		}, 7.0, "low"},
		{"first wins across kinds", []RuleRow{
			{Is: "7", Gives: "exact"},
			{From: fp(0), Gives: "range"},
		}, 7.0, "exact"},
		{"later row used when earlier misses", []RuleRow{
			{Is: "x", Gives: "no"},
			{From: fp(0), Gives: "yes"},
		}, 1.0, "yes"},
		{"no match", []RuleRow{{Is: "a", Gives: "A"}, {From: fp(10), Gives: "B"}}, "zzz", ""},
		{"nil value", []RuleRow{{Is: "", Gives: "A"}, {To: fp(100), Gives: "B"}}, nil, ""},
		{"empty string value", []RuleRow{{To: fp(100), Gives: "B"}}, "", ""},
		{"blank string value", []RuleRow{{To: fp(100), Gives: "B"}}, "   ", ""},
		{"no rows", nil, "x", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &Rule{Rows: tc.rows}
			if got := r.Gives(tc.value); got != tc.want {
				t.Errorf("Gives(%#v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestChooseWords(t *testing.T) {
	tests := []struct {
		name     string
		words    []LocalWord
		places   []string
		era      int
		director bool
		want     map[string]string
	}{
		{"nearest place beats farther", []LocalWord{
			{Word: "coin", Local: "far", PlaceID: "region"},
			{Word: "coin", Local: "near", PlaceID: "town"},
		}, []string{"town", "region"}, 0, false, map[string]string{"coin": "near"}},
		{"order of words does not matter", []LocalWord{
			{Word: "coin", Local: "near", PlaceID: "town"},
			{Word: "coin", Local: "far", PlaceID: "region"},
		}, []string{"town", "region"}, 0, false, map[string]string{"coin": "near"}},
		{"place beats era", []LocalWord{
			{Word: "coin", Local: "era", EraID: 2},
			{Word: "coin", Local: "place", PlaceID: "region"},
		}, []string{"town", "region"}, 2, false, map[string]string{"coin": "place"}},
		{"era applies when it is current", []LocalWord{
			{Word: "coin", Local: "era", EraID: 2},
		}, []string{"town"}, 2, false, map[string]string{"coin": "era"}},
		{"era ignored when not current", []LocalWord{
			{Word: "coin", Local: "era", EraID: 2},
		}, []string{"town"}, 3, false, map[string]string{}},
		{"era ignored when no current era", []LocalWord{
			{Word: "coin", Local: "era", EraID: 2},
		}, nil, 0, false, map[string]string{}},
		{"era applies with no places", []LocalWord{
			{Word: "coin", Local: "era", EraID: 2},
		}, nil, 2, false, map[string]string{"coin": "era"}},
		{"place not in chain ignored", []LocalWord{
			{Word: "coin", Local: "elsewhere", PlaceID: "other"},
		}, []string{"town"}, 0, false, map[string]string{}},
		{"dm only hidden from players", []LocalWord{
			{Word: "coin", Local: "secret", PlaceID: "town", DMOnly: true},
		}, []string{"town"}, 0, false, map[string]string{}},
		{"dm only falls back to public word for players", []LocalWord{
			{Word: "coin", Local: "secret", PlaceID: "town", DMOnly: true},
			{Word: "coin", Local: "public", PlaceID: "region"},
		}, []string{"town", "region"}, 0, false, map[string]string{"coin": "public"}},
		{"dm only shown to director", []LocalWord{
			{Word: "coin", Local: "secret", PlaceID: "town", DMOnly: true},
			{Word: "coin", Local: "public", PlaceID: "region"},
		}, []string{"town", "region"}, 0, true, map[string]string{"coin": "secret"}},
		{"keys lower-cased and trimmed", []LocalWord{
			{Word: "  Coin ", Local: "mark", PlaceID: "town"},
		}, []string{"town"}, 0, false, map[string]string{"coin": "mark"}},
		{"different words kept apart", []LocalWord{
			{Word: "Coin", Local: "mark", PlaceID: "town"},
			{Word: "Rank", Local: "grade", PlaceID: "town"},
		}, []string{"town"}, 0, false, map[string]string{"coin": "mark", "rank": "grade"}},
		{"duplicate place in chain keeps nearest rank", []LocalWord{
			{Word: "coin", Local: "a", PlaceID: "town"},
			{Word: "coin", Local: "b", PlaceID: "region"},
		}, []string{"town", "region", "town"}, 0, false, map[string]string{"coin": "a"}},
		{"no words", nil, []string{"town"}, 1, true, map[string]string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ChooseWords(tc.words, tc.places, tc.era, tc.director)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("got[%q] = %q, want %q (full %v)", k, got[k], v, got)
				}
			}
		})
	}
}

func TestRename(t *testing.T) {
	words := map[string]string{"mark": "Crown", "gold": "Sun"}
	tests := []struct {
		name   string
		words  map[string]string
		text   string
		want   string
		wantOK bool
	}{
		{"exact", words, "mark", "Crown", true},
		{"ignores case", words, "MaRk", "Crown", true},
		{"trims space", words, "  mark ", "Crown", true},
		{"plural s", words, "Marks", "Crown", true},
		{"plural keeps owner's casing of local", words, "GOLDS", "Sun", true},
		{"exact plural key wins over singular", map[string]string{"marks": "Pair", "mark": "Crown"}, "marks", "Pair", true},
		{"no match", words, "silver", "", false},
		{"only s", words, "s", "", false},
		{"empty text", words, "", "", false},
		{"blank text", words, "   ", "", false},
		{"nil words", nil, "mark", "", false},
		{"empty words", map[string]string{}, "mark", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Rename(tc.words, tc.text)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("Rename(%q) = (%q, %v), want (%q, %v)", tc.text, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// The Sanguine Hunger rule: a moon table, a Bloodnight override and a Hunger
// adjustment, read against different nights.
func TestRuleResult(t *testing.T) {
	hunger := Rule{
		Follows: "moon:Sanguin'mor",
		Rows: []RuleRow{
			{EventTag: "Bloodnight", Gives: "1"},
			{From: fp(24), To: fp(2), Gives: "none"},
			{From: fp(12), To: fp(14), Gives: "2"},
			{Is: "gibbous", Gives: "3"},
			{From: fp(1), To: fp(24), Gives: "4"},
		},
		Adjust: []RuleAdjust{{Tracker: "The Hunger", Op: "gte", Value: 1, Add: -1, Min: fp(1)}},
	}
	moon := func(day int, words ...string) map[string]MoonNow {
		return map[string]MoonNow{"sanguin'mor": {Day: day, Words: words}}
	}
	tests := []struct {
		name string
		rule Rule
		data map[string]any
		sit  Situation
		want string
	}{
		{"crescent, no hunger", hunger, nil, Situation{Moons: moon(5, "Waxing Crescent", "crescent"), Trackers: map[string]float64{"the hunger": 0}}, "4"},
		{"crescent, restless", hunger, nil, Situation{Moons: moon(5, "crescent"), Trackers: map[string]float64{"the hunger": 1}}, "3"},
		{"gibbous by word", hunger, nil, Situation{Moons: moon(9, "Waxing Gibbous", "gibbous")}, "3"},
		{"full, unbound, floor of 1", hunger, nil, Situation{Moons: moon(13, "full"), Trackers: map[string]float64{"the hunger": 3}}, "1"},
		{"new wraps the cycle end", hunger, nil, Situation{Moons: moon(24)}, "none"},
		{"new wraps the cycle start", hunger, nil, Situation{Moons: moon(1)}, "none"},
		{"text is never adjusted", hunger, nil, Situation{Moons: moon(1), Trackers: map[string]float64{"the hunger": 3}}, "none"},
		{"bloodnight overrides", hunger, nil, Situation{Moons: moon(5), EventTags: map[string]bool{"bloodnight": true}}, "1"},
		{"no calendar, no result", hunger, nil, Situation{}, ""},
		{"a moon not on the calendar", hunger, nil, Situation{Moons: map[string]MoonNow{"other": {Day: 5}}}, ""},
		{"hand-set moon has no days", hunger, nil, Situation{Moons: moon(0, "Full")}, ""},
		{"unread tracker leaves it alone", hunger, nil, Situation{Moons: moon(5)}, "4"},
		{"tracker rule", Rule{Follows: "tracker:The Hunger", Rows: []RuleRow{{Is: "2", Gives: "3"}, {Is: "3", Gives: "2"}}}, nil,
			Situation{Trackers: map[string]float64{"the hunger": 3}}, "2"},
		{"time of day rule", Rule{Follows: "time of day", Rows: []RuleRow{{Is: "Night", Gives: "dark"}}}, nil, Situation{DayPart: "night"}, "dark"},
		{"time condition", Rule{Follows: "level", Rows: []RuleRow{{Time: "night", From: fp(1), Gives: "A"}, {From: fp(1), Gives: "B"}}},
			map[string]any{"level": 3.0}, Situation{DayPart: "day"}, "B"},
		{"field rules still work", Rule{Follows: "level", Rows: []RuleRow{{From: fp(5), Gives: "High"}}}, map[string]any{"level": 7.0}, Situation{}, "High"},
		{"adjust caps at max", Rule{Follows: "level", Rows: []RuleRow{{From: fp(0), Gives: "4"}},
			Adjust: []RuleAdjust{{Tracker: "Boon", Op: "gt", Value: 0, Add: 5, Max: fp(6)}}},
			map[string]any{"level": 1.0}, Situation{Trackers: map[string]float64{"boon": 1}}, "6"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rule.Result(tc.data, tc.sit); got != tc.want {
				t.Errorf("Result = %q, want %q", got, tc.want)
			}
		})
	}
}
