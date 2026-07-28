package calendar

// block_almanac_test.go — C-CALV4-SHELF-P7 (calendar-v4 wave 2, W-E).
//
// The PRODUCER half of the Almanac register (r53). Whether the Shelf RENDERS
// what is here is asserted widget-side; whether it survives the composed
// producer→renderer seam is asserted in block_seam_test.go. This file asserts
// only what buildMonthGeometry chose to emit, which is the one thing a
// hand-written widget fixture can never see.

import (
	"math"
	"testing"
)

// blockFourMoons is the SIGNED fixture set (mockups/calendar-v4.html:1317-1324).
// Sable is the fourth body, declared past the grid's ceiling "specifically so
// the ceiling is visible in a render rather than described" (design notes
// :662-667) — it is the whole reason this register exists.
func blockFourMoons() []Moon {
	return []Moon{
		{ID: 1, CalendarID: "cal-harptos", Name: "Alder", CycleDays: 31.4, PhaseOffset: -12},
		{ID: 2, CalendarID: "cal-harptos", Name: "Umber", CycleDays: 46.5, PhaseOffset: 4.25},
		{ID: 3, CalendarID: "cal-harptos", Name: "Flint", CycleDays: 11.3, PhaseOffset: -3},
		{ID: 4, CalendarID: "cal-harptos", Name: "Sable", CycleDays: 88.2, PhaseOffset: -41},
	}
}

// TestBlockAlmanac_IsUncappedByMoonCap is [S5], signed, at the producer.
//
// It is the ONE sanctioned place a host-passed parameter is deliberately
// non-authoritative, and it is the second half of L21: the grid's three-moon
// ceiling is legitimate only because "the Almanac carries every declared body
// at full width". A register that honoured MoonCap would silently drop the
// fourth body from the only surface that was supposed to carry it — and the
// nameplate badge's restored "all of them are in the Almanac" tail would
// become a lie.
func TestBlockAlmanac_IsUncappedByMoonCap(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = blockFourMoons()

	for _, mc := range []int{0, 1, 3, 4, 99} {
		geo := buildMonthGeometry(cal, blockMonthGeometryInput{
			MonthIndex: 0, Year: 1523, ShowMoons: true, MoonCap: mc,
		})
		if len(geo.Almanac) != 4 {
			t.Fatalf("MoonCap %d: Almanac has %d lanes, want 4 — the register is UNCAPPED ([S5])",
				mc, len(geo.Almanac))
		}
		if geo.Almanac[3].Name != "Sable" {
			t.Errorf("MoonCap %d: the fourth declared body is %q, want Sable in declaration order",
				mc, geo.Almanac[3].Name)
		}
		// Drawn is the only thing MoonCap decides here, and it must agree with
		// moonDiscsForDay's own cap — the ceiling's arithmetic in ONE place.
		drawn := 0
		for _, m := range geo.Almanac {
			if m.Drawn {
				drawn++
			}
		}
		want := mc
		if mc <= 0 || mc > 4 {
			want = 4
		}
		if drawn != want {
			t.Errorf("MoonCap %d: %d lanes marked Drawn, want %d", mc, drawn, want)
		}
		// The badge's "+1" figure and the lane count must agree: MoonsDeclared
		// minus the drawn count is exactly the overflow the Almanac carries.
		if geo.MoonsDeclared-drawn != len(geo.Almanac)-drawn {
			t.Errorf("MoonCap %d: MoonsDeclared %d disagrees with the register's %d lanes",
				mc, geo.MoonsDeclared, len(geo.Almanac))
		}
	}
}

