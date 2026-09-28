// reallife.go builds the wizard's "Real-world calendar" tile: a fixed
// Gregorian ImportResult (real months, real weekdays, and — because it can
// be aligned to real new moons, see below — the real Moon). It is
// deliberately NOT one of the shipped presets.go/presets/*.json entries: a
// fantasy preset is a starting point the owner is expected to rename and
// reshape, while the Gregorian structure must stay exactly the real-world
// calendar (WeekdayIndex/MonthDays only compute correctly for a
// TracksRealTime calendar when the stored months/weekdays actually ARE
// January..December and Monday..Sunday — see model.go's MonthDays/
// WeekdayIndex doc comments), so it is never offered next to Harptos or
// Dwarven in the preset picker.
package calendar

import "time"

// gregorianMonthDef is one Gregorian month's fixed name/length/leap-day
// shape — day counts for a common year; February's LeapYearDays is only
// ever read by the naive leap_year_every arithmetic (validateImportCurrentDate,
// pre-creation), never by MonthDays itself, which bypasses it entirely for a
// TracksRealTime calendar in favor of the true 4/100/400 rule
// (daysInGregorianMonth) — see MonthDays' own doc comment.
type gregorianMonthDef struct {
	name         string
	days         int
	leapYearDays int
}

var gregorianMonthDefs = []gregorianMonthDef{
	{"January", 31, 0}, {"February", 28, 1}, {"March", 31, 0}, {"April", 30, 0},
	{"May", 31, 0}, {"June", 30, 0}, {"July", 31, 0}, {"August", 31, 0},
	{"September", 30, 0}, {"October", 31, 0}, {"November", 30, 0}, {"December", 31, 0},
}

