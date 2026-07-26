// block_geometry.go — C-CALV4-SPINE-P2 (calendar-v4 wave 1, W-A).
//
// ONE leap-aware month producer for the calendar Block. Everything the Block's
// Instrument zone draws — week length, lead cells, row count, era bands,
// intercalary days, per-day moon discs — is emitted from here as the pinned
// calendar_block.MonthGeometry.
//
// WHY A SECOND GEOMETRY EXISTS. The V2 page's geometry lives in
// calendar_v2_helpers.go and reads RAW Months[i].Days (monthDays :277,
// monthWeekRowCount :1053, daysInRow :1071, monthEraBands :961). That is
// leap-BLIND: a real-time calendar draws February 2028 with 28 cells while
// Event.OccursOn — which routes through Calendar.MonthDays — places a recurring
// event on Feb 29. Nothing on main pins that disagreement.
//
// The V2 helpers are FROZEN for the whole calendar-v4 wave (COMMON §5): they are
// the densest source-text-pin site in the repo and three of those pins panic
// rather than fail. So the Block gets its own producer, routed through
// Calendar.MonthDays(idx, year), and the duplication is deliberate and
// temporary — the V2 page is retired by a later wave. Unifying now would red
// four JS tests and six Go pins for zero user-visible gain.
//
// WHAT IS DELIBERATELY SHARED. The weekday COLUMN comes from the existing
// v2WeekdayIndexFor (calendar_v2_helpers.go :410), not from a new counter. That
// is coordinator ruling COMMON §6.4: constLenDayIndex semantics do not change in
// wave 1, because making it intercalary-aware shifts the weekday column of every
// calendar already in the operator's production database. Reusing it is what
// guarantees the Block's grid column, the V2 grid column, and Event.OccursOn's
// recurrence stride all agree. See the three-counter note on
// Calendar.constLenDayIndex in model.go and TestBlockCounterDivergencePin.
package calendar

import (
	"fmt"
	"math"
	"strings"

	calblock "github.com/keyxmakerx/chronicle/internal/widgets/calendar_block"
)

// blockDefaultWeekLen is the NAMED fallback week length used when a calendar
// declares zero weekdays. The V2 helpers bury the same value as a bare `7`
// inside monthColumnCount (calendar_v2_helpers.go :807); calendar-v4's rule is
// that the number is never *assumed* — variable week length already works (a
// ten-day week renders ten columns), so the fallback has to be visible and
// nameable rather than a literal in an expression. A calendar with no weekdays
// is a half-built calendar, and 7 is the least surprising shape to draw it in
// until its author adds some.
const blockDefaultWeekLen = 7

// blockHalfRuleWeekLen scopes the signed "five-column rule" — the mid-week
// editorial rule drawn on the middle column of a ten-day week. The signed
// contract applies it to ten-day weeks ONLY (mockups/calendar-v4.html: the cell
// class is `col === Math.floor(m.week / 2) && m.week === 10`), so this is not
// generalised to "any even week": widening it would desynchronise every signed
// still that shows a seven-column month with no mid-week rule.
const blockHalfRuleWeekLen = 10

// blockCellMarkCap / blockCellMarkCapOverflow implement the signed per-cell
// chip ceiling: three marks, OR two marks plus a "+n more" affordance — never
// both, so a named cell is always exactly one height and the mark count can
// never change a row's geometry (mockups/calendar-v4.html cell():
// `const cap = inline.length > 3 ? 2 : 3`).
const (
	blockCellMarkCap         = 3
	blockCellMarkCapOverflow = 2
)

// blockMonthGeometryInput carries the per-render knobs the geometry cannot
// derive from the calendar alone. MonthIndex is 0-BASED (it indexes
// Calendar.Months and is what Calendar.MonthDays takes); Year is the displayed
// year, which is what makes the whole producer leap-aware.
type blockMonthGeometryInput struct {
	MonthIndex int
	Year       int
	// ShowMoons mirrors LayerState.Enabled containing "moons" — the wave-1
	// default surface (DEF = ["moons"]). False skips the moon pass entirely.
	ShowMoons bool
	// MoonCap bounds how many moons get a per-day disc. The ceiling is declared
	// ONCE in the Nameplate ("3 of 4 moons"), never per cell (L25/L30), so the
	// producer only has to honour it, not announce it. <= 0 means "all".
	MoonCap int
}

