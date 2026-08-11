package calendar

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// moonFallbackGregorianCal is a real-life calendar shaped exactly as
// seedDefaults builds one: twelve Gregorian months, February leap-aware,
// leap-every-4 with no offset. Nothing here is a convenience fixture — every
// field below is what the create path actually writes, because the whole point
// of the fallback is that it works on the calendar the product makes.
func moonFallbackGregorianCal(id string, y, m, d int) *Calendar {
	cal := &Calendar{
		ID: id, CampaignID: "camp-1", Mode: ModeRealLife, Name: "Real world",
		CurrentYear: y, CurrentMonth: m, CurrentDay: d,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		LeapYearEvery: 4, LeapYearOffset: 0,
	}
	names := []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	lens := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	for i, n := range names {
		mo := Month{ID: i + 1, CalendarID: id, Name: n, Days: lens[i], SortOrder: i}
		if i == 1 {
			mo.LeapYearDays = 1
		}
		cal.Months = append(cal.Months, mo)
	}
	for i, n := range []string{"Sunday", "Monday", "Tuesday", "Wednesday",
		"Thursday", "Friday", "Saturday"} {
		cal.Weekdays = append(cal.Weekdays, Weekday{ID: i + 1, CalendarID: id, Name: n, SortOrder: i})
	}
	return cal
}

// TestRealMoonFallback_TheBenchsOwnPathGrowsTheRealMoon is the guard for the
// seam the 2026-08-11 report §1.2 measured: Pipeline A synthesized THE Moon and
// Pipeline B — the Bench — did not, so the surface the navigation points at
// showed nothing.
//
// It goes through EagerLoadCampaignCalendars, which IS the Bench's entry point
// (bench.go), rather than calling the helper directly: a fallback that works in
// isolation and never runs on the loader is the defect this closes.
func TestRealMoonFallback_TheBenchsOwnPathGrowsTheRealMoon(t *testing.T) {
	cal := moonFallbackGregorianCal("cal-real", 2026, 8, 11)
	svc := NewBlockService(newBlockFakeRepo(cal))

	got, err := svc.EagerLoadCampaignCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("loaded %d calendars, want 1", len(got))
	}
	if len(got[0].Moons) != 1 {
		t.Fatalf("a real-life calendar with zero calendar_moons rows came back with %d moons; "+
			"want exactly 1 — THE Moon (report §1.2)", len(got[0].Moons))
	}
	moon := got[0].Moons[0]
	if moon.Name != realMoonName {
		t.Errorf("moon name = %q, want %q — the sky band and the month grid must name "+
			"the same body identically or the operator reads it as two moons", moon.Name, realMoonName)
	}
	if moon.ID != 0 {
		t.Errorf("the synthesized moon carries id %d; it has no calendar_moons row and a "+
			"fabricated id invites a PUT against a row that does not exist", moon.ID)
	}
	if moon.CycleDays != realMoonCycleDays {
		t.Errorf("cycle = %v, want the mean synodic month %v", moon.CycleDays, realMoonCycleDays)
	}

	// THE PHASE IS THE POINT. Every day of the anchor's own month must agree
	// with gregorianMoonPhase — the ONE astronomy implementation — to the last
	// bit, because the offset is solved from it rather than approximated.
	for day := 1; day <= 31; day++ {
		abs := got[0].AbsoluteDay(2026, 8, day)
		want := gregorianMoonPhase(2026, 8, day)
		// The tolerance is float64 noise, not slack: the offset is solved
		// algebraically from the same constant, so anything above this is a
		// second implementation creeping in.
		if diff := phaseDistance(moon.MoonPhase(abs), want); diff > moonFallbackEpsilon {
			t.Errorf("2026-08-%02d: Block phase %.9f vs gregorianMoonPhase %.9f (Δ %.9f cycles); "+
				"the fallback must not be a second implementation of the same astronomy",
				day, moon.MoonPhase(abs), want, diff)
		}
	}
}

// TestRealMoonFallback_AnInWorldCalendarInventsNothing. A fantasy calendar's
// sky belongs to the GM. Handing it a body it never declared is worldbuilding
// the product was not asked for — the same refusal seedDefaults makes.
func TestRealMoonFallback_AnInWorldCalendarInventsNothing(t *testing.T) {
	fantasy := blockTenDayCal() // ModeFantasy, no moons
	if len(fantasy.Moons) != 0 {
		t.Fatalf("fixture drift: the fantasy calendar starts with %d moons", len(fantasy.Moons))
	}
	svc := NewBlockService(newBlockFakeRepo(fantasy))

	got, err := svc.EagerLoadCampaignCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Moons) != 0 {
		t.Fatalf("an in-world calendar with zero moons came back with %d; the fallback "+
			"must never invent a body a GM did not declare", len(got[0].Moons))
	}
}

