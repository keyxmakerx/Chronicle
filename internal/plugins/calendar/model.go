// Package calendar provides a custom fantasy calendar system for campaigns.
// Supports non-Gregorian months, named weekdays, moons with phase tracking,
// named seasons, and events linked to entities. Each campaign can have
// multiple calendars (one marked as default); the addon must be enabled
// per-campaign.
package calendar

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// VisibilityRules defines per-user visibility overrides for calendar events.
// If AllowedUsers is set, only those users can see the item (whitelist).
// If DeniedUsers is set, those users cannot see the item (blacklist).
// AllowedUsers takes precedence: if set, DeniedUsers is ignored.
type VisibilityRules struct {
	AllowedUsers []string `json:"allowed_users,omitempty"`
	DeniedUsers  []string `json:"denied_users,omitempty"`
}

// ParseVisibilityRules parses raw JSON (Calendar.VisibilityRules or
// Event.VisibilityRules) into a *VisibilityRules, or nil when unset. Shared
// by both owners of a visibility_rules column so the parse can't drift
// between them; Event additionally exposes it as a method (below) for
// existing callers.
func ParseVisibilityRules(raw *string) *VisibilityRules {
	if raw == nil || *raw == "" {
		return nil
	}
	var rules VisibilityRules
	if err := json.Unmarshal([]byte(*raw), &rules); err != nil {
		return nil
	}
	return &rules
}