// TestBlockAlmanac_EveryNumberComesFromTheMonthsRealDayCount is §4.7 of the
// dispatch, and the acceptance line the signed footnote demands: "the
// arithmetic is printed so it can be audited — no date in the register was
// typed by hand".
//
// The mockup hardcodes a thirty-day month in four places. Three of them are
// arithmetic and are produced here; this test drives a month of a DIFFERENT
// length through the same producer and asserts every figure moved with it. A
// thirty-day literal would pass on the fixture month and fail here, which is
// the only way to catch a ported constant.
func TestBlockAlmanac_EveryNumberComesFromTheMonthsRealDayCount(t *testing.T) {
	register := func(days int) []struct {
		name  string
		lanes int
		drift float64
		turns int
	} {
		cal := blockTenDayCal()
		cal.Moons = blockFourMoons()
		for i := range cal.Months {
			cal.Months[i].Days = days
		}
		geo := buildMonthGeometry(cal, blockMonthGeometryInput{
			MonthIndex: 0, Year: 1523, ShowMoons: true, MoonCap: 3,
		})
		if geo.Days != days {
			t.Fatalf("fixture month is %d days, want %d", geo.Days, days)
		}
		out := make([]struct {
			name  string
			lanes int
			drift float64
			turns int
		}, 0, len(geo.Almanac))
		for _, m := range geo.Almanac {
			if len(m.Days) != days {
				t.Errorf("%s carries %d day entries for a %d-day month — the register is "+
					"never partially filled", m.Name, len(m.Days), days)
			}
			for i, d := range m.Days {
				if d.Day != i+1 {
					t.Fatalf("%s day entry %d has ordinal %d — the lane must be in ordinal order",
						m.Name, i, d.Day)
				}
			}
			out = append(out, struct {
				name  string
				lanes int
				drift float64
				turns int
			}{m.Name, len(m.Days), m.DriftDays, m.TurnsThisMonth})
		}
		return out
	}

	thirty := register(30)
	twenty := register(20)

	if len(thirty) != 4 || len(twenty) != 4 {
		t.Fatalf("both months must carry four lanes; got %d and %d", len(thirty), len(twenty))
	}

	// DRIFT — the mockup's `(30 % mo.period)`. Alder's period is 31.4, so a
	// thirty-day month drifts 30 and a twenty-day month drifts 20; a ported
	// literal would print 30 for both.
	if math.Abs(thirty[0].drift-30) > 1e-9 {
		t.Errorf("Alder drifts %.4f over a 30-day month, want 30", thirty[0].drift)
	}
	if math.Abs(twenty[0].drift-20) > 1e-9 {
		t.Errorf("Alder drifts %.4f over a 20-day month, want 20 — the thirty is a LITERAL "+
			"in the mockup (cv4:2101) and must not be ported", twenty[0].drift)
	}
	// Flint's period is 11.3: over 30 days it drifts 30 mod 11.3 = 7.4, over
	// 20 days 8.7. Neither is the month length, so this is the case that
	// proves the figure is real arithmetic rather than a copy of `days`.
	if math.Abs(thirty[2].drift-7.4) > 1e-9 {
		t.Errorf("Flint drifts %.4f over a 30-day month, want 7.4", thirty[2].drift)
	}
	if math.Abs(twenty[2].drift-8.7) > 1e-9 {
		t.Errorf("Flint drifts %.4f over a 20-day month, want 8.7", twenty[2].drift)
	}

	// TURNS — a shorter month cannot contain more turns of the same moon.
	for i := range thirty {
		if twenty[i].turns > thirty[i].turns {
			t.Errorf("%s has %d turns in a 20-day month and %d in a 30-day one — "+
				"TurnsThisMonth must be counted over the month's real length",
				thirty[i].name, twenty[i].turns, thirty[i].turns)
		}
	}
	// Flint (period 11.3) turns new and full roughly every 5.65 days, so a
	// thirty-day month must contain several turns. A register that never
	// detected a turn would satisfy every inequality above vacuously.
	if thirty[2].turns < 4 {
		t.Errorf("Flint records %d turns over 30 days at period 11.3 — the turn detector "+
			"is not firing, and every assertion above would pass vacuously", thirty[2].turns)
	}
}

// TestBlockAlmanac_FractionalPhaseAgreesWithTheModel holds blockMoonPhaseAt
// against Moon.MoonPhase on whole days.
//
// The widened function exists only because turn detection needs half-day
// samples (the signed turnsIn, cv4:1327-1335). If it ever stops agreeing with
// the model on whole days, the Almanac's illumination and the grid's disc —
// which comes from Moon.MoonPhase via moonDiscsForDay — would draw two
// different moons on the same night.
func TestBlockAlmanac_FractionalPhaseAgreesWithTheModel(t *testing.T) {
	for _, m := range blockFourMoons() {
		for abs := -20; abs <= 400; abs++ {
			want := m.MoonPhase(abs)
			got := blockMoonPhaseAt(m, float64(abs))
			if math.Abs(got-want) > 1e-12 {
				t.Fatalf("%s day %d: fractional phase %.15f, model %.15f", m.Name, abs, got, want)
			}
		}
	}
	// A zero/negative cycle must not divide by zero — the model refuses too.
	if got := blockMoonPhaseAt(Moon{Name: "Void", CycleDays: 0}, 12.5); got != 0 {
		t.Errorf("a zero-cycle moon has phase %v, want 0", got)
	}
}

// TestBlockAlmanac_LanesStepOffTheSameBaseDayAsTheDiscs is the seam between the
// register and the cells beside it. Both must read the same absolute day, or
// the fourth body is computed by different arithmetic from the three the grid
// drew and the Almanac quietly disagrees with the month it annotates.
func TestBlockAlmanac_LanesStepOffTheSameBaseDayAsTheDiscs(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = blockFourMoons()
	geo := buildMonthGeometry(cal, blockMonthGeometryInput{
		MonthIndex: 0, Year: 1523, ShowMoons: true, MoonCap: 3,
	})

	// Collect the grid's discs by day for the three drawn bodies.
	byDay := map[int][]float64{}
	for _, r := range geo.Rows {
		for _, c := range r.Cells {
			if c.Day > 0 {
				for _, md := range c.Moons {
					byDay[c.Day] = append(byDay[c.Day], md.Illum)
				}
			}
		}
	}
	if len(byDay) == 0 {
		t.Fatal("the fixture drew no discs — the comparison would be vacuous")
	}
	if len(geo.Almanac) < 3 {
		t.Fatalf("Almanac has %d lanes; the three drawn bodies cannot be compared", len(geo.Almanac))
	}
	for i := 0; i < 3; i++ {
		lane := geo.Almanac[i]
		for _, d := range lane.Days {
			discs := byDay[d.Day]
			if i >= len(discs) {
				t.Fatalf("day %d drew %d discs, want at least %d", d.Day, len(discs), i+1)
			}
			if math.Abs(discs[i]-d.Illum) > 1e-12 {
				t.Errorf("%s day %d: the grid's disc is %.6f lit and the Almanac says %.6f — "+
					"the two are stepping off different base days",
					lane.Name, d.Day, discs[i], d.Illum)
			}
		}
	}
}

