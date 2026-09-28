package calendar

import "testing"

func TestMoonPhaseLabelsCentered(t *testing.T) {
	phases := []struct{ name, icon string }{
		{"New Moon", "circle-dot"},
		{"Waxing Crescent", "moon-waxing-crescent"},
		{"First Quarter", "moon-first-quarter"},
		{"Waxing Gibbous", "moon-waxing-gibbous"},
		{"Full Moon", "moon"},
		{"Waning Gibbous", "moon-waning-gibbous"},
		{"Last Quarter", "moon-last-quarter"},
		{"Waning Crescent", "moon-waning-crescent"},
	}
	for i, phase := range phases {
		t.Run(phase.name, func(t *testing.T) {
			// A 32-day cycle makes every half-sector boundary an exact day.
			moon := Moon{CycleDays: 32}
			for _, day := range []int{i*4 - 2, i * 4, i*4 + 1} {
				if got := moon.MoonPhaseName(day); got != phase.name {
					t.Errorf("day %d name = %q, want %q", day, got, phase.name)
				}
				if got := moon.MoonPhaseIcon(day); got != phase.icon {
					t.Errorf("day %d icon = %q, want %q", day, got, phase.icon)
				}
			}
		})
	}
}

func TestMoonPhaseLabelsWrapAndOffset(t *testing.T) {
	for _, day := range []int{-33, -32, -31, -1, 0, 1, 31, 32, 33} {
		moon := Moon{CycleDays: 32}
		if moon.MoonPhaseName(day) != "New Moon" || moon.MoonPhaseIcon(day) != "circle-dot" {
			t.Errorf("day %d should be new moon", day)
		}
	}
	moon := Moon{CycleDays: 29.5, PhaseOffset: 0.75}
	if moon.MoonPhaseName(14) != "Full Moon" || moon.MoonPhaseIcon(14) != "moon" {
		t.Error("offset fractional cycle should be full at its midpoint")
	}
	for _, cycle := range []float64{0, -1} {
		moon := Moon{CycleDays: cycle}
		if moon.MoonPhaseName(100) != "New Moon" || moon.MoonPhaseIcon(100) != "circle-dot" {
			t.Error("non-positive cycle should keep its new-moon fallback")
		}
	}
}