// Allows reports whether userID may see an item carrying these rules — a
// nil receiver (no rules stored) allows everyone. This is a VERBATIM MIRROR
// of maps.VisibilityRules.Allows (internal/plugins/maps/model.go) and the
// allow/deny half of timeline's canUserView: cross-plugin repo/model access
// is forbidden (plugins reach each other only through service interfaces),
// so the policy is replicated rather than imported.
//
// SECURITY-SENSITIVE: keep in lockstep with maps' and timeline's twins,
// including the ADR-049 DeniesAnonymous check — a logged-out viewer can't be
// proven not to be the specific player a deny list names, so a non-empty
// DeniedUsers excludes them too, even though they match no literal entry.
func (v *VisibilityRules) Allows(userID string) bool {
	if v == nil {
		return true
	}
	if permissions.DeniesAnonymous(v.DeniedUsers, userID) {
		return false
	}
	for _, id := range v.DeniedUsers {
		if id == userID {
			return false
		}
	}
	if len(v.AllowedUsers) == 0 {
		return true
	}
	for _, id := range v.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

// UpdateEventVisibilityInput is the validated input for updating event
// visibility. Visibility stays a plain string: this is a "set visibility"
// action, not a general settings save, so the caller always states the new
// visibility outright — there is no absent-preserves case for it (see
// governedFieldExceptions in internal/patch/partial_update_contract_test.go).
// VisibilityRules is patch.Field so a visibility-only call (flip
// everyone/dm_only) can leave an existing per-user rules blob untouched:
// absent preserves it, explicit null clears it, a value replaces it.
type UpdateEventVisibilityInput struct {
	Visibility      string
	VisibilityRules patch.Field[string]
}

// UpdateCalendarVisibilityInput is the validated input for updating a
// calendar's per-calendar visibility. Same shape as the event one — the
// calendar reuses the event visibility model + resolver.
type UpdateCalendarVisibilityInput struct {
	Visibility      string
	VisibilityRules patch.Field[string]
}

// CalendarEventDate is a lightweight (calendar, date, name) tuple from the
// batch upcoming read — kept minimal so the cross-calendar query stays cheap.
type CalendarEventDate struct {
	CalendarID string
	Year       int
	Month      int
	Day        int
	Name       string
}

// CalendarUpcoming is a calendar's next upcoming event + a short agenda,
// computed from the batch read relative to that calendar's own current date.
// Next is nil when the calendar has none upcoming.
type CalendarUpcoming struct {
	Next   *CalendarEventDate
	Agenda []CalendarEventDate
}

// Calendar mode constants.
const (
	// ModeFantasy indicates a fully custom fantasy calendar.
	ModeFantasy = "fantasy"
	// ModeRealLife indicates a Gregorian calendar synced to real-world time.
	ModeRealLife = "reallife"
)

// Calendar is the top-level calendar definition for a campaign.
type Calendar struct {
	ID               string  `json:"id"`
	CampaignID       string  `json:"campaign_id"`
	Mode             string  `json:"mode"` // "fantasy" or "reallife"
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	EpochName        *string `json:"epoch_name,omitempty"`
	CurrentYear      int     `json:"current_year"`
	CurrentMonth     int     `json:"current_month"`
	CurrentDay       int     `json:"current_day"`
	HoursPerDay      int     `json:"hours_per_day"`
	MinutesPerHour   int     `json:"minutes_per_hour"`
	SecondsPerMinute int     `json:"seconds_per_minute"`
	CurrentHour      int     `json:"current_hour"`
	CurrentMinute    int     `json:"current_minute"`
	LeapYearEvery    int     `json:"leap_year_every"`
	LeapYearOffset   int     `json:"leap_year_offset"`
	SortOrder        int     `json:"sort_order"`
	IsDefault        bool    `json:"is_default"`
	// Hemisphere flips how a real-world calendar's seasons read (winter in
	// the north is summer in the south); nil means unset (every fantasy
	// calendar, and a real-world one whose owner hasn't chosen yet).
	Hemisphere *string `json:"hemisphere,omitempty"`
	// ForecastsEnabled gates the one exception to "players never learn a
	// future day's weather before it happens": off by default, and even on,
	// a forecast only ever describes days ahead of the current one.
	ForecastsEnabled bool `json:"forecasts_enabled"`
	// MonthStartsNewWeek makes day 1 of every month the first weekday, with
	// intercalary festival days belonging to no week, so a festival never
	// shifts the weekdays that follow it (a Harptos-style calendar: ten-day
	// "tendays" that reset every month, with 1-day festivals between months
	// belonging to no week at all). WeekdayIndex is the single reader of this
	// switch — every weekday placement (recurrence display, a future grid
	// column) goes through it, so the two can never disagree.
	MonthStartsNewWeek bool `json:"month_starts_new_week"`
	// Per-calendar visibility, mirroring the event model: Visibility is
	// "everyone" | "dm_only"; VisibilityRules is the optional
	// {allowed_users,denied_users} JSON allow/deny override. Resolved by
	// canUserView / filterCalendarsByUser.
	Visibility      string  `json:"visibility"`
	VisibilityRules *string `json:"visibility_rules,omitempty"`
	// Real-time (wall-clock) mode: a flag on `reallife` mode, not a separate
	// mode. TracksRealTime=1 makes the loader compute Current* from the wall
	// clock in RealTimeZone and the date-writers reject manual changes; =0 is
	// stored-not-computed. RealTimeZone is the IANA anchor, required at enable.
	// Tagged json:"-": not wire-exposed directly, consumed server-side only.
	TracksRealTime bool    `json:"-"`
	RealTimeZone   *string `json:"-"`
	// The real-date anchor: one in-world date and the Gregorian date it
	// equals, from which every other day follows by AbsoluteDay arithmetic.
	//
	// All four or none — a partial anchor maps nothing, so HasRealAnchor()
	// requires the set and the service refuses to write a subset.
	//
	// Serialized, unlike the real-time pair above: the Foundry module and the
	// export both need to know which real date a session lands on. It carries
	// no zone and no time.
	AnchorYear     *int       `json:"anchor_year,omitempty"`
	AnchorMonth    *int       `json:"anchor_month,omitempty"`
	AnchorDay      *int       `json:"anchor_day,omitempty"`
	AnchorRealDate *time.Time `json:"anchor_real_date,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Eager-loaded sub-resources (populated by service, not by every query).
	Months   []Month   `json:"months,omitempty"`
	Weekdays []Weekday `json:"weekdays,omitempty"`
	Moons    []Moon    `json:"moons,omitempty"`
	Seasons  []Season  `json:"seasons,omitempty"`
	Eras     []Era     `json:"eras,omitempty"`
	// EventKinds is the campaign's shared kind list (see EventKind), not this
	// calendar's own — every calendar in a campaign sees the same kinds.
	EventKinds []EventKind `json:"event_kinds,omitempty"`
	Cycles     []Cycle     `json:"cycles,omitempty"`
	Festivals  []Festival  `json:"festivals,omitempty"`
	// Weather is nil when no row exists in calendar_weather for this
	// calendar, so the settings page can render current state without a
	// second handler-side fetch.
	Weather *Weather `json:"weather,omitempty"`
}

// GetCampaignID returns the campaign ID this calendar belongs to.
// Implements middleware.CampaignScoped for IDOR protection.
func (c *Calendar) GetCampaignID() string {
	return c.CampaignID
}

// IsRealLife returns true if this calendar syncs to real-world time.
func (c *Calendar) IsRealLife() bool {
	return c.Mode == ModeRealLife
}

// UsesRealTime reports whether this calendar computes its current date from
// the wall clock (and rejects manual date changes). It is the single
// predicate gating the real-time seam, the manual-date-write guard, and the
// stdlib month geometry, so all three agree. Keyed on the flag, not just the
// mode, so a `reallife`-but-manual calendar (TracksRealTime=0) behaves exactly
// as before real-time support existed. A nil/blank RealTimeZone is handled by
// the loader's fail-safe (serve the stored date, never 500).
func (c *Calendar) UsesRealTime() bool {
	return c.Mode == ModeRealLife && c.TracksRealTime
}

// IsLeapYear returns true if the given year is a leap year according to
// the calendar's leap year configuration. LeapYearEvery=0 means no leap years.
func (c *Calendar) IsLeapYear(year int) bool {
	if c.LeapYearEvery <= 0 {
		return false
	}
	return (year-c.LeapYearOffset)%c.LeapYearEvery == 0
}

// YearLength returns the total number of days in a year by summing all month
// lengths. Does not account for leap year — use YearLengthForYear for that.
func (c *Calendar) YearLength() int {
	total := 0
	for _, m := range c.Months {
		total += m.Days
	}
	return total
}

// YearLengthForYear returns the total days in a specific year, including
// leap year extra days if applicable.
func (c *Calendar) YearLengthForYear(year int) int {
	total := 0
	isLeap := c.IsLeapYear(year)
	for _, m := range c.Months {
		total += m.Days
		if isLeap {
			total += m.LeapYearDays
		}
	}
	return total
}

// MonthDays returns the number of days in a month for a given year,
// accounting for leap year extra days.
//
// Real-time calendars (UsesRealTime) derive month length from the proleptic
// Gregorian stdlib (daysInGregorianMonth — correct 4/100/400 leap rule)
// instead of the configurable LeapYearEvery/IsLeapYear fields, which cannot
// express Gregorian (with LeapYearEvery=4 they render Feb 2100 as 29). Only
// flagged real-time calendars take this branch — fantasy and
// reallife-but-manual calendars keep their configured geometry unchanged.
func (c *Calendar) MonthDays(monthIdx int, year int) int {
	if monthIdx < 0 || monthIdx >= len(c.Months) {
		return 0
	}
	if c.UsesRealTime() {
		return daysInGregorianMonth(year, monthIdx+1)
	}
	days := c.Months[monthIdx].Days
	if c.IsLeapYear(year) {
		days += c.Months[monthIdx].LeapYearDays
	}
	return days
}

// WeekLength returns the number of days in a week (number of weekdays).
func (c *Calendar) WeekLength() int {
	return len(c.Weekdays)
}

// Recurrence type constants mirror the sessions plugin's vocabulary
// (internal/plugins/sessions/model.go) so the two share semantics. Any other
// or empty recurrence_type renders once at its stored date.
//
// This block is the accepted set, stated exactly once. CreateEventAPI /
// UpdateEventAPI validate an inbound recurrence_type against these five
// constants plus the empty string and reject anything else with a 400.
// Adding a member here widens what the API accepts, so a new constant is a
// wire-contract change, not a rename.
const (
	RecurrenceWeekly   = "weekly"   // every week, on the base date's weekday
	RecurrenceBiWeekly = "biweekly" // every 2 weeks
	RecurrenceMonthly  = "monthly"  // same day-of-month each month
	RecurrenceCustom   = "custom"   // every N weeks (RecurrenceInterval)
	RecurrenceYearly   = "yearly"   // same month + day-of-month each year
)

// RecurrenceTypes is the accepted set as data, for the handlers' input
// validation. It is derived from the constants above rather than re-typed,
// so the handler stays the one validator for one set.
var RecurrenceTypes = []string{
	RecurrenceWeekly, RecurrenceBiWeekly, RecurrenceMonthly,
	RecurrenceCustom, RecurrenceYearly,
}

// IsSupportedRecurrenceType reports whether t is a recurrence_type the engine
// actually expands. The empty string is accepted and means "not recurring".
//
// The comparison is exact and case-sensitive by design: a case-different
// "WEEKLY" from an integration is rejected loudly rather than coerced, since
// coercion would turn an undocumented spelling into a supported one.
func IsSupportedRecurrenceType(t string) bool {
	if t == "" {
		return true
	}
	for _, ok := range RecurrenceTypes {
		if t == ok {
			return true
		}
	}
	return false
}

// absDayIndex returns a calendar-absolute day number for (year, month, day).
// It is the single place the day-counter choice is made; recurrence expansion
// (OccursOn) and WeekdayIndex's continuous-week branch both consume it, so a
// weekly event and the weekday column it renders on can never disagree.
// month is 1-based.
//
// Real-time (Gregorian) calendars use the proleptic-Gregorian Julian Day
// Number, a true day counter that advances by exactly one across a leap day.
// A positive year delegates to AbsoluteDay, the same leap-aware closed-form
// counter moon-phase math already uses — this used to be a separate,
// leap-naive constant-length sum (year*YearLength()) that diverged from
// AbsoluteDay by one day per elapsed leap year for positive years;
// reconciled so recurrence, weekday placement and moon phase can never drift
// apart there.
//
// year <= 0 stays on the old linear formula: AbsoluteDay's leap term is
// defined only for year > 0 (its own doc comment — "a negative or zero year
// contributes nothing"), so delegating unconditionally would collapse every
// non-positive year onto the same index (AbsoluteDay(-5,1,1) ==
// AbsoluteDay(0,1,1)), breaking recurrence and weekday placement for any
// calendar that permits year 0 or negative years. linearDayIndex stays
// leap-unaware but strictly monotonic across zero and negative years, which
// is what those two consumers actually need.
func (c *Calendar) absDayIndex(year, month, day int) int {
	if c.UsesRealTime() {
		return gregorianJDN(year, month, day)
	}
	if year <= 0 {
		return c.linearDayIndex(year, month, day)
	}
	return c.AbsoluteDay(year, month, day)
}

// linearDayIndex is the constant-length, leap-unaware absolute-day counter:
// year*YearLength() + prior configured month days + day (month 1-based).
// absDayIndex's fallback for year <= 0 — see its doc comment.
func (c *Calendar) linearDayIndex(year, month, day int) int {
	abs := year * c.YearLength()
	for i := 0; i < month-1 && i < len(c.Months); i++ {
		abs += c.Months[i].Days
	}
	return abs + day
}

// monthIsIntercalary reports whether the 1-based month index refers to an
// intercalary (festival) month. Out-of-range indices are not intercalary —
// callers that pass a bad month get "not intercalary" rather than a panic.
func (c *Calendar) monthIsIntercalary(month int) bool {
	i := month - 1
	if i < 0 || i >= len(c.Months) {
		return false
	}
	return c.Months[i].IsIntercalary
}

// WeekdayIndex returns the 0-based index into c.Weekdays that (year, month,
// day) falls on. It is the single function any weekday renderer (grid column,
// list badge) calls to place a day in its week — never absDayIndex's modulo
// directly, so MonthStartsNewWeek's reset/no-week rules apply everywhere or
// nowhere.
//
// LIMITATION: OccursOn's week-based recurrence (weekly/biweekly/custom) does
// NOT read MonthStartsNewWeek — it steps by continuous absDayIndex days
// regardless, so on a MonthStartsNewWeek calendar a weekly-recurring event
// can land in a different WeekdayIndex column than it started in once an
// intercalary month has passed. Reconciling the two needs OccursOn's stride
// math to skip intercalary days the same way WeekdayIndex does, which is
// deferred: WeekdayIndex has no production caller yet (a future grid render
// is the first one), so nothing surfaces the mismatch today.
//
// Real-time calendars ignore MonthStartsNewWeek entirely: Gregorian weeks are
// always continuous, matching this method's own MonthStartsNewWeek=false
// branch below.
//
// When MonthStartsNewWeek is true, day 1 of every month is always the week's
// first day (a month never inherits a stray offset from the one before it),
// and a day inside an INTERCALARY month belongs to no week at all — it is a
// festival day outside the weekday cycle, so a caller must render it in its
// own between-week band rather than a weekday column. -1 signals that case.
//
// When MonthStartsNewWeek is false (the default), weeks run continuously
// across month boundaries via the reconciled absDayIndex, guarded against a
// negative modulo for a year before the calendar's epoch.
func (c *Calendar) WeekdayIndex(year, month, day int) int {
	wl := c.WeekLength()
	if wl <= 0 {
		return 0
	}
	if c.MonthStartsNewWeek && !c.UsesRealTime() {
		if c.monthIsIntercalary(month) {
			return -1
		}
		return (day - 1) % wl
	}
	return ((c.absDayIndex(year, month, day) % wl) + wl) % wl
}

// OccursOn reports whether the event lands on (year, month, day) for cal.
// The single recurrence-expansion predicate; every grid/list projection
// routes through it.
//
// Non-recurring (or legacy/unknown recurrence_type) events match only their
// stored date. Recurring types expand forward from the base date:
//   - weekly/biweekly/custom: every (interval × week) days, base-anchored.
//   - monthly: same day-of-month every interval months, skipped (not
//     clamped) when the month is too short (leap-aware via MonthDays).
//   - yearly: same month/day every interval years, skipped when the year's
//     month is too short for that day.
//
// Recurrence stops at the recurrence-end date (inclusive) and/or after
// RecurrenceMaxOccurrences.
//
// Multi-day events are not expanded here — OccursOn answers only "does the
// rule put an instance here", not "is this day inside the stored window";
// each consumer decides that separately.
// TODO(keyxmakerx/Chronicle#741): render a multi-day event's span as one
// continuous bar (currently only its start date gets marked).
func (e Event) OccursOn(cal *Calendar, year, month, day int) bool {
	onBase := e.Year == year && e.Month == month && e.Day == day
	if !e.IsRecurring || e.RecurrenceType == nil || cal == nil {
		return onBase
	}
	switch *e.RecurrenceType {
	case RecurrenceWeekly, RecurrenceBiWeekly, RecurrenceMonthly, RecurrenceCustom, RecurrenceYearly:
		// expanded below
	default:
		return onBase // legacy / unknown type → single occurrence
	}

	base := cal.absDayIndex(e.Year, e.Month, e.Day)
	target := cal.absDayIndex(year, month, day)
	if target < base {
		return false // recurrence only moves forward from the base date
	}
	if e.RecurrenceEndYear != nil && e.RecurrenceEndMonth != nil && e.RecurrenceEndDay != nil {
		if target > cal.absDayIndex(*e.RecurrenceEndYear, *e.RecurrenceEndMonth, *e.RecurrenceEndDay) {
			return false // past the recurrence-end date
		}
	}

	// YEARLY — the festival branch.
	//
	// A missing day is skipped, never clamped: where the base day does not
	// exist in a later year (a leap day, an intercalary day a cycle omits),
	// the occurrence simply does not happen, rather than silently landing on
	// the wrong day. MonthDays already answers "does this day exist in this
	// year", so this is a lookup, not new arithmetic.
	if *e.RecurrenceType == RecurrenceYearly {
		if month != e.Month || day != e.Day || day > cal.MonthDays(month-1, year) {
			return false
		}
		step := 1
		if e.RecurrenceInterval != nil && *e.RecurrenceInterval > 1 {
			step = *e.RecurrenceInterval
		}
		n := year - e.Year
		if n < 0 || n%step != 0 {
			return false
		}
		// The cap counts occurrences, not years — n/step is the 0-based
		// occurrence index, the same quantity the monthly and week-based
		// branches compare.
		if e.RecurrenceMaxOccurrences != nil && n/step >= *e.RecurrenceMaxOccurrences {
			return false
		}
		return true
	}

	if *e.RecurrenceType == RecurrenceMonthly {
		if day != e.Day || day > cal.MonthDays(month-1, year) {
			return false
		}
		// Monthly honours recurrence_interval the same way the week-based
		// branch below applies its interval through recurrenceWeeks — counted
		// in months rather than weeks.
		// A monthly event with no interval, a zero, or a negative one does not
		// move; only a stored interval of 2 or more changes the expansion.
		step := 1
		if e.RecurrenceInterval != nil && *e.RecurrenceInterval > 1 {
			step = *e.RecurrenceInterval
		}
		n := monthsBetween(cal, e.Year, e.Month, year, month)
		if n < 0 || n%step != 0 {
			return false
		}
		// The cap counts occurrences, not months: n is the whole-month offset
		// from the base date, so with an interval it must be divided by step
		// to get the 0-based occurrence index — the same quantity `diff/stride`
		// is in the week-based branch below.
		if e.RecurrenceMaxOccurrences != nil && n/step >= *e.RecurrenceMaxOccurrences {
			return false
		}
		return true
	}

	// Week-based (weekly / biweekly / custom).
	wl := cal.WeekLength()
	stride := wl * recurrenceWeeks(*e.RecurrenceType, e.RecurrenceInterval)
	if stride <= 0 {
		return onBase
	}
	diff := target - base
	if diff%stride != 0 {
		return false
	}
	if e.RecurrenceMaxOccurrences != nil && diff/stride >= *e.RecurrenceMaxOccurrences {
		return false // occurrence index (0-based) past the cap
	}
	return true
}

// recurrenceWeeks maps a week-based recurrence type to its week interval.
func recurrenceWeeks(rtype string, interval *int) int {
	switch rtype {
	case RecurrenceBiWeekly:
		return 2
	case RecurrenceCustom:
		if interval != nil && *interval > 0 {
			return *interval
		}
	}
	return 1 // weekly (and custom with no/invalid interval)
}

// monthsBetween returns the whole-month offset from (y1,m1) to (y2,m2) using the
// calendar's (constant) month count. Negative when (y2,m2) precedes (y1,m1).
func monthsBetween(cal *Calendar, y1, m1, y2, m2 int) int {
	mc := len(cal.Months)
	if mc == 0 {
		return 0
	}
	return (y2-y1)*mc + (m2 - m1)
}

// MonthName returns the name of the given 1-based month, or a numeric
// fallback ("Month N") when Months isn't loaded far enough (a list read that
// didn't eager-load sub-resources, or a month index outside the calendar's
// own range).
func (c *Calendar) MonthName(month int) string {
	idx := month - 1
	if idx >= 0 && idx < len(c.Months) {
		return c.Months[idx].Name
	}
	return fmt.Sprintf("Month %d", month)
}

// CurrentMonthName is MonthName for the calendar's own current month.
func (c *Calendar) CurrentMonthName() string {
	return c.MonthName(c.CurrentMonth)
}

// FullDateLabel formats the calendar's current date as "<Month> <Day>,
// <Year>" (e.g. "Deepwinter 12, 1024") — the calendars list card and preview
// panel's one-line "world date".
func (c *Calendar) FullDateLabel() string {
	return fmt.Sprintf("%s %d, %d", c.CurrentMonthName(), c.CurrentDay, c.CurrentYear)
}

// FormatCurrentTime returns the current time formatted as "HH:MM".
// Pads hours/minutes with leading zeros based on the max values
// (e.g. a 24-hour system uses 2 digits, a 100-hour system uses 3).
func (c *Calendar) FormatCurrentTime() string {
	return fmt.Sprintf("%02d:%02d", c.CurrentHour, c.CurrentMinute)
}

// CurrentSeason returns the season for the current date, or nil if none match.
func (c *Calendar) CurrentSeason() *Season {
	return c.SeasonForDate(c.CurrentMonth, c.CurrentDay)
}

// SeasonForDate returns the season containing the given month+day, or nil.
func (c *Calendar) SeasonForDate(month, day int) *Season {
	for i := range c.Seasons {
		s := &c.Seasons[i]
		if s.ContainsDate(month, day) {
			return s
		}
	}
	return nil
}

// CurrentEra returns the era containing the current date, or nil if none
// match. Day-granular (EraForDate), since the current date is always known to
// full precision.
func (c *Calendar) CurrentEra() *Era {
	return c.EraForDate(c.CurrentYear, c.CurrentMonth, c.CurrentDay)
}

// EraForYear returns the era containing the given year, or nil if none match,
// ignoring month/day — for callers that only have a year (e.g. legacy data
// with no finer precision). An era with nil EndYear is ongoing (matches every
// year >= StartYear). Prefer EraForDate when month/day are known: a year-only
// check can misplace a date that falls before an era's start day or after its
// end day within the boundary years.
func (c *Calendar) EraForYear(year int) *Era {
	for i := range c.Eras {
		e := &c.Eras[i]
		if year >= e.StartYear && (e.EndYear == nil || year <= *e.EndYear) {
			return e
		}
	}
	return nil
}

// EraForDate returns the era containing the given date, or nil if none match.
// Comparisons are lexicographic over (year, month, day) so an era's start/end
// day within its boundary years is honoured, not just the year.
func (c *Calendar) EraForDate(year, month, day int) *Era {
	for i := range c.Eras {
		e := &c.Eras[i]
		if e.ContainsDate(year, month, day) {
			return e
		}
	}
	return nil
}

// AbsoluteDay returns the total number of days from year 0 day 0 to the given
// date (year, month 1-indexed, day). Used for moon phase calculation.
//
// The year term MUST stay closed-form: `year` reaches this from unauthenticated
// query strings on public seed routes, and a per-year summation loop is an
// O(year) denial-of-service (TestAbsoluteDayClosedFormIsNotLinear). Do not
// reintroduce a bound on the input instead — a fantasy calendar's year number
// is authored data, so the fix belongs in the algorithm.
//
// The arithmetic is exactly the loop's, term for term, so no stored date moves:
//
//	Σ_{y=0}^{year-1} YearLengthForYear(y)
//	  = Σ (YearLength() + leapExtraDays if IsLeapYear(y))
//	  = year·YearLength() + leapExtraDays·|{y ∈ [0,year) : IsLeapYear(y)}|
//
// and leapYearsBefore counts that set in O(1). A negative or zero `year`
// contributes nothing.
func (c *Calendar) AbsoluteDay(year, month, day int) int {
	total := 0
	// Add full years — closed form, see the function comment.
	if year > 0 {
		total = year * c.YearLength()
		if extra := c.leapExtraDays(); extra != 0 {
			total += extra * c.leapYearsBefore(year)
		}
	}
	// Add full months in the current year.
	for i := 0; i < month-1 && i < len(c.Months); i++ {
		total += c.MonthDays(i, year)
	}
	total += day
	return total
}

// leapExtraDays is the number of days a leap year adds over a common one: the
// sum of every month's LeapYearDays, which is exactly the difference between
// YearLengthForYear(leap) and YearLength().
func (c *Calendar) leapExtraDays() int {
	total := 0
	for _, m := range c.Months {
		total += m.LeapYearDays
	}
	return total
}

// leapYearsBefore counts the leap years in [0, year) in O(1) — the counting
// half of AbsoluteDay's closed form.
//
// IsLeapYear(y) is `(y-LeapYearOffset) % LeapYearEvery == 0`, and a Go
// remainder of zero means exact divisibility for either sign, so the predicate
// is plain congruence: y ≡ LeapYearOffset (mod LeapYearEvery). Reducing the
// offset to its least non-negative residue r turns the count into "how many
// members of the arithmetic progression r, r+e, r+2e, … are below year", which
// is (year-1-r)/e + 1 once year exceeds r, and 0 otherwise. Every division here
// is on non-negative operands, so Go's truncating integer division is floor
// division and the formula is exact.
func (c *Calendar) leapYearsBefore(year int) int {
	e := c.LeapYearEvery
	if e <= 0 || year <= 0 {
		return 0 // IsLeapYear is unconditionally false with no modulus
	}
	r := c.LeapYearOffset % e
	if r < 0 {
		r += e
	}
	if year <= r {
		return 0
	}
	return (year-1-r)/e + 1
}

// CurrentAbsoluteDay returns AbsoluteDay for the current date.
func (c *Calendar) CurrentAbsoluteDay() int {
	return c.AbsoluteDay(c.CurrentYear, c.CurrentMonth, c.CurrentDay)
}

// HasRealAnchor reports whether this calendar's real-date anchor is fully
// set — see AnchorYear's doc comment for why it is all four fields or none.
func (c *Calendar) HasRealAnchor() bool {
	return c.AnchorYear != nil && c.AnchorMonth != nil && c.AnchorDay != nil && c.AnchorRealDate != nil
}

// Month is a named period in the calendar with a configurable number of days.
type Month struct {
	ID            int    `json:"id"`
	CalendarID    string `json:"calendar_id"`
	Name          string `json:"name"`
	Days          int    `json:"days"`
	SortOrder     int    `json:"sort_order"`
	IsIntercalary bool   `json:"is_intercalary"`
	LeapYearDays  int    `json:"leap_year_days"`
}

// Weekday is a named day in the repeating weekly cycle.
type Weekday struct {
	ID         int    `json:"id"`
	CalendarID string `json:"calendar_id"`
	Name       string `json:"name"`
	SortOrder  int    `json:"sort_order"`
	IsRestDay  bool   `json:"is_rest_day"`
}

// Moon is a celestial body with a phase cycle used for moon phase display.
//
// BaseDesign / Tint / PhaseSource / Size / OrbitSpeed are the moon-library
// render params, mirroring the showcase's MOON_DESIGNS parameters so a
// moon's appearance can be authored rather than hardcoded in JS.
type Moon struct {
	ID          int     `json:"id"`
	CalendarID  string  `json:"calendar_id"`
	Name        string  `json:"name"`
	CycleDays   float64 `json:"cycle_days"`
	PhaseOffset float64 `json:"phase_offset"`
	Color       string  `json:"color"`
	BaseDesign  string  `json:"base_design"`
	Tint        *string `json:"tint,omitempty"`
	PhaseSource string  `json:"phase_source"`
	Size        float64 `json:"size"`
	OrbitSpeed  float64 `json:"orbit_speed"`
	// HiddenFromPlayers lets the Director hide a moon from players entirely
	// (undiscovered, or secret world lore) while it still renders for the GM.
	HiddenFromPlayers bool `json:"hidden_from_players"`
}

// MoonPhase returns the phase (0.0–1.0) of this moon on a given absolute day
// number (days since year 0 day 0). 0=new, 0.25=first quarter, 0.5=full,
// 0.75=last quarter.
func (m *Moon) MoonPhase(absoluteDay int) float64 {
	if m.CycleDays <= 0 {
		return 0
	}
	raw := (float64(absoluteDay) + m.PhaseOffset) / m.CycleDays
	phase := raw - float64(int(raw))
	if phase < 0 {
		phase += 1
	}
	return phase
}

// MoonPhaseName returns a human-readable phase name.
func (m *Moon) MoonPhaseName(absoluteDay int) string {
	phase := m.MoonPhase(absoluteDay)
	switch {
	case phase < 0.125:
		return "New Moon"
	case phase < 0.25:
		return "Waxing Crescent"
	case phase < 0.375:
		return "First Quarter"
	case phase < 0.5:
		return "Waxing Gibbous"
	case phase < 0.625:
		return "Full Moon"
	case phase < 0.75:
		return "Waning Gibbous"
	case phase < 0.875:
		return "Last Quarter"
	default:
		return "Waning Crescent"
	}
}

// MoonPhaseIcon returns an icon identifier for the current phase.
func (m *Moon) MoonPhaseIcon(absoluteDay int) string {
	phase := m.MoonPhase(absoluteDay)
	switch {
	case phase < 0.125:
		return "circle-dot"
	case phase < 0.25:
		return "moon-waxing-crescent"
	case phase < 0.375:
		return "moon-first-quarter"
	case phase < 0.5:
		return "moon-waxing-gibbous"
	case phase < 0.625:
		return "moon"
	case phase < 0.75:
		return "moon-waning-gibbous"
	case phase < 0.875:
		return "moon-last-quarter"
	default:
		return "moon-waning-crescent"
	}
}

// Season is a named period spanning a range of month+day to month+day.
type Season struct {
	ID            int     `json:"id"`
	CalendarID    string  `json:"calendar_id"`
	Name          string  `json:"name"`
	StartMonth    int     `json:"start_month"`
	StartDay      int     `json:"start_day"`
	EndMonth      int     `json:"end_month"`
	EndDay        int     `json:"end_day"`
	Description   *string `json:"description,omitempty"`
	Color         string  `json:"color"`
	WeatherEffect *string `json:"weather_effect,omitempty"`
}

// ContainsDate returns true if the given month+day falls within this season.
// Handles wrap-around (e.g. Winter: month 11 day 1 → month 2 day 28).
func (s *Season) ContainsDate(month, day int) bool {
	startVal := s.StartMonth*100 + s.StartDay
	endVal := s.EndMonth*100 + s.EndDay
	dateVal := month*100 + day

	if startVal <= endVal {
		// Normal range (e.g. Spring: 3/1 → 5/31).
		return dateVal >= startVal && dateVal <= endVal
	}
	// Wrap-around (e.g. Winter: 11/1 → 2/28).
	return dateVal >= startVal || dateVal <= endVal
}

// Era is a named time period spanning a range of dates (e.g. "First Age",
// "Age of Fire"). An era begins on a day, not just a year: StartMonth/
// StartDay give that day within StartYear. EndYear nil means ongoing;
// EndMonth/EndDay are set together with EndYear (all three or none — the
// same all-or-nothing shape Calendar's real-date anchor uses).
type Era struct {
	ID          int     `json:"id"`
	CalendarID  string  `json:"calendar_id"`
	Name        string  `json:"name"`
	StartYear   int     `json:"start_year"`
	StartMonth  int     `json:"start_month"`
	StartDay    int     `json:"start_day"`
	EndYear     *int    `json:"end_year,omitempty"` // nil = ongoing
	EndMonth    *int    `json:"end_month,omitempty"`
	EndDay      *int    `json:"end_day,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       string  `json:"color"`
	SortOrder   int     `json:"sort_order"`
}

// IsOngoing returns true if this era has no end year (still in progress).
func (e *Era) IsOngoing() bool {
	return e.EndYear == nil
}

// ContainsYear returns true if the given year falls within this era,
// ignoring month/day — see EraForYear's doc comment for when to prefer this
// over ContainsDate.
func (e *Era) ContainsYear(year int) bool {
	return year >= e.StartYear && (e.EndYear == nil || year <= *e.EndYear)
}

// ContainsDate returns true if the given date falls within this era, honoring
// its start/end day rather than just the boundary years. Comparisons are
// lexicographic over (year, month, day). An end year with no end month/day
// (an era imported without day-level bounds) includes its whole end year,
// rather than only its first day.
func (e *Era) ContainsDate(year, month, day int) bool {
	if dateLess(year, month, day, e.StartYear, e.StartMonth, e.StartDay) {
		return false
	}
	if e.EndYear == nil {
		return true
	}
	if e.EndMonth == nil || e.EndDay == nil {
		return year <= *e.EndYear
	}
	return !dateLess(*e.EndYear, *e.EndMonth, *e.EndDay, year, month, day)
}

// dateLess reports whether (y1,m1,d1) sorts strictly before (y2,m2,d2).
func dateLess(y1, m1, d1, y2, m2, d2 int) bool {
	if y1 != y2 {
		return y1 < y2
	}
	if m1 != m2 {
		return m1 < m2
	}
	return d1 < d2
}

// Event is a calendar entry on a specific date, optionally linked to an entity.
// Description stores ProseMirror JSON for rich text editing; DescriptionHTML
// stores pre-rendered sanitized HTML for display (same pattern as entity entries).
type Event struct {
	ID                       string  `json:"id"`
	CalendarID               string  `json:"calendar_id"`
	EntityID                 *string `json:"entity_id,omitempty"`
	Name                     string  `json:"name"`
	Description              *string `json:"description,omitempty"`
	DescriptionHTML          *string `json:"description_html,omitempty"`
	Year                     int     `json:"year"`
	Month                    int     `json:"month"`
	Day                      int     `json:"day"`
	StartHour                *int    `json:"start_hour,omitempty"`
	StartMinute              *int    `json:"start_minute,omitempty"`
	EndYear                  *int    `json:"end_year,omitempty"`
	EndMonth                 *int    `json:"end_month,omitempty"`
	EndDay                   *int    `json:"end_day,omitempty"`
	EndHour                  *int    `json:"end_hour,omitempty"`
	EndMinute                *int    `json:"end_minute,omitempty"`
	IsRecurring              bool    `json:"is_recurring"`
	RecurrenceType           *string `json:"recurrence_type,omitempty"`
	RecurrenceInterval       *int    `json:"recurrence_interval,omitempty"`
	RecurrenceEndYear        *int    `json:"recurrence_end_year,omitempty"`
	RecurrenceEndMonth       *int    `json:"recurrence_end_month,omitempty"`
	RecurrenceEndDay         *int    `json:"recurrence_end_day,omitempty"`
	RecurrenceMaxOccurrences *int    `json:"recurrence_max_occurrences,omitempty"`
	RecurrenceDayOfWeek      *int    `json:"recurrence_day_of_week,omitempty"`
	Visibility               string  `json:"visibility"`
	VisibilityRules          *string `json:"visibility_rules,omitempty"`
	// KindID references one of the campaign's calendar_event_kinds by id.
	// Nil means no kind assigned.
	KindID *int `json:"kind_id,omitempty"`
	// Announced is this event's override of its kind's DefaultAnnounced
	// ("ahead" or "on_day" — see EventKind). Nil means "use the kind's
	// default", falling back to "on_day" when the event has no kind, the same
	// nil-means-inherit shape Tier already uses on this struct.
	Announced *string `json:"announced,omitempty"`
	// Tier references one of the campaign's event_tier_definitions by slug.
	// Nil means "use platform default" (render-time fallback).
	Tier   *string `json:"tier,omitempty"`
	Color  *string `json:"color,omitempty"`
	Icon   *string `json:"icon,omitempty"`
	AllDay bool    `json:"all_day"`
	// Payload is one small nullable JSON slot for typed extras that vary by
	// event — a moon night, a sky event — so a new one doesn't need another
	// migration. Raw JSON text, parsed on demand via ParsePayload; nil means
	// no payload. See MoonNightPayload for the one currently-defined shape.
	Payload   *string   `json:"payload,omitempty"`
	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Joined fields for display (populated by some queries).
	EntityName  string `json:"entity_name,omitempty"`
	EntityIcon  string `json:"entity_icon,omitempty"`
	EntityColor string `json:"entity_color,omitempty"`
	// KindName/KindIcon/KindColor/KindSlug are joined from calendar_event_kinds
	// when KindID is set; empty when it is nil.
	KindName  string `json:"kind_name,omitempty"`
	KindSlug  string `json:"kind_slug,omitempty"`
	KindIcon  string `json:"kind_icon,omitempty"`
	KindColor string `json:"kind_color,omitempty"`
}

// HasTime returns true if this event has a specific start time (not all-day).
func (e *Event) HasTime() bool {
	return e.StartHour != nil && e.StartMinute != nil
}

// FormatTime returns the event's start time as "HH:MM", or empty for all-day events.
func (e *Event) FormatTime() string {
	if !e.HasTime() {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", *e.StartHour, *e.StartMinute)
}

// FormatEndTime returns the event's end time as "HH:MM", or empty if not set.
func (e *Event) FormatEndTime() string {
	if e.EndHour == nil || e.EndMinute == nil {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", *e.EndHour, *e.EndMinute)
}

// FormatTimeRange returns "HH:MM - HH:MM" or just "HH:MM" if no end time.
func (e *Event) FormatTimeRange() string {
	start := e.FormatTime()
	if start == "" {
		return ""
	}
	end := e.FormatEndTime()
	if end == "" {
		return start
	}
	return start + " – " + end
}

// IsMultiDay returns true if this event spans more than one day.
func (e *Event) IsMultiDay() bool {
	return e.EndYear != nil && e.EndMonth != nil && e.EndDay != nil
}

// ParseVisibilityRules parses the JSON visibility rules into a VisibilityRules struct.
// Returns nil if no rules are set.
func (e *Event) ParseVisibilityRules() *VisibilityRules {
	return ParseVisibilityRules(e.VisibilityRules)
}

// Announced setting constants: separate from Visibility (who can ever see an
// event) and from an event's date (when it happens). AnnouncedAhead means
// players may know about an event before it happens (a festival's date is
// common knowledge); AnnouncedOnDay means it only becomes knowable on the
// day itself.
const (
	AnnouncedAhead = "ahead"
	AnnouncedOnDay = "on_day"
)

// AnnouncedValues is the accepted set, for input validation — mirrors the
// RecurrenceTypes pattern above.
var AnnouncedValues = []string{AnnouncedAhead, AnnouncedOnDay}

// IsSupportedAnnounced reports whether a is a recognized Announced value.
func IsSupportedAnnounced(a string) bool {
	for _, ok := range AnnouncedValues {
		if a == ok {
			return true
		}
	}
	return false
}

// EffectiveAnnounced resolves this event's announced setting, in order:
// the event's own override; "ahead" if the event repeats yearly (a yearly
// festival is expected knowledge regardless of its kind); its kind's
// default, only when kind actually matches e.KindID (a caller must not pass
// an unrelated kind and have it apply); otherwise "on_day".
func (e *Event) EffectiveAnnounced(kind *EventKind) string {
	if e.Announced != nil && *e.Announced != "" {
		return *e.Announced
	}
	if e.IsRecurring && e.RecurrenceType != nil && *e.RecurrenceType == RecurrenceYearly {
		return AnnouncedAhead
	}
	if kind != nil && e.KindID != nil && kind.ID == *e.KindID && kind.DefaultAnnounced != "" {
		return kind.DefaultAnnounced
	}
	return AnnouncedOnDay
}

// Hemisphere constants for Calendar.Hemisphere.
const (
	HemisphereNorth = "north"
	HemisphereSouth = "south"
)

// Moon night payload: the "type" a calendar_events.payload JSON blob carries
// for a moon-themed night, e.g. `{"type":"blood","moons":[3]}`. moons holds
// the ids of the calendar_moons rows the night applies to.
const (
	MoonNightBlood       = "blood"
	MoonNightMagic       = "magic"
	MoonNightConjunction = "conjunction"
	MoonNightEclipse     = "eclipse"
	MoonNightHarvest     = "harvest"
)

// MoonNightPayload is one shape an Event.Payload JSON blob can carry.
// "a sky event" is a second shape the same column is meant to hold — its
// fields aren't decided yet, so it isn't typed here.
type MoonNightPayload struct {
	Type  string `json:"type"`
	Moons []int  `json:"moons,omitempty"`
}

// IsMoonNightType reports whether t is one of the five recognized moon-night
// types.
func IsMoonNightType(t string) bool {
	switch t {
	case MoonNightBlood, MoonNightMagic, MoonNightConjunction, MoonNightEclipse, MoonNightHarvest:
		return true
	}
	return false
}

// ParsePayload parses Event.Payload as a MoonNightPayload. Returns nil when
// there is no payload, it isn't valid JSON, or its type is not one of the
// MoonNight* constants (a future non-moon-night payload, or JSON "null",
// which unmarshals into a zero-value, untyped struct) — callers that need to
// tell those apart should parse e.Payload themselves instead.
func (e *Event) ParsePayload() *MoonNightPayload {
	if e.Payload == nil || *e.Payload == "" {
		return nil
	}
	var p MoonNightPayload
	if err := json.Unmarshal([]byte(*e.Payload), &p); err != nil {
		return nil
	}
	if !IsMoonNightType(p.Type) {
		return nil
	}
	return &p
}

// HasRichText returns true if this event has a rich text description (ProseMirror JSON
// with pre-rendered HTML), as opposed to a legacy plain text description.
func (e *Event) HasRichText() bool {
	return e.DescriptionHTML != nil && *e.DescriptionHTML != ""
}

// PlainDescription returns a plain text version of the description for tooltips.
// For rich text events, returns empty (tooltip should not show raw JSON).
// For legacy plain text events, returns the description as-is.
func (e *Event) PlainDescription() string {
	if e.Description == nil || *e.Description == "" {
		return ""
	}
	// If there's no HTML version, description is plain text (legacy).
	if !e.HasRichText() {
		return *e.Description
	}
	// Rich text event: description is ProseMirror JSON, not displayable as text.
	return ""
}

// --- Request DTOs ---

// CreateCalendarInput is the validated input for creating a calendar.
//
// Visibility/VisibilityRules default to "everyone" when left blank (the
// ordinary create-a-calendar route never sets them), mirroring
// CreateEventInput's own default. The route that exposes calendar creation
// is Owner-only, so no separate CanAuthorDmOnly gate is needed the way
// CreateEvent needs one for Scribes.
type CreateCalendarInput struct {
	Mode             string // "fantasy" or "reallife"
	Name             string
	Description      *string
	EpochName        *string
	CurrentYear      int
	HoursPerDay      int
	MinutesPerHour   int
	SecondsPerMinute int
	LeapYearEvery    int
	LeapYearOffset   int
	Visibility       string
	VisibilityRules  *string
}

// CreateCalendarFromImportOptions carries the caller's explicit choice for
// the calendar CreateCalendarFromImport is about to create's current
// ("today") date. CreateCalendarFromImport never falls back to an
// undocumented default (#741 — "an import never silently resets the
// calendar's current date"): when the import itself left a field of
// ImportResult.Today unspecified (Month/Day nil — Calendaria only ever
// determines a year), the matching field here MUST be set or
// CreateCalendarFromImport returns a validation error instead of silently
// picking day 1. CurrentYear is optional even then — the import always
// determines a year, and this only overrides it when the caller wants the
// calendar to start somewhere else than the source file did.
type CreateCalendarFromImportOptions struct {
	CurrentYear  *int
	CurrentMonth *int
	CurrentDay   *int
}

// UpdateCalendarInput is the validated input for updating calendar settings.
// Partial update (internal/patch's Field type): an absent field preserves
// the stored value, an explicit null clears it, a present value replaces it.
// Name stays a plain string: UpdateCalendar rejects a blank merged name with
// 400, so an absent name fails loudly instead of silently overwriting — the
// same governedFieldExceptions shape as maps.UpdateMapInput.Name and
// timeline.UpdateTimelineInput.Name. Mode, when present, must be one of the
// calendar Mode constants (ModeFantasy / ModeRealLife).
type UpdateCalendarInput struct {
	Name             string
	Description      patch.Field[string]
	EpochName        patch.Field[string]
	Mode             patch.Field[string]
	CurrentYear      patch.Field[int]
	CurrentMonth     patch.Field[int]
	CurrentDay       patch.Field[int]
	CurrentHour      patch.Field[int]
	CurrentMinute    patch.Field[int]
	HoursPerDay      patch.Field[int]
	MinutesPerHour   patch.Field[int]
	SecondsPerMinute patch.Field[int]
	LeapYearEvery    patch.Field[int]
	LeapYearOffset   patch.Field[int]
	// Hemisphere flips how a real-world calendar's seasons read (see
	// Calendar.Hemisphere's doc comment). When present, must be
	// HemisphereNorth or HemisphereSouth. Setting it for the first time on a
	// real-life calendar with no seasons yet auto-seeds the four default
	// seasons for that hemisphere (see UpdateCalendar and
	// defaultRealLifeSeasons) — a calendar that already has any seasons is
	// never touched by this field.
	Hemisphere patch.Field[string]
	// SetRealTime is nil for every caller that does not manage the flag (e.g.
	// PutDate, worldstate advance/time, seed/create), so their update
	// preserves the stored TracksRealTime/RealTimeZone — a *bool so "absent"
	// (leave unchanged) is distinct from "false" (disable). RealTimeZone is
	// the IANA anchor, required and validated when enabling; ignored and
	// cleared when disabling.
	SetRealTime  *bool
	RealTimeZone *string
}

// CreateEventInput is the validated input for creating a calendar event.
type CreateEventInput struct {
	Name                     string
	Description              *string
	DescriptionHTML          *string
	EntityID                 *string
	Year                     int
	Month                    int
	Day                      int
	StartHour                *int
	StartMinute              *int
	EndYear                  *int
	EndMonth                 *int
	EndDay                   *int
	EndHour                  *int
	EndMinute                *int
	IsRecurring              bool
	RecurrenceType           *string
	RecurrenceInterval       *int
	RecurrenceEndYear        *int
	RecurrenceEndMonth       *int
	RecurrenceEndDay         *int
	RecurrenceMaxOccurrences *int
	Visibility               string
	VisibilityRules          *string
	// KindID references calendar_event_kinds.id; nil = no kind.
	KindID *int
	// Announced: nil = inherit from the kind's default (see
	// Event.EffectiveAnnounced).
	Announced *string
	// Tier — campaign tier definition slug; nil = use platform default
	// at render.
	Tier      *string
	Color     *string
	Icon      *string
	AllDay    bool
	Payload   *string
	CreatedBy string
	// CanAuthorDmOnly is set by the handler from the request viewer
	// (v.SkipsPerUserRules(), true for an Owner or a granted co-DM) before
	// this reaches the service: CreateEvent has no stored row to compare a
	// visibility change against the way UpdateEvent does, so a create with
	// Visibility "dm_only" (or a non-empty VisibilityRules) is refused
	// outright for a caller this is false for, rather than silently
	// downgraded. Zero value (false) is "not authorized" — a caller that
	// never sets it is never trusted by default.
	CanAuthorDmOnly bool
}

// UpdateEventInput is the validated input for updating an event.
//
// Partial update: an absent key preserves the stored value, an explicit null
// clears it, a present value replaces it (internal/patch's Field type). Every
// field uses patch.Field, including EntityID: a plain nil-preserving pointer
// would have made the link impossible to clear, so it needs the three-way
// absent/null/value distinction like the rest.
type UpdateEventInput struct {
	Name                     patch.Field[string]
	Description              patch.Field[string]
	DescriptionHTML          patch.Field[string]
	EntityID                 patch.Field[string]
	Year                     patch.Field[int]
	Month                    patch.Field[int]
	Day                      patch.Field[int]
	StartHour                patch.Field[int]
	StartMinute              patch.Field[int]
	EndYear                  patch.Field[int]
	EndMonth                 patch.Field[int]
	EndDay                   patch.Field[int]
	EndHour                  patch.Field[int]
	EndMinute                patch.Field[int]
	IsRecurring              patch.Field[bool]
	RecurrenceType           patch.Field[string]
	RecurrenceInterval       patch.Field[int]
	RecurrenceEndYear        patch.Field[int]
	RecurrenceEndMonth       patch.Field[int]
	RecurrenceEndDay         patch.Field[int]
	RecurrenceMaxOccurrences patch.Field[int]
	Visibility               patch.Field[string]
	VisibilityRules          patch.Field[string]
	KindID                   patch.Field[int]
	Announced                patch.Field[string]
	// Tier — see CreateEventInput.Tier doc.
	Tier    patch.Field[string]
	Color   patch.Field[string]
	Icon    patch.Field[string]
	AllDay  patch.Field[bool]
	Payload patch.Field[string]
}

// MonthInput is the input for creating/updating a month.
type MonthInput struct {
	Name          string `json:"name"`
	Days          int    `json:"days"`
	SortOrder     int    `json:"sort_order"`
	IsIntercalary bool   `json:"is_intercalary"`
	LeapYearDays  int    `json:"leap_year_days"`
}

// WeekdayInput is the input for creating/updating a weekday.
type WeekdayInput struct {
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	IsRestDay bool   `json:"is_rest_day"`
}

// MoonInput is the input for creating/updating a moon.
type MoonInput struct {
	// ID is nil for a new moon, or an existing calendar_moons id to update in
	// place. SetMoons upserts on it, so an id present in the calendar but
	// absent from the input list is deleted, and one present in both is
	// updated rather than replaced. An update never touches
	// HiddenFromPlayers or the render parameters (this input carries no
	// render parameters at all); a fresh insert sets HiddenFromPlayers from
	// this field, which is how an imported hidden moon stays hidden.
	ID                *int    `json:"id,omitempty"`
	Name              string  `json:"name"`
	CycleDays         float64 `json:"cycle_days"`
	PhaseOffset       float64 `json:"phase_offset"`
	Color             string  `json:"color"`
	HiddenFromPlayers bool    `json:"hidden_from_players,omitempty"`
}

// EraInput is the input for creating/updating an era. Field order matches
// ExportEra, which converts directly to/from this type via a Go type
// conversion in import.go; keep the two in lockstep.
type EraInput struct {
	// ID is nil for a new era, or an existing calendar_eras id to update in
	// place. SetEras upserts on it, so an id present in the calendar but
	// absent from the input list is deleted, and one present in both is
	// updated rather than replaced (preserving entity_era_links, which
	// cascade-delete when the era row itself is deleted and reinserted).
	// Always nil coming from an import file, which has no Chronicle ids.
	ID          *int    `json:"id,omitempty"`
	Name        string  `json:"name"`
	StartYear   int     `json:"start_year"`
	StartMonth  int     `json:"start_month"`
	StartDay    int     `json:"start_day"`
	EndYear     *int    `json:"end_year"`
	EndMonth    *int    `json:"end_month"`
	EndDay      *int    `json:"end_day"`
	Description *string `json:"description"`
	Color       string  `json:"color"`
	SortOrder   int     `json:"sort_order"`
}

// UpdateEraInput is the validated PARTIAL-update input for an existing era:
// an absent field preserves the stored value, an explicit null clears it, a
// present value replaces it. Name stays a plain string — UpdateEra rejects a
// blank merged name with 400, so an absent name fails loudly instead of
// silently overwriting (the same governedFieldExceptions shape as
// calendar.UpdateCalendarInput.Name).
type UpdateEraInput struct {
	Name        string
	StartYear   patch.Field[int]
	StartMonth  patch.Field[int]
	StartDay    patch.Field[int]
	EndYear     patch.Field[int]
	EndMonth    patch.Field[int]
	EndDay      patch.Field[int]
	Description patch.Field[string]
	Color       patch.Field[string]
	SortOrder   patch.Field[int]
}

// EventKind is a campaign-defined event kind (category) for calendar events.
// Kinds are shared by every calendar in a campaign — keyed by CampaignID, not
// a single calendar — and carry a slug (referenced by events), display name,
// emoji icon, color, and the default Announced setting new events of this
// kind get unless overridden (see Event.EffectiveAnnounced).
type EventKind struct {
	ID               int    `json:"id"`
	CampaignID       string `json:"campaign_id"`
	Slug             string `json:"slug"`
	Name             string `json:"name"`
	Icon             string `json:"icon"`
	Color            string `json:"color"`
	SortOrder        int    `json:"sort_order"`
	DefaultAnnounced string `json:"default_announced"`
}

// GetCampaignID implements middleware.CampaignScoped for IDOR protection.
func (k *EventKind) GetCampaignID() string {
	return k.CampaignID
}

// EventKindInput is the input for creating/updating an event kind.
type EventKindInput struct {
	Slug             string `json:"slug"`
	Name             string `json:"name"`
	Icon             string `json:"icon"`
	Color            string `json:"color"`
	SortOrder        int    `json:"sort_order"`
	DefaultAnnounced string `json:"default_announced"`
}

// UpdateEventKindInput is the validated PARTIAL-update input for an
// existing event kind: an absent field preserves the stored value, an
// explicit null clears it, a present value replaces it. Name stays a plain
// string for the same reason UpdateCalendarInput.Name does (a blank merged
// name fails loudly rather than silently overwriting).
type UpdateEventKindInput struct {
	Slug             patch.Field[string]
	Name             string
	Icon             patch.Field[string]
	Color            patch.Field[string]
	SortOrder        patch.Field[int]
	DefaultAnnounced patch.Field[string]
}

// Weather represents the current weather state for a calendar.
// Set manually by the GM or synced from external tools (Calendaria).
type Weather struct {
	ID                 int            `json:"id"`
	CalendarID         string         `json:"calendar_id"`
	PresetID           *string        `json:"preset_id,omitempty"`
	PresetLabel        *string        `json:"preset_label,omitempty"`
	Icon               *string        `json:"icon,omitempty"`
	Color              *string        `json:"color,omitempty"`
	TemperatureCelsius *float64       `json:"temperature_celsius,omitempty"`
	Wind               *Wind          `json:"wind,omitempty"`
	Precipitation      *Precipitation `json:"precipitation,omitempty"`
	ZoneID             *string        `json:"zone_id,omitempty"`
	ZoneName           *string        `json:"zone_name,omitempty"`
	Description        *string        `json:"description,omitempty"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// Wind describes wind speed and direction.
type Wind struct {
	SpeedKPH         *float64 `json:"speed_kph,omitempty"`
	SpeedTier        *string  `json:"speed_tier,omitempty"`
	Direction        *string  `json:"direction,omitempty"`
	DirectionDegrees *int     `json:"direction_degrees,omitempty"`
}

// Precipitation describes the type and intensity of precipitation.
type Precipitation struct {
	Type      *string  `json:"type,omitempty"`
	Intensity *float64 `json:"intensity,omitempty"`
}

// WeatherInput is the input for setting weather state.
type WeatherInput struct {
	PresetID               *string  `json:"preset_id"`
	PresetLabel            *string  `json:"preset_label"`
	Icon                   *string  `json:"icon"`
	Color                  *string  `json:"color"`
	TemperatureCelsius     *float64 `json:"temperature_celsius"`
	WindSpeedKPH           *float64 `json:"wind_speed_kph"`
	WindSpeedTier          *string  `json:"wind_speed_tier"`
	WindDirection          *string  `json:"wind_direction"`
	WindDirectionDeg       *int     `json:"wind_direction_degrees"`
	PrecipitationType      *string  `json:"precipitation_type"`
	PrecipitationIntensity *float64 `json:"precipitation_intensity"`
	ZoneID                 *string  `json:"zone_id"`
	ZoneName               *string  `json:"zone_name"`
	Description            *string  `json:"description"`
}

// WeatherZone is a per-calendar climate region definition (e.g.
// "temperate", "tropical", "arctic"). The zone's payload carries the
// active presets + per-season overrides as opaque JSON — the structural
// shape is owned by Calendaria/Foundry sync (presets array,
// season_overrides map) and Chronicle stores it verbatim with a small
// validation helper (see service.go validateWeatherZonePayload).
//
// Zones are calendar-scoped: each calendar has its own zones. The
// active-zone reference lives on calendar_weather.zone_id + zone_name; the
// zone definitions live in this table.
type WeatherZone struct {
	CalendarID string         `json:"calendar_id"`
	ZoneID     string         `json:"zone_id"`
	Name       string         `json:"name"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"created_at,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at,omitempty"`
}

// WeatherZonesState bundles the per-calendar active-zone reference +
// the full zone definitions list — the canonical GET response from
// /api/v1/campaigns/:cid/calendar/weather/zones. ActiveZone is "" when
// no zone is currently active; Zones may be empty.
type WeatherZonesState struct {
	ActiveZone string        `json:"active_zone"`
	Zones      []WeatherZone `json:"zones"`
}

// Cycle is a periodic named cycle (zodiac, elemental, seasonal, etc.).
// Entries rotate based on cycle_length (number of years per full rotation).
type Cycle struct {
	ID          int          `json:"id"`
	CalendarID  string       `json:"calendar_id"`
	Name        string       `json:"name"`
	CycleLength int          `json:"cycle_length"`
	Type        string       `json:"type"` // "yearly", "monthly", etc.
	SortOrder   int          `json:"sort_order"`
	Entries     []CycleEntry `json:"entries,omitempty"`
}

// CycleEntry is a single entry within a cycle.
type CycleEntry struct {
	ID         int     `json:"id"`
	CycleID    int     `json:"cycle_id"`
	Name       string  `json:"name"`
	Icon       *string `json:"icon,omitempty"`
	YearOffset int     `json:"year_offset"`
	SortOrder  int     `json:"sort_order"`
}

// CycleInput is the input for creating/updating a cycle with its entries.
type CycleInput struct {
	Name        string            `json:"name"`
	CycleLength int               `json:"cycle_length"`
	Type        string            `json:"type"`
	SortOrder   int               `json:"sort_order"`
	Entries     []CycleEntryInput `json:"entries"`
}

// CycleEntryInput is the input for a single cycle entry.
type CycleEntryInput struct {
	Name       string  `json:"name"`
	Icon       *string `json:"icon"`
	YearOffset int     `json:"year_offset"`
	SortOrder  int     `json:"sort_order"`
}

// Festival is a fixed calendar entry (holiday) that is part of the calendar
// structure rather than a recurring event. month+day specifies the date;
// after_month is used for intercalary festivals that fall between months.
type Festival struct {
	ID          int     `json:"id"`
	CalendarID  string  `json:"calendar_id"`
	Name        string  `json:"name"`
	Month       *int    `json:"month,omitempty"`
	Day         *int    `json:"day,omitempty"`
	AfterMonth  *int    `json:"after_month,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	SortOrder   int     `json:"sort_order"`
}

// FestivalInput is the input for creating/updating a festival.
type FestivalInput struct {
	Name        string  `json:"name"`
	Month       *int    `json:"month"`
	Day         *int    `json:"day"`
	AfterMonth  *int    `json:"after_month"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
	Icon        *string `json:"icon"`
	SortOrder   int     `json:"sort_order"`
}

// DefaultEventKinds returns the default event kinds seeded for a new
// campaign. DefaultAnnounced records each kind's own default: festivals and
// holidays default to "ahead"; everything else to "on_day". A yearly-
// recurring event overrides its kind's default to "ahead" regardless (see
// Event.EffectiveAnnounced).
func DefaultEventKinds() []EventKindInput {
	return []EventKindInput{
		// Holiday is lime, not amber: gold means "GM only" across the product
		// (a GM-only day strikes all its runes gold), and amber at small sizes
		// is indistinguishable from that gold by hue. This seeds new campaigns
		// only — an existing kind keeps its stored colour; the owner can
		// change it in settings -> Kinds.
		{Slug: "holiday", Name: "Holiday", Icon: "⭐", Color: "#84cc16", SortOrder: 0, DefaultAnnounced: AnnouncedAhead},
		{Slug: "battle", Name: "Battle", Icon: "⚔", Color: "#ef4444", SortOrder: 1, DefaultAnnounced: AnnouncedOnDay},
		{Slug: "quest", Name: "Quest", Icon: "❗", Color: "#8b5cf6", SortOrder: 2, DefaultAnnounced: AnnouncedOnDay},
		{Slug: "birthday", Name: "Birthday", Icon: "🎂", Color: "#ec4899", SortOrder: 3, DefaultAnnounced: AnnouncedOnDay},
		{Slug: "festival", Name: "Festival", Icon: "🎉", Color: "#10b981", SortOrder: 4, DefaultAnnounced: AnnouncedAhead},
		{Slug: "travel", Name: "Travel", Icon: "🚶", Color: "#3b82f6", SortOrder: 5, DefaultAnnounced: AnnouncedOnDay},
	}
}

// defaultRealLifeSeasons returns the four seasons a new real-world calendar
// starts with, per the operator's signed decision: the same four Gregorian
// reference dates (Mar 20 / Jun 21 / Sep 23 / Dec 21) every real-world
// calendar uses, with the season NAMES flipped between hemispheres — a
// northern Spring lands on the same dates as a southern Autumn. An unknown
// hemisphere value returns the northern set (UpdateCalendar validates
// Hemisphere against HemisphereNorth/HemisphereSouth before this is ever
// called, so that case is unreached in practice).
//
// Season.ContainsDate's wrap-around logic (start > end) already handles
// Winter/Summer spanning the Dec 21 → Mar 19 year boundary — see
// TestDefaultRealLifeSeasonsContainsDate.
func defaultRealLifeSeasons(hemisphere string) []Season {
	names := [4]string{"Spring", "Summer", "Autumn", "Winter"}
	if hemisphere == HemisphereSouth {
		names = [4]string{"Autumn", "Winter", "Spring", "Summer"}
	}
	return []Season{
		{Name: names[0], StartMonth: 3, StartDay: 20, EndMonth: 6, EndDay: 20, Color: "#22c55e"},
		{Name: names[1], StartMonth: 6, StartDay: 21, EndMonth: 9, EndDay: 22, Color: "#eab308"},
		{Name: names[2], StartMonth: 9, StartDay: 23, EndMonth: 12, EndDay: 20, Color: "#f97316"},
		{Name: names[3], StartMonth: 12, StartDay: 21, EndMonth: 3, EndDay: 19, Color: "#38bdf8"},
	}
}