// gregorianWeekdayNames is Monday-first (ISO 8601 order), which is the order
// WeekdayIndex actually needs: a TracksRealTime calendar computes its
// weekday column as gregorianJDN(...) % 7 (WeekdayIndex's UsesRealTime
// branch), and that JDN is congruent to 0 on a Monday — verified against
// 2000-01-01 (a Saturday) and 2026-09-27 (a Sunday) in reallife_test.go.
// Reordering this slice without re-deriving that alignment would silently
// shift every real-world calendar's weekday column.
var gregorianWeekdayNames = [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// gregorianWeekendIndices marks Saturday/Sunday (indices 5 and 6 in the
// Monday-first order above) as rest days — the one opinionated default this
// package adds on top of the bare calendar geometry.
var gregorianWeekendIndices = map[int]bool{5: true, 6: true}

// gregorianMonths returns the twelve real months, in order, as MonthInput.
func gregorianMonths() []MonthInput {
	out := make([]MonthInput, len(gregorianMonthDefs))
	for i, m := range gregorianMonthDefs {
		out[i] = MonthInput{Name: m.name, Days: m.days, SortOrder: i, LeapYearDays: m.leapYearDays}
	}
	return out
}

// gregorianWeekdays returns the seven real weekdays, Monday first.
func gregorianWeekdays() []WeekdayInput {
	out := make([]WeekdayInput, len(gregorianWeekdayNames))
	for i, name := range gregorianWeekdayNames {
		out[i] = WeekdayInput{Name: name, SortOrder: i, IsRestDay: gregorianWeekendIndices[i]}
	}
	return out
}

// realMoonEpochJDN anchors the real Moon's phase to a well-documented
// reference new moon (2000-01-06), computed through gregorianJDN — the SAME
// day-counter WeekdayIndex/recurrence/moon-phase math all share for a
// TracksRealTime calendar, so this stays self-consistent with them even
// though gregorianJDN's own zero point is arbitrary.
var realMoonEpochJDN = gregorianJDN(2000, 1, 6)

// realMoonSynodicDays is the Moon's mean synodic period (new moon to new
// moon) in days — a standard astronomical constant. It is a MEAN: the true
// cycle varies about +/-0.5 day around it (the Moon's orbit is elliptical),
// so a phase computed from it can drift by roughly that much from the real
// sky over long stretches. That is worth telling the owner (see the
// Warnings entry below), not a reason to leave the Moon out — it is exactly
// the same simple epoch+period approximation most lightweight moon-phase
// calculators use.
const realMoonSynodicDays = 29.530588853

// gregorianMoon is the real Moon, aligned to realMoonEpochJDN so
// Moon.MoonPhase reads 0 (new moon) on that date and every synodic period
// after it, for any TracksRealTime calendar (whose absDayIndex is
// gregorianJDN — see MoonPhase's caller in view_helpers.go/calendar_view.js).
func gregorianMoon() MoonInput {
	return MoonInput{
		Name:        "Moon",
		CycleDays:   realMoonSynodicDays,
		PhaseOffset: -float64(realMoonEpochJDN),
		Color:       "#c0c0c0",
	}
}

// GregorianImportResult builds the "Real-world calendar" tile's fixed
// definition: twelve real months, seven real weekdays (Monday first) and the
// real Moon, as an ImportResult ready for CreateCalendarFromImport — the
// same entry point a preset or an uploaded file uses, so it gets the exact
// same server-side validation (WizardRealWorldReview/WizardCreate never
// trust a client-submitted structure for this path; only the name, time
// zone, "today follows the real date" flag and — when that flag is off — an
// explicit date come from the browser).
//
// Settings.Mode is ModeRealLife; TracksRealTime/RealTimeZone are left unset
// here for the caller (WizardCreate) to fill in from the owner's actual
// choice. Today.Year defaults to the real current year (Month/Day stay nil,
// same "never invent a day-level default" rule #741 already applies to
// every other source) — sensible for both branches: the wall-clock branch
// overwrites all three, and the manual branch starts the year/month/day
// pickers somewhere plausible instead of year zero.
// Seasons/Hemisphere default to the northern set: defaultRealLifeSeasons's
// four reference dates (Mar 20/Jun 21/Sep 23/Dec 21) are hemisphere-neutral
// and only the NAMES flip, so "northern" here is a starting guess, not a
// geography check — nothing about the owner's campaign says which
// hemisphere their table is in. Hemisphere is set alongside the seasons
// (not left nil) so the two stay coherent from the very first read; an
// owner in the southern hemisphere flips it after creation via
// UpdateCalendar's existing Hemisphere field, exactly the same control an
// imported real-life calendar already uses for this — but the same control
// only AUTO-reseeds when the calendar has no seasons yet (its own doc
// comment), so seeding seasons here means a later flip renames nothing on
// its own; the owner would delete/redo them by hand. Worth a decision (open
// item on #741 — "should a new real-world calendar start with seasons"),
// not a call this package makes unilaterally.
func realWorldDefaultHemisphere() string { return HemisphereNorth }

func GregorianImportResult() (*ImportResult, error) {
	hemisphere := realWorldDefaultHemisphere()
	result := &ImportResult{
		Format:       FormatRealWorld,
		CalendarName: "Real world",
		Months:       gregorianMonths(),
		Weekdays:     gregorianWeekdays(),
		Moons:        []MoonInput{gregorianMoon()},
		Seasons:      defaultRealLifeSeasons(hemisphere),
		Settings: ImportedSettings{
			Mode:             ModeRealLife,
			HoursPerDay:      24,
			MinutesPerHour:   60,
			SecondsPerMinute: 60,
			Hemisphere:       &hemisphere,
		},
		Today: ImportedToday{Year: time.Now().Year()},
		Warnings: []string{
			"Moon phases use a mean synodic month (an average, not the true perturbed cycle), so they can drift by up to about half a day from the real Moon over long stretches.",
			"Seasons default to the northern hemisphere; change it under the calendar's settings if your table is in the south.",
		},
	}
	if err := clampCalendarStructure(result); err != nil {
		return nil, err
	}
	return result, nil
}