// blockWeekLen returns the calendar's week length, falling back to the NAMED
// blockDefaultWeekLen. This is the value that becomes --week-len; there is no
// literal 7 anywhere downstream of it.
func blockWeekLen(cal *Calendar) int {
	if cal == nil || len(cal.Weekdays) == 0 {
		return blockDefaultWeekLen
	}
	return len(cal.Weekdays)
}

// blockMonthDays returns the LEAP-AWARE day count for a month in a year. This
// is the single line the whole file exists for: Calendar.MonthDays consults
// IsLeapYear (fantasy) or the proleptic-Gregorian stdlib (real-time), where the
// V2 helpers read Months[i].Days flat.
func blockMonthDays(cal *Calendar, monthIdx, year int) int {
	if cal == nil {
		return 0
	}
	return cal.MonthDays(monthIdx, year)
}

// blockMonthLead returns the number of blank leading cells before day 1 — the
// weekday column day 1 falls under.
//
// It delegates to v2WeekdayIndexFor deliberately (see the file header): the
// Block must place a day in the SAME column the V2 grid and Event.OccursOn's
// recurrence stride already place it, and that column is defined by
// constLenDayIndex, whose semantics ruling COMMON §6.4 freezes for wave 1.
func blockMonthLead(cal *Calendar, monthIdx, year int) int {
	if cal == nil {
		return 0
	}
	lead := v2WeekdayIndexFor(cal, year, monthIdx+1, 1)
	if lead < 0 {
		return 0
	}
	if wl := blockWeekLen(cal); lead >= wl {
		return lead % wl
	}
	return lead
}

// blockRowCount is ceil((lead+days)/weekLen) — the pinned RowCount rule.
func blockRowCount(lead, days, weekLen int) int {
	if weekLen < 1 || days < 1 {
		return 0
	}
	return (lead + days + weekLen - 1) / weekLen
}

// blockIsHalfCol reports whether a 1-based column carries the five-column rule.
func blockIsHalfCol(weekLen, col int) bool {
	return weekLen == blockHalfRuleWeekLen && col == weekLen/2
}

// blockWeekdays projects the calendar's weekday names into the pinned Weekday
// shape. Abbr is the first three runes (the full tier's `w.slice(0,3)`); the
// renderer shortens further per size class — tier is not the producer's
// business. A calendar with no weekdays yields the fallback-length header row
// with empty names rather than a fabricated Sun..Sat, so the surface says "this
// calendar has no weekdays" instead of quietly inventing Gregorian ones.
func blockWeekdays(cal *Calendar) []calblock.Weekday {
	wl := blockWeekLen(cal)
	out := make([]calblock.Weekday, 0, wl)
	for i := 0; i < wl; i++ {
		name := ""
		if cal != nil && i < len(cal.Weekdays) {
			name = cal.Weekdays[i].Name
		}
		out = append(out, calblock.Weekday{
			Name:  name,
			Abbr:  blockAbbrev(name, 3),
			Half:  blockIsHalfCol(wl, i+1),
			Index: i,
		})
	}
	return out
}

