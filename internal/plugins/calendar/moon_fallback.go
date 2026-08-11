// moon_fallback.go — THE REAL MOON, for the surfaces that read `Calendar.Moons`.
//
// WHY THIS FILE EXISTS. Chronicle had two moon systems that never met
// (2026-08-11 observations report §1.2). Pipeline A — BuildWorldStateSeed →
// moonSeeds (worldstate.go :362, :439-459) — already gives a real-life
// (Gregorian) calendar with no authored moons THE Moon, its phase computed
// locally from the real synodic cycle by gregorianMoonPhase. Pipeline B — the
// calendar Block and the v4 Bench — reads `Calendar.Moons` straight out of
// `calendar_moons` (block_repository.go :129) with no mode branch and no
// fallback, and never calls BuildWorldStateSeed. So the operator's real-world
// calendar showed no moon at all, while the product computed one correctly on a
// surface the navigation does not point at.
//
// THE ASTRONOMY IS NOT DUPLICATED. gregorianMoonPhase (worldstate.go :468) is
// still the ONE implementation; this file only reshapes its answer into the
// (CycleDays, PhaseOffset) pair `Moon.MoonPhase` steps through, so that every
// consumer of Calendar.Moons — the per-day discs, the Almanac register, the
// moon panel — gets the real Moon through the code path it already has, with no
// second astronomy and no per-surface branch.
package calendar

import "math"

// realMoonCycleDays is the mean synodic month in days — the same constant
// gregorianMoonPhase divides by. Declared here as well so the synthesized
// moon's PERIOD (which the Almanac prints and the drift arithmetic uses) is the
// same number the phase was computed from; a rounded 29.53 in one of the two
// places is how the builder wizard's "Luna" ended up 0.85 days behind the sky
// it claimed to be (see builder_presets.go).
const realMoonCycleDays = 29.530588853

// realMoonName is what the synthesized body is called. "Moon" — the real one
// has no other name, and Pipeline A's fallback (worldstate.go :443) already
// calls it that. Two surfaces naming the same object differently is a defect
// the operator would report as two moons.
const realMoonName = "Moon"

// realMoonBaseDesign / realMoonPhaseSource mirror the Pipeline A fallback's
// render hints so a campaign that shows the sky band and the month grid on one
// page draws the same body twice, not two different ones.
const (
	realMoonBaseDesign  = "moon-realistic-selene"
	realMoonPhaseSource = "css-clip"
	// realMoonColor is the neutral silver the wizard already stamps on a moon
	// it created itself (builderImportMoonSwatch). The Block's discs are
	// achromatic by design — the governing render draws three achromatic phase
	// discs — so this only matters to surfaces that read Moon.Color.
	realMoonColor = "#c0c0c0"
)

// synthesizedRealMoon returns THE Moon for a real-life calendar that has no
// authored moons, and reports whether one was produced.
//
// THE THREE CONDITIONS, and each is a refusal to invent:
//
//   - `cal.Mode == ModeRealLife`. A fantasy calendar's sky is the GM's to
//     write. Handing it a body it did not declare would be worldbuilding this
//     product did not ask permission for, and it is the same refusal
//     seedDefaults makes (service.go).
//   - `len(cal.Moons) == 0`. One authored moon replaces the default outright,
//     exactly as Pipeline A's fallback does — the operator's declaration wins,
//     and it wins whole rather than being merged with a body they did not add.
//   - `len(cal.Months) > 0`. AbsoluteDay walks the month list; with no months
//     it cannot place the anchor date and the offset below would be meaningless.
//
// THE ANCHOR IS THE CALENDAR'S OWN CURRENT DATE, and that is the load-bearing
// choice. Moon.MoonPhase steps a cycle over Calendar.AbsoluteDay, which counts
// with the calendar's OWN leap rule — for a real-life calendar, leap-every-4
// with no century correction. That is the Julian rule, not the Gregorian one,
// so AbsoluteDay and the true Julian Day Number diverge by one day at every
// non-Gregorian century boundary (measured: JDN-AbsoluteDay is 1721045 at
// 1900-01-01, 1721044 from 2000 through 2100, 1721043 at 2200-01-01). Solving
// for the offset at the calendar's current date makes the phase EXACT on the
// day the operator is looking at, and lets it degrade by that one-day-per-
// crossed-century-boundary term only as they navigate away. Anchoring at a
// fixed far epoch instead would spread the same error over the date they
// actually care about.
func synthesizedRealMoon(cal *Calendar) (Moon, bool) {
	if cal == nil || cal.Mode != ModeRealLife || len(cal.Moons) > 0 || len(cal.Months) == 0 {
		return Moon{}, false
	}
	if cal.CurrentMonth < 1 || cal.CurrentMonth > len(cal.Months) || cal.CurrentDay < 1 {
		return Moon{}, false
	}
	pct := gregorianMoonPhase(cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
	abs := cal.AbsoluteDay(cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)

	// MoonPhase(abs) is frac((abs + offset) / cycle), so an offset of
	// pct*cycle - abs makes that frac exactly pct. Reduced into [0, cycle) so
	// the stored-looking value is the same one a human would type.
	offset := math.Mod(pct*realMoonCycleDays-float64(abs), realMoonCycleDays)
	if offset < 0 {
		offset += realMoonCycleDays
	}

	return Moon{
		// ID stays 0: this body has no `calendar_moons` row and must not look
		// like one. Nothing in the Block keys on a moon's id (moonDiscsForDay
		// and blockAlmanacRegister both read name + cycle + offset only), and a
		// fabricated id would be an invitation to PUT against it.
		ID:          0,
		CalendarID:  cal.ID,
		Name:        realMoonName,
		CycleDays:   realMoonCycleDays,
		PhaseOffset: offset,
		Color:       realMoonColor,
		BaseDesign:  realMoonBaseDesign,
		PhaseSource: realMoonPhaseSource,
		Size:        1,
		OrbitSpeed:  1,
	}, true
}

// applyRealMoonFallback appends the synthesized Moon to a calendar that
// qualifies, and is a no-op for every calendar that does not. It exists so the
// two hydration sites (the Block spine's batched loader and the wizard's
// preview calendar) cannot drift on the condition.
func applyRealMoonFallback(cal *Calendar) {
	if m, ok := synthesizedRealMoon(cal); ok {
		cal.Moons = append(cal.Moons, m)
	}
}