// TestRealMoonFallback_AnAuthoredMoonReplacesItWhole. One authored row turns
// the default off outright — it is not merged with, or appended to, the
// operator's declaration. Same rule Pipeline A's fallback follows
// (worldstate.go: `len(out) == 0`).
func TestRealMoonFallback_AnAuthoredMoonReplacesItWhole(t *testing.T) {
	cal := moonFallbackGregorianCal("cal-real", 2026, 8, 11)
	cal.Moons = []Moon{{ID: 7, CalendarID: cal.ID, Name: "Selûne", CycleDays: 30, PhaseOffset: 3}}
	svc := NewBlockService(newBlockFakeRepo(cal))

	got, err := svc.EagerLoadCampaignCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Moons) != 1 || got[0].Moons[0].Name != "Selûne" {
		t.Fatalf("authored moons = %+v; one authored row must replace the default whole, "+
			"never sit beside it", got[0].Moons)
	}
}

// TestRealMoonFallback_TheAnchorIsThePostSeamDate. Ordering guard. A real-time
// calendar's stored date is whatever day the row was last written; only
// ApplyRealTime makes it today. Anchoring the synodic offset before the seam
// runs would peg the Moon to a stale day and the error would be invisible —
// the discs would still draw, just wrong.
func TestRealMoonFallback_TheAnchorIsThePostSeamDate(t *testing.T) {
	cal := moonFallbackGregorianCal("cal-real", 2020, 1, 1) // the STALE stored date
	zone := "UTC"
	cal.TracksRealTime, cal.RealTimeZone = true, &zone
	svc := NewBlockService(newBlockFakeRepo(cal))
	svc.SetRealTimeSeam(blockSeamStub{year: 2026, month: 8, day: 11})

	got, err := svc.EagerLoadCampaignCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Moons) != 1 {
		t.Fatalf("moons = %d, want 1", len(got[0].Moons))
	}
	abs := got[0].AbsoluteDay(2026, 8, 11)
	want := gregorianMoonPhase(2026, 8, 11)
	if diff := phaseDistance(got[0].Moons[0].MoonPhase(abs), want); diff > moonFallbackEpsilon {
		t.Fatalf("the wall-clock day is off by %.9f cycles (%.3f days); the fallback must "+
			"anchor AFTER ApplyRealTime, not on the stored date", diff, diff*realMoonCycleDays)
	}
}

// TestRealMoonFallback_TheDiscsTheGridActuallyDraws closes the loop the report
// warned about: the data fix is worthless if it does not reach the cells. This
// runs the shipped geometry — the same moonDiscsForDay the Bench calls — and
// checks the disc the operator sees against the astronomy.
func TestRealMoonFallback_TheDiscsTheGridActuallyDraws(t *testing.T) {
	cal := moonFallbackGregorianCal("cal-real", 2026, 8, 11)
	svc := NewBlockService(newBlockFakeRepo(cal))
	got, err := svc.EagerLoadCampaignCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	live := &got[0]

	geo := buildMonthGeometry(live, blockMonthGeometryInput{
		MonthIndex: 7, Year: 2026, ShowMoons: true, MoonCap: 3,
	})
	if geo.MoonsDeclared != 1 {
		t.Fatalf("MoonsDeclared = %d, want 1", geo.MoonsDeclared)
	}
	seen := 0
	for _, row := range geo.Rows {
		for _, cell := range row.Cells {
			if cell.Day == 0 {
				continue
			}
			if len(cell.Moons) != 1 {
				t.Fatalf("August %d drew %d discs, want 1", cell.Day, len(cell.Moons))
			}
			seen++
			want := gregorianMoonPhase(2026, 8, cell.Day)
			wantIllum := (1 - math.Cos(2*math.Pi*want)) / 2
			if math.Abs(cell.Moons[0].Illum-wantIllum) > 1e-9 {
				t.Errorf("August %d: disc illumination %.6f, want %.6f from the real cycle",
					cell.Day, cell.Moons[0].Illum, wantIllum)
			}
		}
	}
	if seen != 31 {
		t.Fatalf("walked %d day cells, want 31", seen)
	}
}

