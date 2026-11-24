package calendar

import (
	"slices"
	"testing"
)

func TestMoonCycleDayAndPhaseWords(t *testing.T) {
	m := &Moon{Name: "Red", CycleDays: 24}
	tests := []struct {
		abs     int
		day     int
		has     []string
		hasNone []string
	}{
		{0, 1, []string{"New Moon", "new"}, []string{"full", "waxing"}},
		{3, 4, []string{"Waxing Crescent", "waxing", "crescent"}, []string{"new", "gibbous"}},
		{12, 13, []string{"Full Moon", "full"}, []string{"waxing", "waning"}},
		{15, 16, []string{"Waning Gibbous", "waning", "gibbous"}, []string{"full"}},
		{23, 24, []string{"waning", "crescent"}, []string{"full"}},
	}
	for _, tc := range tests {
		if got := m.CycleDay(tc.abs); got != tc.day {
			t.Errorf("day %d: CycleDay = %d, want %d", tc.abs, got, tc.day)
		}
		words := m.PhaseWords(tc.abs)
		for _, w := range tc.has {
			if !slices.Contains(words, w) {
				t.Errorf("day %d: words %v lack %q", tc.abs, words, w)
			}
		}
		for _, w := range tc.hasNone {
			if slices.Contains(words, w) {
				t.Errorf("day %d: words %v hold %q", tc.abs, words, w)
			}
		}
	}
	if got := (&Moon{CycleDays: 0}).PhaseWords(5); got != nil {
		t.Errorf("a moon with no cycle answers %v", got)
	}
	if got := (&Moon{PhaseMode: MoonManual, CycleDays: 24}).CycleDay(5); got != 0 {
		t.Errorf("a hand-set moon counts day %d", got)
	}
}
