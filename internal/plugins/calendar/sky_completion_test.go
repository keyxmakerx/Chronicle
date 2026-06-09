// sky_completion_test.go — C-CAL-SKY-COMPLETION. The real-life default moon
// (phase computed locally from the Gregorian synodic cycle — no API/location),
// the fantasy no-default rule, and the GM-triggerable eclipses (Part B is
// unblocked by the moon; the effect + clear path are W2).
package calendar

import "testing"

func gregMonthsForTest() []Month {
	return []Month{
		{Name: "January", Days: 31}, {Name: "February", Days: 28}, {Name: "March", Days: 31},
		{Name: "April", Days: 30}, {Name: "May", Days: 31}, {Name: "June", Days: 30},
		{Name: "July", Days: 31}, {Name: "August", Days: 31}, {Name: "September", Days: 30},
		{Name: "October", Days: 31}, {Name: "November", Days: 30}, {Name: "December", Days: 31},
	}
}

func TestGregorianMoonPhase_RealDates(t *testing.T) {
	// Real new/full moons (local synodic math, no API): 2025-06-11 ≈ full (~0.5),
	// 2025-06-25 ≈ new (~0 / ~1.0).
	if p := gregorianMoonPhase(2025, 6, 11); p < 0.45 || p > 0.60 {
		t.Errorf("2025-06-11 (full moon) phase = %.3f; want ~0.5", p)
	}
	if p := gregorianMoonPhase(2025, 6, 25); p > 0.06 && p < 0.94 {
		t.Errorf("2025-06-25 (new moon) phase = %.3f; want ~0 (near 0 or 1)", p)
	}
}

func TestRealLifeMoonDefault(t *testing.T) {
	cal := &Calendar{Mode: ModeRealLife, Months: gregMonthsForTest(), CurrentYear: 2025, CurrentMonth: 6, CurrentDay: 11}
	moons := moonSeeds(cal, 2025, 6, 11, nil)
	if len(moons) != 1 {
		t.Fatalf("a real-life calendar with no authored moons should get 1 default Moon; got %d", len(moons))
	}
	m := moons[0]
	if m.Name != "Moon" || m.BaseDesign != "moon-realistic-selene" {
		t.Errorf("default moon = %q/%q; want Moon/moon-realistic-selene", m.Name, m.BaseDesign)
	}
	if m.CyclePct < 0.45 || m.CyclePct > 0.60 {
		t.Errorf("default moon cyclePct on a full-moon date = %.3f; want ~0.5 (the real phase)", m.CyclePct)
	}
	// Phase-linked orbit so it sits at realistic times (not floating at noon).
	if m.OrbitOffset == 0 {
		t.Errorf("default moon orbit offset should track phase (0.5 - pct), got 0")
	}
}

func TestFantasyNoDefaultMoon(t *testing.T) {
	cal := &Calendar{Mode: ModeFantasy, Months: gregMonthsForTest(), CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1}
	if moons := moonSeeds(cal, 1, 1, 1, nil); len(moons) != 0 {
		t.Errorf("a fantasy calendar with no authored moons must NOT get a default moon; got %d", len(moons))
	}
}

func TestAuthoredMoonsWin(t *testing.T) {
	// A real-life calendar that DOES define moons uses them, not the default.
	cal := &Calendar{
		Mode: ModeRealLife, Months: gregMonthsForTest(), CurrentYear: 2025, CurrentMonth: 6, CurrentDay: 11,
		Moons: []Moon{{ID: 7, Name: "Custom", CycleDays: 40, BaseDesign: "moon-etched"}},
	}
	moons := moonSeeds(cal, 2025, 6, 11, nil)
	if len(moons) != 1 || moons[0].Name != "Custom" {
		t.Errorf("authored moons must replace the default; got %v", moons)
	}
}

func TestEclipsesGMTriggerable(t *testing.T) {
	// Part B: both eclipses are offered in the GM world-event picker; lunar is
	// unblocked now that a moon exists, and both clear via the W2 ClearEvents path.
	got := map[string]bool{}
	for _, c := range gmCelestialTypes() {
		got[c.ID] = true
	}
	for _, want := range []string{"eclipse-lunar", "eclipse-solar"} {
		if !got[want] {
			t.Errorf("GM world-event picker missing %q", want)
		}
	}
}