// TestRealMoonFallback_TheAccuracyBoundIsMeasuredNotAssumed.
//
// Moon.MoonPhase steps over Calendar.AbsoluteDay, which counts with the
// calendar's own leap rule — leap-every-4, with no century correction. That is
// the JULIAN rule, so AbsoluteDay and the true Julian Day Number diverge by
// exactly one day at each non-Gregorian century boundary. MEASURED, with a
// 2026 anchor: 0 for any date from 1901 through 2099, 1.0 day at 2100,
// 2.0 days at 2200. This test states the bound so a future change that makes
// it worse is caught, and so nobody reads "exact" as unqualified.
func TestRealMoonFallback_TheAccuracyBoundIsMeasuredNotAssumed(t *testing.T) {
	cal := moonFallbackGregorianCal("cal-real", 2026, 8, 11)
	moon, ok := synthesizedRealMoon(cal)
	if !ok {
		t.Fatal("no moon synthesized")
	}
	for _, tc := range []struct {
		name       string
		y, m, d    int
		wantErrDay float64
	}{
		{"the anchor day itself", 2026, 8, 11, 0},
		{"a century of navigation back", 1926, 6, 1, 0},
		{"the 1900 boundary is behind us", 1901, 1, 1, 0},
		{"the far end of the same band", 2099, 12, 31, 0},
		{"one skipped leap day away", 2100, 6, 1, 1},
		{"two skipped leap days away", 2200, 6, 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			abs := cal.AbsoluteDay(tc.y, tc.m, tc.d)
			diff := phaseDistance(moon.MoonPhase(abs), gregorianMoonPhase(tc.y, tc.m, tc.d))
			gotDays := diff * realMoonCycleDays
			if math.Abs(gotDays-tc.wantErrDay) > 1e-6 {
				t.Errorf("%04d-%02d-%02d: error %.6f days, want %.1f",
					tc.y, tc.m, tc.d, gotDays, tc.wantErrDay)
			}
		})
	}
}

// TestRealMoonFallback_RefusesWhatItCannotPlace. A calendar with no months
// cannot resolve an absolute day, so there is no anchor and no honest offset —
// the helper declines rather than emitting a moon phased off a meaningless
// number. Same for an out-of-range stored date.
func TestRealMoonFallback_RefusesWhatItCannotPlace(t *testing.T) {
	for _, tc := range []struct {
		name string
		bend func(*Calendar)
	}{
		{"no months", func(c *Calendar) { c.Months = nil }},
		{"month index past the list", func(c *Calendar) { c.CurrentMonth = 13 }},
		{"a zero month", func(c *Calendar) { c.CurrentMonth = 0 }},
		{"a zero day", func(c *Calendar) { c.CurrentDay = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cal := moonFallbackGregorianCal("cal-real", 2026, 8, 11)
			tc.bend(cal)
			if _, ok := synthesizedRealMoon(cal); ok {
				t.Error("synthesized a moon for a calendar it cannot place a date in")
			}
		})
	}
	if _, ok := synthesizedRealMoon(nil); ok {
		t.Error("synthesized a moon for a nil calendar")
	}
}

// moonFallbackEpsilon is float64 round-off on a cycle position, nothing more.
// Two of these tests assert EQUALITY with gregorianMoonPhase; the slack must
// stay at the noise floor, because slack is where a second approximation would
// hide.
//
// MEASURED, not guessed. Moon.MoonPhase divides an absolute day of ~740,000 by
// the cycle before taking the fractional part, so its own representable
// precision is ~5.6e-12 of a cycle; gregorianMoonPhase divides a number ~30×
// smaller and is correspondingly finer. Sampling every 7th year from 1901 to
// 2099, twelve months each, the worst residual between the two is 2.089e-12
// cycles — 6.2e-11 days, 5.3 MICROSECONDS of moon phase. 1e-11 is that number
// with headroom and nothing else.
const moonFallbackEpsilon = 1e-11

// phaseDistance is the shortest distance between two cycle positions, so 0.999
// and 0.001 read as 0.002 apart rather than 0.998.
func phaseDistance(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 0.5 {
		d = 1 - d
	}
	return d
}

// --- seedDefaults: the refusal, stated as a test ----------------------------

