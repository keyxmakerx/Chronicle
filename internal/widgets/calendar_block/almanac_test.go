package calendar_block

// almanac_test.go — Zone D's celestial panel, RENDERER side.
//
// Seam discipline again: whether the PRODUCER fills MonthGeometry.Almanac
// uncapped is asserted in package calendar (block_almanac_test.go and the seam
// suite). What is asserted here is what this package draws GIVEN a register —
// including the thing L21 actually depends on, which is that the register's
// LAST lane reaches the DOM as readily as its first.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fxAlmanac is the signed four-moon fixture as a REGISTER: Sable is declared
// past the grid's ceiling, "specifically so the ceiling is visible in a render
// rather than described" (design notes :662-667).
//
// The numbers are hand-written on purpose — this is the widget package and it
// may not compute geometry. Their agreement with the real producer is the seam
// suite's claim, not this file's.
func fxAlmanac(t *testing.T, gm bool) BlockData {
	t.Helper()
	d := fxShelf(t, gm)
	d.Month.MoonsDeclared = 4

	mk := func(name string, period float64, drawn bool, turns int, drift float64,
		newDay, fullDay int, illumAt func(int) float64, phase string) AlmanacMoon {
		m := AlmanacMoon{
			Name: name, PeriodDays: period, Drawn: drawn,
			TurnsThisMonth: turns, DriftDays: drift,
			NextNewDay: newDay, NextFullDay: fullDay,
		}
		for day := 1; day <= d.Month.Days; day++ {
			a := AlmanacDay{Day: day, Illum: illumAt(day), Phase: phase}
			switch day {
			case newDay:
				a.Turn = "new"
			case fullDay:
				a.Turn = "full"
			}
			m.Days = append(m.Days, a)
		}
		return m
	}
	bright := func(int) float64 { return .68 }
	dark := func(int) float64 { return .04 }

	d.Month.Almanac = []AlmanacMoon{
		mk("Alder", 31.4, true, 2, 30, 3, 19, bright, "waxing gibbous"),
		mk("Umber", 46.5, true, 1, 30, 0, 11, bright, "waning gibbous"),
		// Flint's cycle divides the month exactly: it is the body that KEEPS
		// the month, and the only way the derived Sky line can be exercised.
		mk("Flint", 10, true, 5, 0, 7, 12, dark, "new"),
		mk("Sable", 88.2, false, 0, 30, 0, 0, dark, "waning crescent"),
	}
	return d
}

func almZone(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `data-spane="almanac"`)
	if i < 0 {
		t.Fatal("no Almanac panel in the render")
	}
	return body[i:]
}

// TestAlmanac_CarriesEveryDeclaredBodyInAllThreeSubTabs is L21's second half,
// and the reason this slice exists.
//
// The grid caps at moonCap so "the grid can never grow with the fiction". That
// ceiling is legitimate ONLY because "the Almanac carries every declared body at
// full width" — so a fourth declared moon that appears in two panels and not
// the third is the same silent drop the ceiling was supposed to be safe from.
func TestAlmanac_CarriesEveryDeclaredBodyInAllThreeSubTabs(t *testing.T) {
	z := almZone(t, render(t, fxAlmanac(t, true)))

	panes := map[string]string{}
	for _, key := range []string{"tonight", "month", "moons"} {
		i := strings.Index(z, `data-alm="`+key+`"`)
		if i < 0 {
			t.Fatalf("the Almanac has no %s sub-panel", key)
		}
		panes[key] = z[i:]
		if j := strings.Index(panes[key][1:], `data-alm="`); j >= 0 {
			panes[key] = panes[key][:j+1]
		}
	}
	for _, name := range []string{"Alder", "Umber", "Flint", "Sable"} {
		for key, pane := range panes {
			if !strings.Contains(pane, name) {
				t.Errorf("%s is declared and is absent from the %s panel — the grid's ceiling "+
					"is only legitimate because the Almanac carries every body at full width",
					name, key)
			}
		}
	}
	// The overflow body is stated as one, in words, in the panel that prints
	// its arithmetic — the only place the person configuring a fourth moon is
	// told where it went.
	if !strings.Contains(panes["moons"], "past the ceiling the grid draws") {
		t.Error("the Moons panel must say which bodies the grid does not draw; MonthGeometry" +
			".Almanac[].Drawn is the ONE place the renderer learns it")
	}
	if strings.Count(panes["moons"], "past the ceiling the grid draws") != 1 {
		t.Error("exactly one of the four fixture bodies is past the ceiling")
	}
}