// TestBlockAlmanac_EmptyWhenNoShelfIsReachable pins the register's own
// contract: "empty when the calendar declares no moons, or when no Shelf is
// reachable in this render". The Bench's real-world Block renders with noShelf,
// and building four registers for four Blocks that draw none is payload nobody
// reads.
func TestBlockAlmanac_EmptyWhenNoShelfIsReachable(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = blockFourMoons()

	hidden := buildMonthGeometry(cal, blockMonthGeometryInput{
		MonthIndex: 0, Year: 1523, ShowMoons: true, MoonCap: 3, ShelfHidden: true,
	})
	if len(hidden.Almanac) != 0 {
		t.Errorf("a Block whose host removed the Shelf built %d Almanac lanes", len(hidden.Almanac))
	}
	// …and the flag must travel through the projection, not just the geometry.
	got := projectBlock(BlockProjectionInput{
		Calendar: cal, Viewer: BlockViewer{UserID: "u-gm"}, MonthIndex: 0, Year: 1523,
		ShelfHidden: true, MoonCap: 3,
	})
	if len(got.Month.Almanac) != 0 {
		t.Errorf("projectBlock ignored ShelfHidden: %d lanes", len(got.Month.Almanac))
	}

	// No moons declared → no register, whatever the Shelf is doing.
	bare := blockTenDayCal()
	if geo := buildMonthGeometry(bare, blockMonthGeometryInput{
		MonthIndex: 0, Year: 1523, ShowMoons: true,
	}); len(geo.Almanac) != 0 {
		t.Errorf("a calendar declaring no moons built %d Almanac lanes", len(geo.Almanac))
	}
}

// TestBlockAlmanac_NextTurnAnchorsOnTheRenderedDay pins NextNewDay/NextFullDay.
//
// The anchor is the SERVER-RENDERED answered day — TodayDay when today falls in
// this month, day 1 otherwise — because selection inside the Block is CSS-only
// and per-render, so there is no request state to consult. The signed find is
// `t.find(x => x.d > d) || t[0]` (cv4:2110): the next turn, WRAPPING to the
// month's first, and 0 when the month contains none so the readout drops the
// segment rather than printing a zero.
func TestBlockAlmanac_NextTurnAnchorsOnTheRenderedDay(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = blockFourMoons()
	geo := buildMonthGeometry(cal, blockMonthGeometryInput{
		MonthIndex: 0, Year: 1523, ShowMoons: true, MoonCap: 3,
	})
	if geo.TodayDay != 14 {
		t.Fatalf("fixture TodayDay = %d, want 14", geo.TodayDay)
	}
	// Fail cleanly rather than panicking on an index: COMMON §3 names a bare
	// slice index used as a pin as the failure mode that PANICS on a
	// regression instead of reporting one.
	if len(geo.Almanac) != 4 {
		t.Fatalf("Almanac has %d lanes, want the four declared", len(geo.Almanac))
	}

	for _, lane := range geo.Almanac {
		var news, fulls []int
		for _, d := range lane.Days {
			switch d.Turn {
			case "new":
				news = append(news, d.Day)
			case "full":
				fulls = append(fulls, d.Day)
			}
		}
		check := func(kind string, got int, turns []int) {
			t.Helper()
			if len(turns) == 0 {
				if got != 0 {
					t.Errorf("%s: next %s is %d but the month contains none — it must be 0 "+
						"so the readout drops the segment", lane.Name, kind, got)
				}
				return
			}
			want := turns[0] // the wrap
			for _, d := range turns {
				if d >= geo.TodayDay {
					want = d
					break
				}
			}
			if got != want {
				t.Errorf("%s: next %s is %d, want %d (turns %v, anchor %d)",
					lane.Name, kind, got, want, turns, geo.TodayDay)
			}
		}
		check("new", lane.NextNewDay, news)
		check("full", lane.NextFullDay, fulls)
	}

	// Sable's period is 88.2 days: a 30-day month cannot contain both turns,
	// so at least one lane must exercise the "month contains none" branch.
	sable := geo.Almanac[3]
	if sable.NextNewDay != 0 && sable.NextFullDay != 0 {
		t.Logf("note: Sable records both turns in this month (new %d, full %d)",
			sable.NextNewDay, sable.NextFullDay)
	}
}
