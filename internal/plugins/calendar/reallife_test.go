// reallife_test.go covers the Real-world calendar's fixed Gregorian
// definition: true (4/100/400) month lengths, weekday alignment against
// known real-world dates, and the real Moon's phase anchor — plus
// GregorianImportResult's overall shape.
package calendar

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// TestDaysInGregorianMonth_LeapYearRule is the direct test of #the
// documented reason MonthDays bypasses the naive leap_year_every field for
// a TracksRealTime calendar: 2000 and 2024 are leap years (%4==0, and 2000
// also clears the %100/%400 exception), 2100 and 2025 are not.
func TestDaysInGregorianMonth_LeapYearRule(t *testing.T) {
	tests := []struct {
		year int
		want int
	}{
		{2024, 29}, // %4==0, not a century year
		{2025, 28}, // not %4==0
		{2100, 28}, // %4==0 and %100==0, but not %400==0 — NOT a leap year
		{2000, 29}, // %4==0, %100==0, AND %400==0 — a leap year
	}
	for _, tt := range tests {
		// daysInGregorianMonth's parameter is the natural 1-based month
		// number here (February=2) — see MonthDays' own call,
		// daysInGregorianMonth(year, monthIdx+1), for why.
		if got := daysInGregorianMonth(tt.year, 2); got != tt.want {
			t.Errorf("daysInGregorianMonth(%d, February) = %d, want %d", tt.year, got, tt.want)
		}
	}
}

// TestCalendar_MonthDays_UsesTrueGregorianRule confirms the same thing
// through the actual model API a real-time calendar's callers use
// (Calendar.MonthDays), not just the bare helper.
func TestCalendar_MonthDays_UsesTrueGregorianRule(t *testing.T) {
	cal := &Calendar{Mode: ModeRealLife, TracksRealTime: true, Months: monthInputsToMonths(gregorianMonths())}
	tests := []struct {
		year int
		want int
	}{
		{2024, 29}, {2025, 28}, {2100, 28}, {2000, 29},
	}
	for _, tt := range tests {
		// February is index 1 (0-based) in the Gregorian month list.
		if got := cal.MonthDays(1, tt.year); got != tt.want {
			t.Errorf("MonthDays(February, %d) = %d, want %d", tt.year, got, tt.want)
		}
	}
	// A non-leap month is unaffected by any of this — pinned so the table
	// above can't pass by accident (e.g. every month returning 28).
	if got := cal.MonthDays(0, 2024); got != 31 { // January
		t.Errorf("MonthDays(January, 2024) = %d, want 31", got)
	}
}

// TestCalendar_TracksRealTimeIsWireExposed pins that TracksRealTime is
// exposed on the JSON wire (not tagged json:"-"), so a browser-side mirror
// of UsesRealTime can key on both Mode and this flag, matching
// Calendar.UsesRealTime — which requires both — for a manual real-world
// calendar.
func TestCalendar_TracksRealTimeIsWireExposed(t *testing.T) {
	tests := []struct {
		name string
		val  bool
	}{
		{"tracking on", true},
		{"tracking off (manual)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := &Calendar{Mode: ModeRealLife, TracksRealTime: tt.val}
			b, err := json.Marshal(cal)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var out map[string]any
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			got, ok := out["tracks_real_time"]
			if !ok {
				t.Fatalf("tracks_real_time missing from JSON: %s", b)
			}
			if got != tt.val {
				t.Errorf("tracks_real_time = %v, want %v", got, tt.val)
			}
		})
	}
}

// monthInputsToMonths adapts MonthInput (the wizard's wire shape) to Month
// (the loaded-calendar shape) for a test Calendar literal — same fields,
// different structs, see MonthInput's own doc comment.
func monthInputsToMonths(inputs []MonthInput) []Month {
	out := make([]Month, len(inputs))
	for i, m := range inputs {
		out[i] = Month{Name: m.Name, Days: m.Days, SortOrder: m.SortOrder, IsIntercalary: m.IsIntercalary, LeapYearDays: m.LeapYearDays}
	}
	return out
}

