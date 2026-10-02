package calendar

import "testing"

// TestElvenPresetMoonsImportWithTheirReferenceDate pins that the Elven
// preset's moons carry a field the Calendaria parser actually reads
// (referenceDate): with keys the parser ignores, both moons imported with
// PhaseOffset 0 and silently started on the wrong phase.
func TestElvenPresetMoonsImportWithTheirReferenceDate(t *testing.T) {
	res, err := LoadPreset("elven")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}

	// Year 0, month 1, day N is absolute day N (45-day months, no leap), so
	// the expected offset is -N modulo the cycle length.
	tests := []struct {
		moon       string
		cycle      float64
		wantOffset float64
		newMoonDay int
	}{
		{"Sehanine", 54.2, 33.2, 21},
		{"Lira", 27.1, 21.1, 6},
	}
	for _, tt := range tests {
		t.Run(tt.moon, func(t *testing.T) {
			var got *MoonInput
			for i := range res.Moons {
				if res.Moons[i].Name == tt.moon {
					got = &res.Moons[i]
				}
			}
			if got == nil {
				t.Fatalf("moon %q missing from imported preset", tt.moon)
			}
			if got.CycleDays != tt.cycle {
				t.Errorf("CycleDays = %v, want %v", got.CycleDays, tt.cycle)
			}
			if got.PhaseOffset != tt.wantOffset {
				t.Errorf("PhaseOffset = %v, want %v", got.PhaseOffset, tt.wantOffset)
			}
			cal := &Calendar{Months: make([]Month, len(res.Months))}
			for i, m := range res.Months {
				cal.Months[i] = Month{Days: m.Days, LeapYearDays: m.LeapYearDays}
			}
			moon := Moon{CycleDays: got.CycleDays, PhaseOffset: got.PhaseOffset}
			if phase := moon.MoonPhase(cal.AbsoluteDay(0, 1, tt.newMoonDay)); phase > 0.001 && phase < 0.999 {
				t.Errorf("phase on reference date = %v, want ~0 (New Moon)", phase)
			}
		})
	}
}

// TestDwarvenPresetEveryDayHasASeason pins that no day of the Dwarven year
// falls outside both seasons. The one-day intercalary month "The Long
// Vigil" follows the last regular month, so it belongs to the season that
// ends the year (Stillrock) rather than to none.
func TestDwarvenPresetEveryDayHasASeason(t *testing.T) {
	res, err := LoadPreset("dwarven")
	if err != nil {
		t.Fatalf("LoadPreset: %v", err)
	}
	for mi, m := range res.Months {
		for d := 1; d <= m.Days; d++ {
			var in []string
			for _, si := range res.Seasons {
				s := Season{StartMonth: si.StartMonth, StartDay: si.StartDay, EndMonth: si.EndMonth, EndDay: si.EndDay}
				if s.ContainsDate(mi+1, d) {
					in = append(in, si.Name)
				}
			}
			if len(in) != 1 {
				t.Errorf("month %d (%q) day %d is in seasons %v, want exactly one", mi+1, m.Name, d, in)
			}
		}
	}
}