// blockAbbrev truncates to n RUNES (not bytes) so a non-ASCII weekday or month
// name is not sliced mid-codepoint.
func blockAbbrev(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// --- moon phase: ONE absolute day per month, then step ---------------------

// monthBaseAbsoluteDay returns Calendar.AbsoluteDay for day 1 of the month.
//
// THE PERFORMANCE TRAP (COMMON §7). Calendar.AbsoluteDay loops y = 0..year-1
// and sums YearLengthForYear, which itself loops the months: one call costs
// year × len(Months) iterations. At year 1523 with 12 months that is 18,276
// iterations — per call. Calling it per cell for a 30-day month costs 548,280,
// and per (cell × moon) with three moons, 1,644,840. That is the difference
// between a page load and a timeout.
//
// AbsoluteDay(y, m, d) == AbsoluteDay(y, m, 1) + (d-1) by construction (the
// day term is a plain addition at the end of the function), so ONE base call
// plus an integer offset per cell is exact, not an approximation.
// TestBlockMoonBaseDayIsSteppedNotRecomputed pins both the equality and the
// cost ratio.
func monthBaseAbsoluteDay(cal *Calendar, monthIdx, year int) int {
	if cal == nil {
		return 0
	}
	return cal.AbsoluteDay(year, monthIdx+1, 1)
}

// moonDiscsForDay derives the per-day moon discs from a PRECOMPUTED base
// absolute day. Taking `baseAbsDay` rather than a (year, month, day) triple is
// the API-level guarantee that no caller can reintroduce the per-cell
// AbsoluteDay loop.
//
// The disc is a derived TERMINATOR, not a pie fill (L25):
//   - Illum   = (1 - cos(2π·phase)) / 2   — 0 at new, 1 at full. Matches the
//     signed illumOf() exactly (mockups/calendar-v4.html :1326).
//   - Waxing  = phase < 0.5               — the signed `wane = ph > 0.5`.
//   - Terminator = "<f> <mode>" where f = |cos(2π·phase)| is the terminator
//     ellipse's width as a FRACTION of the disc diameter (the signed
//     `term = |cos(2πph)| * px`, with px left to the renderer because the disc
//     size is a size-class decision), and mode is "add" for a gibbous disc
//     (illum > 0.5 — the ellipse ADDS lit area, signed --termfill:
//     rule-structural-strong) or "sub" for a crescent (the ellipse SUBTRACTS,
//     signed --termfill: surface-card). New = empty ring, full = solid disc,
//     quarters = exactly half.
//
// WAVE-1 RULING — Eclipse is always false. calendar_celestial_events
// (migration 008 :46-64) has NO moon_id column: a stored eclipse is a dated sky
// event with no link to the moon it eclipses. Marking every disc on that date
// would assert a fact the schema cannot support, which is the same class of
// dishonesty the fog ruling (COMMON §6.1) forbids. The field stays false until
// a per-moon linkage exists; the Almanac (W-E) is where dated celestial events
// belong.
func moonDiscsForDay(moons []Moon, baseAbsDay, day, moonCap int) []calblock.MoonDisc {
	if len(moons) == 0 {
		return nil
	}
	n := len(moons)
	if moonCap > 0 && moonCap < n {
		n = moonCap
	}
	absDay := baseAbsDay + day - 1
	out := make([]calblock.MoonDisc, 0, n)
	for i := 0; i < n; i++ {
		m := moons[i]
		phase := m.MoonPhase(absDay)
		illum := (1 - math.Cos(2*math.Pi*phase)) / 2
		term := math.Abs(math.Cos(2 * math.Pi * phase))
		// Gibbous is decided on the ROUNDED percentage, exactly as the signed
		// moonSil() does (`il = Math.round(illumOf(ph) * 100); gibbous = il > 50`).
		// Comparing the float against 0.5 instead puts a knife-edge on the
		// quarters: cos(3π/2) is -1.8e-16 rather than 0, so a first-quarter disc
		// would flip to the gibbous fill on floating-point noise alone.
		mode := "sub"
		if int(math.Round(illum*100)) > 50 {
			mode = "add"
		}
		out = append(out, calblock.MoonDisc{
			Name:       m.Name,
			Illum:      illum,
			Waxing:     phase < 0.5,
			Eclipse:    false, // ruling above — no per-moon eclipse linkage exists
			Terminator: fmt.Sprintf("%.4f %s", term, mode),
		})
	}
	return out
}

// --- era bands -------------------------------------------------------------

// blockErasForYear returns the calendar's eras covering `year`, in the order
// the operator sorted them. Chronicle's Era is YEAR-granular (start_year /
// end_year, migration 001), so ordinarily exactly zero or one era applies to a
// whole month; more than one only when the operator has authored OVERLAPPING
// eras, which the model permits and the band producer therefore renders rather
// than silently dropping.
func blockErasForYear(cal *Calendar, year int) []Era {
	if cal == nil {
		return nil
	}
	out := make([]Era, 0, 1)
	for _, e := range cal.Eras {
		if e.ContainsYear(year) {
			out = append(out, e)
		}
	}
	return out
}

// blockEraBands builds the era bands for ONE week row.
//
// PER WEEK ROW, never per month (§9 deviation 2): a 30-day month in ten-day
// weeks is three rows, so an era cannot be one subgrid-spanning band — span 17
// silently wrecks the grid. Each row it touches gets its own band, column
// aligned, with OpenLeft/OpenRight saying whether it continues past this row.
//
// KNOWN CONTRACT GAP — the signed render shows two eras splitting ONE month at
// day 17/18 (mockups/calendar-v4.html HARPTOS.eras) with a 1.5px editorial rule
// (Edge) at the mid-month boundary. Chronicle's eras are year-granular, so a
// mid-month era boundary is not expressible on main and Edge is therefore
// always false today. The general rule is implemented anyway so a later
// day-granular era model needs no re-touch here; TestBlockEraBandEdgeIsUnreachable
// pins the current answer so the change is deliberate. Flagged to the
// coordinator rather than approximated.
func blockEraBands(cal *Calendar, in blockMonthGeometryInput, row, lead, days, weekLen, rowCount int) []calblock.EraBand {
	eras := blockErasForYear(cal, in.Year)
	if len(eras) == 0 || weekLen < 1 {
		return nil
	}
	rowFirst := row*weekLen + 1 - lead
	rowLast := rowFirst + weekLen - 1
	from := rowFirst
	if from < 1 {
		from = 1 // skip the leading blanks of row 0
	}
	to := rowLast
	if to > days {
		to = days // skip the trailing blanks of the last row
	}
	if from > to {
		return nil
	}
	startCol := ((from - 1 + lead) % weekLen) + 1
	span := to - from + 1
	halfCol := 0
	if weekLen == blockHalfRuleWeekLen {
		halfCol = weekLen / 2
	}

	// Season suffix, row 0 ONLY (§9 deviation 3), and only on the first band of
	// that row — repeating it per overlapping era would print the season twice.
	suffix := ""
	if row == 0 && cal != nil {
		if s := cal.SeasonForDate(in.MonthIndex+1, 1); s != nil {
			suffix = s.Name
		}
	}

	// An era is year-granular, so within this month it covers every day. It
	// therefore continues before this row unless the row is row 0 AND the era
	// begins exactly at this month's day 1 — which needs the era to start in
	// this very year and this to be the year's first month.
	startsHere := func(e Era) bool { return e.StartYear == in.Year && in.MonthIndex == 0 }
	endsHere := func(e Era) bool {
		if e.EndYear == nil || *e.EndYear > in.Year {
			return false
		}
		return cal != nil && in.MonthIndex == len(cal.Months)-1
	}

	out := make([]calblock.EraBand, 0, len(eras))
	for i, e := range eras {
		openLeft := row > 0 || !startsHere(e)
		openRight := row < rowCount-1 || !endsHere(e)
		band := calblock.EraBand{
			Label:     e.Name,
			StartCol:  startCol,
			Span:      span,
			BandHue:   strings.TrimSpace(e.Color),
			OpenLeft:  openLeft,
			OpenRight: openRight,
			// Edge is the signed editorial rule at a MID-MONTH era boundary: the
			// band's left edge is the era's own first day (nothing continues in
			// from the left) AND that day is not the month's day 1. A
			// year-granular era can only begin on day 1 of month 1, so `from`
			// is always 1 when openLeft is false and Edge is unreachable today
			// — see TestBlockEraBandEdgeIsUnreachableToday. The general form is
			// written so a later day-granular era model needs no change here.
			Edge: !openLeft && from > 1,
			Half: halfCol > 0 && startCol <= halfCol && halfCol <= startCol+span-1,
		}
		if i == 0 {
			band.Suffix = suffix
		}
		out = append(out, band)
	}
	return out
}

// --- intercalary days ------------------------------------------------------

// blockIntercalary returns the intercalary days that belong to a month: the
// days of every Month flagged is_intercalary that immediately FOLLOWS it, in
// order, stopping at the first ordinary month.
//
// This is the case a 30-day / ten-day-week month has no cell for — canonical
// Harptos carries festival days outside every tenday, and the grid has nowhere
// to put them. Day is the day number WITHIN the intercalary month (Midwinter is
// day 1); Name is the intercalary month's name, or the festival declared on
// that date when the month is unnamed, so a one-day intercalary month reads as
// "Midwinter" rather than as a blank.
//
// Marks are filled by the projection (block_projection.go) — geometry knows
// nothing about events or viewers.
func blockIntercalary(cal *Calendar, monthIdx, year int) []calblock.IntercalaryDay {
	if cal == nil {
		return nil
	}
	var out []calblock.IntercalaryDay
	for _, i := range blockIntercalaryMonths(cal, monthIdx) {
		m := cal.Months[i]
		days := cal.MonthDays(i, year)
		for d := 1; d <= days; d++ {
			name := strings.TrimSpace(m.Name)
			if name == "" {
				if f := FestivalOnDate(cal, i+1, d); len(f) > 0 {
					name = f[0].Name
				}
			}
			if days > 1 && name != "" {
				name = fmt.Sprintf("%s %d", name, d)
			}
			out = append(out, calblock.IntercalaryDay{Name: name, Day: d})
		}
	}
	return out
}

// blockIntercalaryMonths returns the 0-based indices of the intercalary months
// that belong to monthIdx — every following Month flagged is_intercalary, up to
// the first ordinary one.
//
// It is the SINGLE definition of that adjacency rule: blockIntercalary walks it
// to build the rows, and the projection walks it again to place marks on those
// rows. Two independent copies of "which months hang off this one" is exactly
// how a row and its events end up disagreeing.
func blockIntercalaryMonths(cal *Calendar, monthIdx int) []int {
	if cal == nil {
		return nil
	}
	var out []int
	for i := monthIdx + 1; i < len(cal.Months); i++ {
		if !cal.Months[i].IsIntercalary {
			break
		}
		out = append(out, i)
	}
	return out
}

// --- resolvers the phase-B surfaces consume --------------------------------

// FestivalOnDate returns every festival the calendar declares on (month, day),
// month 1-based. Festivals are part of the calendar STRUCTURE (a fixed holiday
// authored on the calendar), not calendar_events, so they carry no visibility
// and are the same for every viewer — which is why this is a pure resolver and
// not part of the per-viewer projection.
//
// Between-months festivals (Festival.AfterMonth, the intercalary kind) are
// deliberately NOT returned here: they have no day number, so they cannot match
// a date. blockIntercalary is where they surface.
func FestivalOnDate(cal *Calendar, month, day int) []Festival {
	if cal == nil {
		return nil
	}
	var out []Festival
	for _, f := range cal.Festivals {
		if f.Month == nil || f.Day == nil {
			continue
		}
		if *f.Month == month && *f.Day == day {
			out = append(out, f)
		}
	}
	return out
}

// CycleEntryForYear returns the cycle entry in effect for a year, or nil.
//
// A Cycle rotates its entries over cycle_length YEARS; an entry claims the
// years where (year - year_offset) mod cycle_length == 0 in the showcase's
// model, but the shipped data carries an explicit YearOffset per entry, so the
// entry for a year is the one at position (year mod cycle_length) once entries
// are ordered by (year_offset, sort_order). Zero-length cycles and empty entry
// sets resolve to nil rather than dividing by zero.
func CycleEntryForYear(cyc *Cycle, year int) *CycleEntry {
	if cyc == nil || len(cyc.Entries) == 0 {
		return nil
	}
	n := cyc.CycleLength
	if n <= 0 {
		n = len(cyc.Entries)
	}
	// Match an explicit offset first — an authored entry wins over positional
	// rotation, which is what YearOffset exists for.
	pos := ((year % n) + n) % n
	for i := range cyc.Entries {
		if e := &cyc.Entries[i]; e.YearOffset == pos {
			return e
		}
	}
	if pos < len(cyc.Entries) {
		return &cyc.Entries[pos]
	}
	return &cyc.Entries[pos%len(cyc.Entries)]
}

// --- the producer ----------------------------------------------------------

// buildMonthGeometry emits ONE fully-resolved month.
//
// It fills every field of MonthGeometry except the per-viewer decoration
// (DayCell.Marks / Tied / MoreCount and IntercalaryDay.Marks), which
// block_projection.go layers on from a single viewer-filtered event pass.
// Splitting it this way is what makes the count-oracle guarantee possible:
// geometry cannot leak what it never sees.
func buildMonthGeometry(cal *Calendar, in blockMonthGeometryInput) calblock.MonthGeometry {
	weekLen := blockWeekLen(cal)
	geo := calblock.MonthGeometry{
		Index:    in.MonthIndex,
		Year:     in.Year,
		WeekLen:  weekLen,
		Weekdays: blockWeekdays(cal),
	}
	if cal == nil || in.MonthIndex < 0 || in.MonthIndex >= len(cal.Months) {
		return geo
	}

	geo.Name = cal.Months[in.MonthIndex].Name
	geo.Days = blockMonthDays(cal, in.MonthIndex, in.Year)
	geo.Lead = blockMonthLead(cal, in.MonthIndex, in.Year)
	geo.RowCount = blockRowCount(geo.Lead, geo.Days, weekLen)
	geo.Intercalary = blockIntercalary(cal, in.MonthIndex, in.Year)

	if in.Year == cal.CurrentYear && in.MonthIndex+1 == cal.CurrentMonth &&
		cal.CurrentDay >= 1 && cal.CurrentDay <= geo.Days {
		geo.TodayDay = cal.CurrentDay
	}

	// ONE base absolute day for the whole month; every cell steps off it.
	var moonBase int
	var moons []Moon
	if in.ShowMoons && len(cal.Moons) > 0 {
		moonBase = monthBaseAbsoluteDay(cal, in.MonthIndex, in.Year)
		moons = cal.Moons
	}

	geo.Rows = make([]calblock.WeekRow, 0, geo.RowCount)
	for r := 0; r < geo.RowCount; r++ {
		row := calblock.WeekRow{
			Index: r,
			Bands: blockEraBands(cal, in, r, geo.Lead, geo.Days, weekLen, geo.RowCount),
			Cells: make([]calblock.DayCell, 0, weekLen),
		}
		for c := 1; c <= weekLen; c++ {
			d := r*weekLen + c - geo.Lead
			cell := calblock.DayCell{
				Col:  c,
				Half: blockIsHalfCol(weekLen, c),
				// WAVE-1 RULING (COMMON §6.1): there is no queryable knowledge
				// horizon on main — the mockup's m.horizon is a literal on its
				// own month object. Fogged stays false and the Horizon surfaces
				// render the signed `needs backend` chip. W-F builds the real
				// thing; the field exists so it need not re-touch every cell.
				Fogged: false,
			}
			if d >= 1 && d <= geo.Days {
				cell.Day = d
				cell.IsToday = d == geo.TodayDay
				cell.Moons = moonDiscsForDay(moons, moonBase, d, in.MoonCap)
			}
			row.Cells = append(row.Cells, cell)
		}
		geo.Rows = append(geo.Rows, row)
	}
	return geo
}