// TestAlmanac_IsGatedOnTheRegisterAndTheMoonsLayer.
//
// The wave-2 reduction of `SKY_ON() && m.moons`: the moons layer is enabled AND
// the calendar declares at least one moon. Both halves gate the TAB, not just
// the default — a tab whose panel has nothing to draw is an inert control, and
// WG-spec V18 is explicit that a title explaining one is not honesty.
func TestAlmanac_IsGatedOnTheRegisterAndTheMoonsLayer(t *testing.T) {
	on := render(t, fxAlmanac(t, true))
	mustContain(t, on, `data-shelf-pick="almanac"`, "a calendar with moons gets the tab")
	mustContain(t, on, `data-spane="almanac"`, "and its panel")

	// The moons layer off: the grid draws no discs, so an Almanac tab would
	// point at a surface the viewer switched off.
	noLayer := fxAlmanac(t, true)
	noLayer.Layers.Enabled = []string{"eras", "weeknums", "ledger", "shelf"}
	nl := render(t, noLayer)
	mustNotContain(t, nl, `data-shelf-pick="almanac"`, "the moons layer gates the Almanac tab")
	mustNotContain(t, nl, `data-spane="almanac"`, "and its panel")
	mustNotContain(t, nl, `class="seg alm"`, "and its sub-tab segment")
	mustContain(t, nl, `data-zone="shelf"`, "the Shelf itself still docks")
	mustContain(t, nl, `data-spane="upcoming"`, "and falls back to Upcoming")

	// No moons declared: the producer emits an empty register.
	noMoons := fxAlmanac(t, true)
	noMoons.Month.Almanac = nil
	noMoons.Month.MoonsDeclared = 0
	nm := render(t, noMoons)
	mustNotContain(t, nm, `data-shelf-pick="almanac"`, "a calendar with no moons gets no Almanac")
	mustContain(t, nm, `data-spane="upcoming"`, "and lands on Upcoming instead")
}

// TestAlmanac_IsTheDefaultTabWhenItExists — the signed default
// (cv4:1777-1783): the Block's celestial surface leads when there is one.
func TestAlmanac_IsTheDefaultTabWhenItExists(t *testing.T) {
	d := fxAlmanac(t, true)
	if got := shelfDefaultTab(d); got != shelfTabAlmanac {
		t.Errorf("default tab = %q, want the Almanac when the sky is on", got)
	}
	body := render(t, d)
	checkedOn := ""
	for _, frag := range strings.Split(body, "<input ") {
		if strings.Contains(frag, `class="shelfpick"`) && strings.Contains(frag, " checked") {
			m := regexp.MustCompile(`data-shelf-pick="([a-z]+)"`).FindStringSubmatch(frag)
			if m != nil {
				checkedOn = m[1]
			}
		}
	}
	if checkedOn != shelfTabAlmanac {
		t.Errorf("the server pressed %q, want the Almanac", checkedOn)
	}

	bare := fxAlmanac(t, true)
	bare.Month.Almanac = nil
	if got := shelfDefaultTab(bare); got != shelfTabUpcoming {
		t.Errorf("default tab with no register = %q, want Upcoming", got)
	}
}

// TestAlmanac_SubTabsAreCSSOnlyAndPickSuffixed is [S7] plus guard B3.
//
// `data-alm-pick`, never `data-moonstyle`: moonstyle is an <html> state-marker
// noun, and an interactive control that reused one made every click on the
// mockup re-navigate, twice. Tonight is the signed default.
func TestAlmanac_SubTabsAreCSSOnlyAndPickSuffixed(t *testing.T) {
	d := fxAlmanac(t, true)
	z := almZone(t, render(t, d))
	body := render(t, d)

	mustNotContain(t, body, "data-moonstyle", "guard B3: no control reuses an <html> state marker")
	for _, key := range []string{almTabTonight, almTabMonth, almTabMoons} {
		mustContain(t, body, `data-alm-pick="`+key+`"`, "the "+key+" sub-tab must exist")
		id := almPickInputID(d, key)
		mustContain(t, body, `id="`+id+`"`, "with its own input")
		mustContain(t, body, `for="`+id+`"`, "and a label bound to it")
	}
	checked := 0
	tonight := false
	for _, frag := range strings.Split(body, "<input ") {
		if strings.Contains(frag, `class="almpick"`) && strings.Contains(frag, " checked") {
			checked++
			tonight = strings.Contains(frag, `data-alm-pick="tonight"`)
		}
	}
	if checked != 1 || !tonight {
		t.Errorf("%d sub-tabs checked (tonight=%v); Tonight is the signed default and exactly "+
			"one panel may show", checked, tonight)
	}
	if strings.Contains(z, "<script") || strings.Contains(z, "onclick") {
		t.Error("the Almanac must ship no script and no inline handler")
	}
	// The `.seg` primitive is CONSUMED, not redefined (WG spec :824).
	mustContain(t, body, `class="seg alm"`, "the sub-tab control is the shared .seg primitive")
}