// TestGregorianWeekdays_AlignToRealDates is the direct test of the ordering
// comment on gregorianWeekdayNames: 2026-09-27 is a real Sunday and
// 2000-01-01 is a real Saturday (both given), so a TracksRealTime
// calendar's WeekdayIndex — which for a real-time calendar is plain
// gregorianJDN(...) % len(Weekdays), see WeekdayIndex's own doc comment —
// must land on those weekdays' slots in the Monday-first list this package
// stores.
func TestGregorianWeekdays_AlignToRealDates(t *testing.T) {
	cal := &Calendar{
		Mode:           ModeRealLife,
		TracksRealTime: true,
		Months:         monthInputsToMonths(gregorianMonths()),
		Weekdays:       weekdayInputsToWeekdays(gregorianWeekdays()),
	}
	nameAt := func(idx int) string { return cal.Weekdays[idx].Name }

	if idx := cal.WeekdayIndex(2026, 9, 27); nameAt(idx) != "Sunday" {
		t.Errorf("2026-09-27: WeekdayIndex = %d (%s), want Sunday", idx, nameAt(idx))
	}
	if idx := cal.WeekdayIndex(2000, 1, 1); nameAt(idx) != "Saturday" {
		t.Errorf("2000-01-01: WeekdayIndex = %d (%s), want Saturday", idx, nameAt(idx))
	}
	// A third, independently-known date: 2026-09-28 is the Monday right
	// after the Sunday above.
	if idx := cal.WeekdayIndex(2026, 9, 28); nameAt(idx) != "Monday" {
		t.Errorf("2026-09-28: WeekdayIndex = %d (%s), want Monday", idx, nameAt(idx))
	}
	// Saturday and Sunday (and only those two) are rest days.
	for i, w := range cal.Weekdays {
		wantRest := w.Name == "Saturday" || w.Name == "Sunday"
		if w.IsRestDay != wantRest {
			t.Errorf("weekday %d (%s): IsRestDay = %v, want %v", i, w.Name, w.IsRestDay, wantRest)
		}
	}
}

// weekdayInputsToWeekdays adapts WeekdayInput to Weekday, same reasoning as
// monthInputsToMonths above.
func weekdayInputsToWeekdays(inputs []WeekdayInput) []Weekday {
	out := make([]Weekday, len(inputs))
	for i, w := range inputs {
		out[i] = Weekday{Name: w.Name, SortOrder: w.SortOrder, IsRestDay: w.IsRestDay}
	}
	return out
}

// TestGregorianMoon_AlignsToRealNewMoon confirms the real Moon's
// PhaseOffset lands on 0 (new moon) exactly at its reference epoch
// (2000-01-06) and on ~0.5 (full moon) half a synodic month later — the
// "aligned to real new moons" property GregorianImportResult's own doc
// comment claims.
func TestGregorianMoon_AlignsToRealNewMoon(t *testing.T) {
	moon := gregorianMoon()
	epochDay := gregorianJDN(2000, 1, 6)

	phase := moon.MoonPhaseAt(epochDay)
	if phase > 0.01 && phase < 0.99 {
		t.Errorf("phase at the reference new moon = %v, want ~0 (allowing for the mean-synodic approximation)", phase)
	}

	halfCycle := int(math.Round(realMoonSynodicDays / 2))
	fullPhase := moon.MoonPhaseAt(epochDay + halfCycle)
	if math.Abs(fullPhase-0.5) > 0.05 {
		t.Errorf("phase half a synodic month later = %v, want ~0.5 (full moon)", fullPhase)
	}
}

// MoonPhaseAt is a small test-only shim: MoonInput (the wizard's wire
// shape) has no MoonPhase method of its own (only the loaded Moon struct
// does — see Moon.MoonPhase), so this mirrors that formula exactly rather
// than constructing a full Moon just to call it.
func (m MoonInput) MoonPhaseAt(absoluteDay int) float64 {
	mm := &Moon{CycleDays: m.CycleDays, PhaseOffset: m.PhaseOffset}
	return mm.MoonPhase(absoluteDay)
}

