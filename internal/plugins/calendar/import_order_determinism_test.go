package calendar

import (
	"strings"
	"testing"
)

// Calendaria stores months, weekdays, moons, seasons and eras as JSON OBJECTS,
// which parseCalendaria walks with `for k, v := range`. Go randomises map
// iteration, so every one of those lists needs a TOTAL comparator or the same
// bytes parse into a different order run to run — and the order is exactly what
// becomes MonthInput.SortOrder / the INSERT order the repository reads rows back
// in (GetMoons / GetSeasons have no ORDER BY, so the read-back order IS the
// insert order). Moons were never sorted at all; seasons sorted on DayStart,
// which presets/elven.json ties three ways at 0.
//
// Each subtest parses the SAME bytes many times and fails if any two parses
// disagree. Ten iterations already catch a randomised 2-element list with
// probability 1-2^-9; the counts below are generous on top of that.

const orderParseRuns = 100

// TestParseCalendaria_ElvenPresetOrderIsDeterministic pins the shipped payload
// that actually reproduces this: presets/elven.json is the Start gallery's
// Elven card, it carries two moons and three seasons, and all three seasons
// declare dayStart 0.
func TestParseCalendaria_ElvenPresetOrderIsDeterministic(t *testing.T) {
	raw, err := presetFS.ReadFile("presets/elven.json")
	if err != nil {
		t.Fatalf("read embedded elven preset: %v", err)
	}
	if got := detectFormat(raw); got != FormatCalendaria {
		t.Fatalf("elven.json must land on the Calendaria parser; detectFormat = %q", got)
	}

	var firstMoons, firstSeasons string
	for i := 0; i < orderParseRuns; i++ {
		res, err := DetectAndParse(raw)
		if err != nil {
			t.Fatalf("run %d: DetectAndParse: %v", i, err)
		}
		moons := make([]string, 0, len(res.Moons))
		for _, m := range res.Moons {
			moons = append(moons, m.Name)
		}
		seasons := make([]string, 0, len(res.Seasons))
		for _, s := range res.Seasons {
			seasons = append(seasons, s.Name)
		}
		gotMoons, gotSeasons := strings.Join(moons, "|"), strings.Join(seasons, "|")
		if i == 0 {
			firstMoons, firstSeasons = gotMoons, gotSeasons
			continue
		}
		if gotMoons != firstMoons {
			t.Fatalf("run %d: moon order drifted: first %q, now %q", i, firstMoons, gotMoons)
		}
		if gotSeasons != firstSeasons {
			t.Fatalf("run %d: season order drifted: first %q, now %q", i, firstSeasons, gotSeasons)
		}
	}

	// The seasons carry ordinal 1/2/3 while tying on dayStart, so the authored
	// rank — not the tie — is what must decide.
	if firstSeasons != "Budding|Zenith|Waning" {
		t.Errorf("elven seasons must come out in their authored ordinal order; got %q", firstSeasons)
	}
	// Moons have no ordinal anywhere in Calendaria, so the map key ranks them:
	// "lira000000000000" < "sehanine00000000".
	if firstMoons != "Lira|Sehanine" {
		t.Errorf("elven moons must come out in authored-key order; got %q", firstMoons)
	}
}

// TestParseCalendaria_TiedListsAreDeterministic covers the tie on every list
// parseCalendaria builds out of a map, including the three whose comparators
// were already sorting but not totally (months / weekdays on ordinal, eras on
// start year — all tie-able).
func TestParseCalendaria_TiedListsAreDeterministic(t *testing.T) {
	// Every month shares ordinal 1, every weekday ordinal 0, every era start
	// year 100, every season dayStart 0 with no ordinal at all, and the moons
	// have nothing to rank them by construction.
	raw := []byte(`{
		"name": "All Ties",
		"months": {
			"m-a": { "name": "Alpha", "days": 30, "ordinal": 1 },
			"m-b": { "name": "Bravo", "days": 30, "ordinal": 1 },
			"m-c": { "name": "Charlie", "days": 30, "ordinal": 1 },
			"m-d": { "name": "Delta", "days": 30, "ordinal": 1 }
		},
		"days": { "values": {
			"d-a": { "name": "Aday", "ordinal": 0 },
			"d-b": { "name": "Bday", "ordinal": 0 },
			"d-c": { "name": "Cday", "ordinal": 0 },
			"d-d": { "name": "Dday", "ordinal": 0 }
		} },
		"moons": {
			"n-a": { "name": "Amoon", "cycleLength": 10 },
			"n-b": { "name": "Bmoon", "cycleLength": 10 },
			"n-c": { "name": "Cmoon", "cycleLength": 10 },
			"n-d": { "name": "Dmoon", "cycleLength": 10 }
		},
		"seasons": {
			"s-a": { "name": "Aseason", "dayStart": 0, "dayEnd": 10 },
			"s-b": { "name": "Bseason", "dayStart": 0, "dayEnd": 10 },
			"s-c": { "name": "Cseason", "dayStart": 0, "dayEnd": 10 },
			"s-d": { "name": "Dseason", "dayStart": 0, "dayEnd": 10 }
		},
		"eras": {
			"e-a": { "name": "Aera", "startYear": 100 },
			"e-b": { "name": "Bera", "startYear": 100 },
			"e-c": { "name": "Cera", "startYear": 100 },
			"e-d": { "name": "Dera", "startYear": 100 }
		}
	}`)

	// Every list is authored with keys that sort a-b-c-d, so a total comparator
	// yields the alphabetical names below on every single parse.
	want := map[string]string{
		"months":   "Alpha|Bravo|Charlie|Delta",
		"weekdays": "Aday|Bday|Cday|Dday",
		"moons":    "Amoon|Bmoon|Cmoon|Dmoon",
		"seasons":  "Aseason|Bseason|Cseason|Dseason",
		"eras":     "Aera|Bera|Cera|Dera",
	}

	for i := 0; i < orderParseRuns; i++ {
		res, err := parseCalendaria(raw)
		if err != nil {
			t.Fatalf("run %d: parseCalendaria: %v", i, err)
		}
		got := map[string]string{
			"months":   joinNames(len(res.Months), func(k int) string { return res.Months[k].Name }),
			"weekdays": joinNames(len(res.Weekdays), func(k int) string { return res.Weekdays[k].Name }),
			"moons":    joinNames(len(res.Moons), func(k int) string { return res.Moons[k].Name }),
			"seasons":  joinNames(len(res.Seasons), func(k int) string { return res.Seasons[k].Name }),
			"eras":     joinNames(len(res.Eras), func(k int) string { return res.Eras[k].Name }),
		}
		for list, w := range want {
			if got[list] != w {
				t.Fatalf("run %d: %s order is not deterministic: want %q, got %q", i, list, w, got[list])
			}
		}
		// SortOrder is written from the loop index, so a drifting order is a
		// drifting persisted sort_order — the reason this matters at all.
		for k, m := range res.Months {
			if m.SortOrder != k {
				t.Fatalf("run %d: month %q has SortOrder %d at index %d", i, m.Name, m.SortOrder, k)
			}
		}
	}
}

// joinNames renders an ImportResult sub-list as a stable "A|B|C" signature.
func joinNames(n int, at func(int) string) string {
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, at(i))
	}
	return strings.Join(parts, "|")
}