// TestAlmanac_MonthLaneIsOneColumnPerRealDayAndEveryCellIsKeyed.
//
// Guard B4 inside the lane, and §4.7's fix: the lane is one column per DAY OF
// THE MONTH, so a 30-day fixture draws thirty cells per lane and a 20-day one
// draws twenty. The mockup hardcodes thirty in the markup AND the footnote.
func TestAlmanac_MonthLaneIsOneColumnPerRealDayAndEveryCellIsKeyed(t *testing.T) {
	for _, days := range []int{30, 20} {
		d := fxAlmanac(t, true)
		d.Month.Days = days
		for i := range d.Month.Almanac {
			if len(d.Month.Almanac[i].Days) > days {
				d.Month.Almanac[i].Days = d.Month.Almanac[i].Days[:days]
			}
		}
		z := almZone(t, render(t, d))

		lane := regexp.MustCompile(`(?s)<div class="alane">.*?</div>`).FindString(z)
		if lane == "" {
			t.Fatal("no lane in the Month panel")
		}
		if n := strings.Count(lane, `class="c"`); n != days {
			t.Errorf("%d-day month: the lane drew %d cells, want one per day", days, n)
		}
		// Every cell is a DATED surface and carries its ANSWER key.
		keyed := regexp.MustCompile(`class="c" data-day="` +
			regexp.QuoteMeta(d.CalendarSlug) + `-\d+"`)
		if n := len(keyed.FindAllString(lane, -1)); n != days {
			t.Errorf("%d-day month: %d of %d lane cells carry data-day (guard B4)", days, n, days)
		}
		// The footnote states the real count, not the mockup's thirty.
		mustContain(t, z, "one lane per moon, "+strconv.Itoa(days)+" columns",
			"the signed footnote's day count is derived, not the literal 'thirty'")
		// The ruler keeps one box per day, or it drifts out of step with the
		// lanes beneath it.
		head := regexp.MustCompile(`(?s)<div class="almhead">.*?</div>`).FindString(z)
		if n := strings.Count(head, "<b>"); n != days+1 {
			t.Errorf("%d-day month: the ruler drew %d boxes, want %d (one per day plus the "+
				"name column)", days, n, days+1)
		}
	}
}

// TestAlmanac_PartnerCellsBorrowTheDatumsHueAndNeverAccent.
//
// A partner without its own identity borrows the DATUM's hue, NEVER --accent
// (canon A7, cv4:2720-2721) — the moment accent means "related to what you're
// pointing at" it stops meaning anything else. The lane cell carries
// data-axis="var(--text-primary)" for exactly that.
func TestAlmanac_PartnerCellsBorrowTheDatumsHueAndNeverAccent(t *testing.T) {
	z := almZone(t, render(t, fxAlmanac(t, true)))
	lane := regexp.MustCompile(`(?s)<div class="alane">.*?</div>`).FindString(z)

	if !strings.Contains(lane, `data-axis="var(--text-primary)"`) {
		t.Error("the lane cell must name the datum hue it answers in")
	}
	if strings.Contains(z, "--accent") {
		t.Error("the Almanac named --accent on a celestial value; the sky is ACHROMATIC and " +
			"--accent is chrome (REVIEW.md:126-136)")
	}
	if strings.Contains(z, "--axis:") {
		t.Error("the Almanac borrowed the EVENT colour axis for a moon; the sky may never " +
			"borrow it (the §SKY laws)")
	}
}

// TestAlmanac_PrintsNoEpithetAndNoFixtureContent is [S6] plus §4.7's fourth
// literal.
//
// calendar.Moon has no epithet column. The mockup's "the great pale moon" and
// its `d === 14 ? ' — meteors 22:00–02:00'` are both fabricated fixture text,
// and this slice prints neither rather than migrating a dead column or
// inventing a celestial event the register cannot carry.
func TestAlmanac_PrintsNoEpithetAndNoFixtureContent(t *testing.T) {
	z := almZone(t, render(t, fxAlmanac(t, true)))
	for _, fabricated := range []string{
		"the great pale moon", "the dark moon", "small and fast", "the far wanderer",
		"meteors", "22:00", "thirty columns", "three moons declared",
	} {
		if strings.Contains(z, fabricated) {
			t.Errorf("the Almanac printed the mockup's fixture text %q", fabricated)
		}
	}
}