// TestGregorianImportResult_Shape pins GregorianImportResult's overall
// contract: real months/weekdays, exactly one (real) moon, no seasons/eras,
// reallife mode, and an honest warning about the moon-phase approximation
// (see gregorianMoon's own doc comment on why it's a mean, not a perturbed,
// synodic month).
func TestGregorianImportResult_Shape(t *testing.T) {
	ir, err := GregorianImportResult()
	if err != nil {
		t.Fatalf("GregorianImportResult: %v", err)
	}
	if ir.CalendarName != "Real world" {
		t.Errorf("CalendarName = %q, want \"Real world\"", ir.CalendarName)
	}
	if ir.Settings.Mode != ModeRealLife {
		t.Errorf("Settings.Mode = %q, want %q", ir.Settings.Mode, ModeRealLife)
	}
	if len(ir.Months) != 12 {
		t.Fatalf("got %d months, want 12", len(ir.Months))
	}
	wantMonths := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	for i, want := range wantMonths {
		if ir.Months[i].Name != want {
			t.Errorf("month %d = %q, want %q", i, ir.Months[i].Name, want)
		}
	}
	if ir.Months[1].LeapYearDays != 1 {
		t.Errorf("February.LeapYearDays = %d, want 1", ir.Months[1].LeapYearDays)
	}
	if len(ir.Weekdays) != 7 || ir.Weekdays[0].Name != "Monday" || ir.Weekdays[6].Name != "Sunday" {
		t.Fatalf("Weekdays = %+v, want Monday..Sunday", ir.Weekdays)
	}
	if len(ir.Moons) != 1 {
		t.Fatalf("got %d moons, want exactly 1 (the real Moon)", len(ir.Moons))
	}
	// Seasons default to the four northern-hemisphere ones
	// (defaultRealLifeSeasons) — see GregorianImportResult's own doc
	// comment on why this is a starting guess, not a geography check.
	if len(ir.Seasons) != 4 {
		t.Errorf("expected the 4 default northern seasons, got %d: %+v", len(ir.Seasons), ir.Seasons)
	}
	if ir.Settings.Hemisphere == nil || *ir.Settings.Hemisphere != HemisphereNorth {
		t.Errorf("Settings.Hemisphere = %v, want %q (paired with the seeded northern seasons)", ir.Settings.Hemisphere, HemisphereNorth)
	}
	if len(ir.Eras) != 0 {
		t.Errorf("expected no eras from the fixed Gregorian definition, got %d", len(ir.Eras))
	}
	foundMoonWarning, foundHemisphereWarning := false, false
	for _, w := range ir.Warnings {
		if strings.Contains(w, "synodic") {
			foundMoonWarning = true
		}
		if strings.Contains(w, "hemisphere") {
			foundHemisphereWarning = true
		}
	}
	if !foundMoonWarning {
		t.Errorf("expected a warning about the moon-phase approximation, got %v", ir.Warnings)
	}
	if !foundHemisphereWarning {
		t.Errorf("expected a warning that the hemisphere/seasons default is a guess, got %v", ir.Warnings)
	}
	// clampCalendarStructure already ran (GregorianImportResult calls it) —
	// confirm it didn't have to clamp anything ELSE on this fixed,
	// well-formed definition (which would be a bug in the definition
	// itself), by pinning the exact warning count.
	if len(ir.Warnings) != 2 {
		t.Errorf("expected exactly the moon + hemisphere warnings, got %v", ir.Warnings)
	}
}

// TestGregorianImportResult_IsNotAPreset confirms the real-world calendar
// is never offered next to the shipped presets — it has its own dedicated
// wizard step (WizardRealWorldReview) precisely because a fantasy preset
// and the fixed Gregorian structure serve different purposes (see
// reallife.go's package doc).
func TestGregorianImportResult_IsNotAPreset(t *testing.T) {
	names, err := PresetNames()
	if err != nil {
		t.Fatalf("PresetNames: %v", err)
	}
	for _, name := range names {
		if name == "reallife" || name == "real-world" || name == "gregorian" {
			t.Errorf("the real-world calendar must not be a preset name, found %q among %v", name, names)
		}
	}
}

// TestTodayInZone pins the date a real-world calendar starts on: the zone's
// own wall-clock day, and a refusal for a zone that does not exist.
func TestTodayInZone(t *testing.T) {
	svc := newTestCalendarService(&fakeCalendarRepo{}, nil, nil, nil)
	if _, _, _, err := svc.TodayInZone("Not/AZone"); err == nil {
		t.Error("an unknown zone must be refused")
	}
	loc, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Skip("zone database unavailable:", err)
	}
	before := time.Now().In(loc)
	y, m, d, err := svc.TodayInZone("Pacific/Kiritimati")
	after := time.Now().In(loc)
	if err != nil {
		t.Fatalf("TodayInZone: %v", err)
	}
	isDay := func(ts time.Time) bool {
		yy, mm, dd := ts.Date()
		return y == yy && m == int(mm) && d == dd
	}
	if !isDay(before) && !isDay(after) {
		t.Errorf("TodayInZone = %d-%d-%d, want the zone's own date %s", y, m, d, before.Format("2006-01-02"))
	}
}

