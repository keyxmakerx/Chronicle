package crafting

import "testing"

func TestCondOn(t *testing.T) {
	day := &Day{
		Moons: []DayMoon{
			{Name: "Selune", Phase: 0.5},
			{Name: "Thin", Phase: 0.2},
			{Name: "Old", Phase: 0.7},
			{Name: "Blood", Manual: true, State: "red"},
			{Name: "Unmarked", Manual: true},
		},
		Events:  []string{"Festival of Lanterns"},
		Rolled:  map[string]string{"Omens": "Ill"},
		Weather: "Storm",
		Season:  "Winter",
	}
	tower, none := "Tower lab", ""
	cases := []struct {
		name  string
		day   *Day
		cond  Cond
		bench *string
		want  tri
	}{
		{"always", nil, Cond{Kind: "always"}, nil, on},
		{"full moon", day, Cond{Kind: "moon", Moon: "Selune", Phase: "full"}, nil, on},
		{"not new", day, Cond{Kind: "moon", Moon: "Selune", Phase: "new"}, nil, off},
		{"waxing", day, Cond{Kind: "moon", Moon: "Thin", Phase: "waxing"}, nil, on},
		{"waning", day, Cond{Kind: "moon", Moon: "Old", Phase: "waning"}, nil, on},
		{"full is neither waxing nor waning", day, Cond{Kind: "moon", Moon: "Selune", Phase: "waxing"}, nil, off},
		{"hand-set moon", day, Cond{Kind: "moon", Moon: "Blood", Phase: "red"}, nil, on},
		{"hand-set moon before its first mark", day, Cond{Kind: "moon", Moon: "Unmarked", Phase: ""}, nil, off},
		{"a moon the calendar lacks", day, Cond{Kind: "moon", Moon: "Gone", Phase: "full"}, nil, off},
		{"no calendar", nil, Cond{Kind: "moon", Moon: "Selune", Phase: "full"}, nil, off},
		{"event today", day, Cond{Kind: "event", Event: "Festival of Lanterns"}, nil, on},
		{"event not today", day, Cond{Kind: "event", Event: "Harvest"}, nil, off},
		{"rolled result", day, Cond{Kind: "rolled", Event: "Omens", Result: "Ill"}, nil, on},
		{"other rolled result", day, Cond{Kind: "rolled", Event: "Omens", Result: "Good"}, nil, off},
		{"weather", day, Cond{Kind: "weather", Weather: "Storm"}, nil, on},
		{"season", day, Cond{Kind: "season", Season: "Summer"}, nil, off},
		{"bench before the roll", day, Cond{Kind: "bench", Bench: "Tower lab"}, nil, maybe},
		{"at the bench", day, Cond{Kind: "bench", Bench: "Tower lab"}, &tower, on},
		{"at no bench", day, Cond{Kind: "bench", Bench: "Tower lab"}, &none, off},
		{"time of day is never known", day, Cond{Kind: "time", Time: "night"}, &tower, maybe},
		{"ingredient marks are never known", day, Cond{Kind: "ingredient", Mark: "fresh"}, &tower, maybe},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := condOn(tc.day, tc.cond, tc.bench); got != tc.want {
				t.Fatalf("condOn = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPhaseName(t *testing.T) {
	for p, want := range map[float64]string{0: "new", 0.05: "new", 0.1: "waxing crescent", 0.25: "first quarter", 0.5: "full", 0.75: "last quarter", 0.95: "new"} {
		if got := phaseName(p); got != want {
			t.Errorf("phaseName(%v) = %q, want %q", p, got, want)
		}
	}
}

func TestChanceIn(t *testing.T) {
	for s, want := range map[string]int{"always": 1, "": 1, "1 in 4": 4, "1 in 1": 1, "1 in x": 1} {
		if got := chanceIn(s); got != want {
			t.Errorf("chanceIn(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestHold(t *testing.T) {
	day := &Day{Moons: []DayMoon{{Name: "Selune", Phase: 0}}}
	full := []Cond{{Kind: "moon", Moon: "Selune", Phase: "full"}}
	bench := []Cond{{Kind: "bench", Bench: "Tower lab"}}
	cases := []struct {
		name     string
		rules    []Rule
		director bool
		want     string
	}{
		{"no rules", nil, false, ""},
		{"only then, off", []Rule{{When: full, Then: Effect{Kind: "only"}, Players: "book", BookLine: "Only at full moon."}}, false, "Only at full moon."},
		{"only then, one of two maybe", []Rule{
			{When: full, Then: Effect{Kind: "only"}, Players: "book"},
			{When: bench, Then: Effect{Kind: "only"}, Players: "book"},
		}, false, ""},
		{"secret only then, to a player", []Rule{{When: full, Then: Effect{Kind: "only"}, Players: "secret"}}, false, "A rule the Director keeps hidden."},
		{"secret only then, to the Director", []Rule{{When: full, Then: Effect{Kind: "only"}, Players: "secret", BookLine: "Full moon only."}}, true, "Full moon only."},
		{"cannot, always", []Rule{{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "cannot"}, Players: "book", BookLine: "Never."}}, false, "Never."},
		{"cannot by chance never holds", []Rule{{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "cannot"}, Chance: "1 in 6", Players: "book"}}, false, ""},
		{"cannot, maybe", []Rule{{When: bench, Then: Effect{Kind: "cannot"}, Players: "book"}}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hold(day, tc.rules, nil, tc.director); got != tc.want {
				t.Fatalf("hold = %q, want %q", got, tc.want)
			}
		})
	}
}