// TestAlmanac_TonightDerivesEveryNumberItPrints.
//
// The Sky line's two halves are the mockup's two literals (cv4:2112): the
// declared count comes from MonthGeometry.MoonsDeclared (r51) and "keeps the
// month" from each moon's own drift against the month's real length.
func TestAlmanac_TonightDerivesEveryNumberItPrints(t *testing.T) {
	d := fxAlmanac(t, true)
	z := almZone(t, render(t, d))

	mustContain(t, z, "4 moons declared · Flint keeps the month",
		"the Sky line states the DECLARED total and the body whose cycle divides this month")

	// No keeper: every drift is non-zero.
	none := fxAlmanac(t, true)
	for i := range none.Month.Almanac {
		none.Month.Almanac[i].DriftDays = 7.4
	}
	mustContain(t, almZone(t, render(t, none)), "4 moons declared · none keeps the month",
		"the signed 'none keeps the month' is a DERIVED claim, not a constant")

	// The declared total follows MoonsDeclared, not the lane count — a register
	// truncated by a bug must not silently restate a smaller total.
	five := fxAlmanac(t, true)
	five.Month.MoonsDeclared = 5
	mustContain(t, almZone(t, render(t, five)), "5 moons declared",
		"the count is MoonsDeclared; the lane list cannot supply it")

	// The per-moon row: percentage, phase word, and the next turn of either
	// kind at or after the anchor.
	mustContain(t, z, "68% waxing gibbous · next full 19",
		"a moon's Tonight row is its illumination, its phase word and its next turn")

	// The lit line's threshold is illum > .25 — the two dark fixture bodies
	// must not appear in it.
	mustContain(t, z, "Alder and Umber up",
		"only bodies over the signed .25 threshold are 'up'")
	dark := fxAlmanac(t, true)
	for i := range dark.Month.Almanac {
		for j := range dark.Month.Almanac[i].Days {
			dark.Month.Almanac[i].Days[j].Illum = .1
		}
	}
	mustContain(t, almZone(t, render(t, dark)), "no moon meaningfully lit",
		"below the threshold the panel says so rather than listing nothing")
}

// TestAlmanac_AnchorsOnTheServerRenderedDay pins the divergence rather than
// leaving it undocumented: the Tonight readout is written for TodayDay, and for
// day 1 when the rendered month does not contain today.
//
// The signed anchor is `S.sel || m.today`. The selection half is CSS-only and
// per-render, and retargeting would mean emitting the whole readout once per
// selectable day and revealing it through W-B's generated ladder — which is
// W-B's file and a payload this slice will not spend. Booked, not approximated;
// the lane's data-day keys keep the door open.
func TestAlmanac_AnchorsOnTheServerRenderedDay(t *testing.T) {
	d := fxAlmanac(t, true)
	if got := almanacAnchorDay(d); got != d.Month.TodayDay {
		t.Errorf("anchor = %d, want TodayDay %d", got, d.Month.TodayDay)
	}
	mustContain(t, almZone(t, render(t, d)), ">Day "+strconv.Itoa(d.Month.TodayDay)+"<",
		"the Tonight panel names the day it is written for")

	away := fxAlmanac(t, true)
	away.Month.TodayDay = 0
	if got := almanacAnchorDay(away); got != 1 {
		t.Errorf("anchor with today outside the month = %d, want 1", got)
	}
	mustContain(t, almZone(t, render(t, away)), ">Day 1<",
		"a month that does not contain today still names the day it read")
}

// TestAlmanac_MoonsPanelPrintsAuditableArithmetic — the panel whose own signed
// footnote is a promise: "the arithmetic is printed so it can be audited — no
// date in the register was typed by hand".
func TestAlmanac_MoonsPanelPrintsAuditableArithmetic(t *testing.T) {
	z := almZone(t, render(t, fxAlmanac(t, true)))

	mustContain(t, z, "period 31.4 days · 2 turns this month · drifts 30.0 days per month",
		"every figure in the Moons row is the register's own, to one decimal")
	mustContain(t, z, "no date in the register was typed by hand",
		"the signed footnote is the promise the row above it keeps")
	// Periods are decimals: rounding them to whole days would make two
	// different moons print the same period.
	mustContain(t, z, "period 46.5 days", "a decimal cycle length keeps its decimal")
}

// TestAlmanac_DrawsNoNodeBracket. AlmanacDay.Node is false everywhere because
// calendar.Moon has no orbital node column — the mockup's nodeWindow() keys on
// a hardcoded flag on its second fixture moon. An interval with nothing behind
// it is the fog horizon's defect one surface over.
func TestAlmanac_DrawsNoNodeBracket(t *testing.T) {
	z := almZone(t, render(t, fxAlmanac(t, true)))
	mustNotContain(t, z, `class="abr"`,
		"the node-window bracket has no backend; the CSS ships as vocabulary and nothing emits it")
	mustNotContain(t, z, "shadow ", "and no node-window title")
}