// TestRealWorldCalendar_MoonMatchesTheSky reads the real Moon the way every
// page does, through the calendar's own day counter, on dates with a known
// sky. The real Moon is anchored to the Julian Day Number, so reading it
// through AbsoluteDay (which counts no leap days here) put it about a week
// behind by 2026.
func TestRealWorldCalendar_MoonMatchesTheSky(t *testing.T) {
	mi := gregorianMoon()
	cal := &Calendar{
		Mode:           ModeRealLife,
		TracksRealTime: true,
		Months:         monthInputsToMonths(gregorianMonths()),
		Moons:          []Moon{{Name: mi.Name, CycleDays: mi.CycleDays, PhaseOffset: mi.PhaseOffset}},
	}
	tests := []struct {
		name             string
		year, month, day int
		want             string
	}{
		{"new moon", 2026, 9, 11, "New Moon"},
		{"full moon", 2026, 9, 26, "Full Moon"},
		{"two days after full", 2026, 9, 28, "Waning Gibbous"},
		{"total solar eclipse", 2024, 4, 8, "New Moon"},
		{"total lunar eclipse", 2025, 3, 14, "Full Moon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = tt.year, tt.month, tt.day
			if got := cal.Moons[0].MoonPhaseName(cal.CurrentDayIndex()); got != tt.want {
				t.Errorf("%04d-%02d-%02d reads %q, want %q", tt.year, tt.month, tt.day, got, tt.want)
			}
		})
	}

	// The preview's month grid draws the same Moon.
	cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = 2026, 9, 28
	for _, week := range buildMonthGrid(cal, 2026, 9, nil, &cal.Moons[0]) {
		for _, cell := range week {
			if !cell.Blank && cell.Day == 26 && math.Abs(cell.MoonPhase-0.5) > 0.04 {
				t.Errorf("the preview grid draws 2026-09-26 at phase %.3f, want the full moon (0.5)", cell.MoonPhase)
			}
		}
	}
}

// TestFollowRealClock pins that a real-time calendar's date and time come
// from the clock in its own zone on every read, not from the stored row.
func TestFollowRealClock(t *testing.T) {
	zone := func(s string) *string { return &s }
	// 2026-10-10 03:30 UTC is still 9 October in New York.
	now := time.Date(2026, 10, 10, 3, 30, 0, 0, time.UTC)
	tests := []struct {
		name                  string
		cal                   Calendar
		y, m, d, hour, minute int
	}{
		{"real time follows its zone", Calendar{Mode: ModeRealLife, TracksRealTime: true, RealTimeZone: zone("America/New_York")}, 2026, 10, 9, 23, 30},
		{"real time in UTC", Calendar{Mode: ModeRealLife, TracksRealTime: true, RealTimeZone: zone("UTC")}, 2026, 10, 10, 3, 30},
		{"manual real-world date stays", Calendar{Mode: ModeRealLife, RealTimeZone: zone("UTC")}, 2026, 9, 20, 8, 0},
		{"fantasy calendar stays", Calendar{Mode: ModeFantasy, TracksRealTime: true, RealTimeZone: zone("UTC")}, 2026, 9, 20, 8, 0},
		{"no zone keeps the stored date", Calendar{Mode: ModeRealLife, TracksRealTime: true}, 2026, 9, 20, 8, 0},
		{"unknown zone keeps the stored date", Calendar{Mode: ModeRealLife, TracksRealTime: true, RealTimeZone: zone("Mars/Olympus")}, 2026, 9, 20, 8, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cal := tt.cal
			cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, cal.CurrentHour, cal.CurrentMinute = 2026, 9, 20, 8, 0
			cal.followRealClock(now)
			got := [5]int{cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay, cal.CurrentHour, cal.CurrentMinute}
			want := [5]int{tt.y, tt.m, tt.d, tt.hour, tt.minute}
			if got != want {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

// TestRealWorldGrid_StartsOnSunday pins that a real-world calendar's month
// grid starts its rows on Sunday while WeekdayIndex keeps Monday first.
func TestRealWorldGrid_StartsOnSunday(t *testing.T) {
	weekdays := weekdayInputsToWeekdays(gregorianWeekdays())
	months := monthInputsToMonths(gregorianMonths())
	tests := []struct {
		name      string
		cal       *Calendar
		wantFirst int
		wantLead  int // blanks before 1 October 2026, a Thursday
	}{
		{"real time", &Calendar{Mode: ModeRealLife, TracksRealTime: true, Months: months, Weekdays: weekdays}, 6, 4},
		{"fantasy keeps its first weekday", &Calendar{Mode: ModeFantasy, Months: months, Weekdays: weekdays}, 0, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cal.GridFirstWeekday(7); got != tt.wantFirst {
				t.Fatalf("GridFirstWeekday = %d, want %d", got, tt.wantFirst)
			}
			if tt.wantLead < 0 {
				return
			}
			lead := 0
			for _, c := range buildMonthGrid(tt.cal, 2026, 10, nil, nil)[0] {
				if !c.Blank {
					break
				}
				lead++
			}
			if lead != tt.wantLead {
				t.Errorf("1 October 2026 sits after %d blanks, want %d (Sun Mon Tue Wed, then Thu)", lead, tt.wantLead)
			}
		})
	}
	cal := &Calendar{Mode: ModeRealLife, TracksRealTime: true, Weekdays: weekdays}
	if idx := cal.WeekdayIndex(2026, 10, 1); weekdays[idx].Name != "Thursday" {
		t.Errorf("WeekdayIndex(2026-10-01) = %s, want Thursday", weekdays[idx].Name)
	}
}