// TestSeedDefaults_SeedsNoMoonsAndThatIsTheAnswer.
//
// The 2026-08-11 report named seedDefaults never calling SetMoons as the reason
// "every calendar in the product is born moonless", which reads like an
// omission waiting to be closed. It is not, and this test is here so the next
// hand does not close it:
//
//   - REAL-LIFE: a seeded row would switch synthesizedRealMoon OFF (it requires
//     `len(Moons) == 0`) and put an approximation where the exact Moon was.
//     Seeding is the HARMFUL option on this branch, not the neutral one.
//   - FANTASY: a seeded moon is an invented name on an invented period, in
//     somebody's sky, that they never asked for.
//
// The assertion is on the repository, because that is where a seeded row would
// have to appear regardless of which service method wrote it.
func TestSeedDefaults_SeedsNoMoonsAndThatIsTheAnswer(t *testing.T) {
	for _, mode := range []string{ModeRealLife, ModeFantasy} {
		t.Run(mode, func(t *testing.T) {
			var moonWrites int
			var months, weekdays int
			// The real-life branch ends in UpdateCalendar, which re-reads the
			// row it is about to write, so the mock has to hand one back.
			row := &Calendar{ID: "cal-new", CampaignID: "camp-1", Mode: mode, Name: "New",
				HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4}
			repo := &mockCalendarRepo{
				getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return row, nil },
				createFn: func(_ context.Context, cal *Calendar) error {
					cal.ID = row.ID
					return nil
				},
				setMoonsFn: func(_ context.Context, _ string, m []MoonInput) error {
					moonWrites += len(m)
					return nil
				},
				setMonthsFn: func(_ context.Context, _ string, m []MonthInput) error {
					months = len(m)
					return nil
				},
				setWeekdaysFn: func(_ context.Context, _ string, w []WeekdayInput) error {
					weekdays = len(w)
					return nil
				},
			}
			svc := NewCalendarService(repo)
			if _, err := svc.CreateCalendar(context.Background(), "camp-1", CreateCalendarInput{
				Mode: mode, Name: "New", CurrentYear: 1,
				HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, LeapYearEvery: 4,
			}); err != nil {
				t.Fatal(err)
			}
			if moonWrites != 0 {
				t.Errorf("a new %s calendar was seeded %d moon rows; see this test's comment "+
					"— on real-life that removes the exact Moon, on fantasy it invents a body",
					mode, moonWrites)
			}
			// The seeding that SHOULD happen still does, so this test cannot
			// pass by seedDefaults having stopped running at all.
			if months != 12 || weekdays != 7 {
				t.Errorf("seedDefaults wrote %d months / %d weekdays; the refusal above is "+
					"about moons only", months, weekdays)
			}
		})
	}
}

// TestMoonsSettingsTab_TheCopyMatchesWhatTheProductDoes.
//
// The Moons tab used to promise, in one sentence, "Phase icons display on the
// calendar." Every clause of that was doing work it could not support: the
// discs were behind a container query no real width satisfied (a day cell
// measured 83.4px against an 84px threshold on a 1280px desktop), the grid caps
// at three per day with the rest in the Almanac, and on a real-world calendar an
// EMPTY list is not "no moon" — it is THE Moon, computed. That last one is the
// operator's own complaint, and the settings page would have confirmed it.
//
// This asserts the two facts a GM needs from this panel and cannot get anywhere
// else, so the copy cannot quietly shrink back to the sentence that shipped.
func TestMoonsSettingsTab_TheCopyMatchesWhatTheProductDoes(t *testing.T) {
	render := func(cal *Calendar) string {
		t.Helper()
		var sb strings.Builder
		cc := &campaigns.CampaignContext{
			Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner, IsMember: true,
		}
		if err := moonsSettingsTab(cc, cal, "tok").Render(context.Background(), &sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}

	realWorld := render(moonFallbackGregorianCal("cal-real", 2026, 8, 11))
	if !strings.Contains(realWorld, "the Moon") ||
		!strings.Contains(realWorld, "synodic cycle") {
		t.Error("a real-world calendar's Moons tab does not say that an empty list already " +
			"shows the real Moon — which is exactly the question that brought the operator here")
	}
	if !strings.Contains(realWorld, "Adding a moon below replaces it") {
		t.Error("the tab does not warn that adding a moon replaces the computed one, which is " +
			"the one irreversible thing this panel does")
	}

	fantasy := blockTenDayCal()
	inWorld := render(fantasy)
	if strings.Contains(inWorld, "synodic cycle") {
		t.Error("an in-world calendar's Moons tab claims a real-Moon fallback it does not get")
	}

	// Both say where the discs go and that the grid has a ceiling.
	for name, html := range map[string]string{"real-world": realWorld, "in-world": inWorld} {
		if !strings.Contains(html, "Almanac") {
			t.Errorf("%s: the tab does not say where a fourth moon goes; the grid's cap of "+
				"three is only legitimate because the overflow lands somewhere", name)
		}
		if strings.Contains(html, "Phase icons display on the calendar.") {
			t.Errorf("%s: the copy has reverted to the sentence that was false at every "+
				"shipped width", name)
		}
	}
}
