// Package calendar - structure_edit.go is the owner's whole-structure save
// for an EXISTING calendar: months, weekdays, the leap rule, moons and
// seasons in one go. It previews before it writes, because a structure
// change re-dates things the owner may not have thought about.
//
// The rules it keeps:
//   - Events are warned about, never refused and never silently rewritten.
//     The one rewrite it makes is shown in the preview first: an event in a
//     month that keeps its name follows that month to its new place.
//   - Months are matched by name where the name is unchanged; a month whose
//     name changed but whose place did not is treated as renamed.
//   - The current date is never reset. It follows its month like an event,
//     and is clamped (and the preview says so) only when its day is gone.
//   - Moons and seasons keep their ids where the editor sent them back, so
//     anything keyed to a moon's id (its hidden flag, its phases) survives.
package calendar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// maxStructurePreviewEvents caps how many events each preview list names;
// the totals beside them still count every one.
const maxStructurePreviewEvents = 25

// StructureEdit is an owner's whole-structure save for an existing
// calendar. Unlike an Update*Input it is not a partial update: the lists it
// carries are the calendar's complete new months, weekdays, moons and
// seasons. Existing moons and seasons are identified by ID; fields the
// editor does not show (a moon's colour, phase offset and look, a season's
// colour and description) are left as stored rather than echoed back.
type StructureEdit struct {
	Months        []MonthInput
	Weekdays      []WeekdayInput
	Moons         []MoonInput
	Seasons       []Season
	LeapYearEvery int
	// Warnings are clamp notes from validating the submitted structure,
	// passed through to the preview so the owner sees them before saving.
	Warnings []string
}

// StructureEditFromImport takes the structure editor's submission, already
// size-checked and clamped by the same path a wizard build goes through,
// and keeps only the parts a structure save writes.
func StructureEditFromImport(ir *ImportResult) StructureEdit {
	return StructureEdit{
		Months:        ir.Months,
		Weekdays:      ir.Weekdays,
		Moons:         ir.Moons,
		Seasons:       ir.Seasons,
		LeapYearEvery: ir.Settings.LeapYearEvery,
		Warnings:      ir.Warnings,
	}
}

// StructureEventChange names one event a structure save affects.
type StructureEventChange struct {
	EventID string
	Name    string
	// Date is the event's date as it reads before the save.
	Date string
	// Note says what happens to it, in words.
	Note string
}

// StructurePreview is what a structure save would do, shown before it is
// confirmed. Fingerprint identifies the calendar state the preview was
// computed against; the save refuses a fingerprint that no longer matches,
// so what is confirmed is always what was shown.
type StructurePreview struct {
	CalendarName string
	// MonthNotes describe each month that moves, is renamed, removed, added
	// or changes length.
	MonthNotes []string
	// OtherNotes cover the week, the leap rule, and removed moons/seasons.
	OtherNotes []string
	// MovedEvents counts events that follow their month to its new place.
	MovedEvents int
	// Redated lists events left on a month position that now holds a
	// different month, so they will show in that month instead.
	Redated      []StructureEventChange
	RedatedTotal int
	// Stranded lists events whose date (or end date) will no longer exist.
	// They are kept, untouched, but the calendar cannot show them there.
	Stranded      []StructureEventChange
	StrandedTotal int
	// CurrentDate and NewCurrentDate read as "<Month> <Day>, <Year>".
	CurrentDate        string
	NewCurrentDate     string
	CurrentDateClamped bool
	// Day-weather readings: moved with their month, now in a different
	// month (theirs is gone but its position remains), replaced by a moved
	// reading on the same day, or left on a day that no longer exists.
	WeatherMoved    int
	WeatherRedated  int
	WeatherReplaced int
	WeatherStranded int
	// Repeat rules that name months follow them (RulesUpdated); rules that
	// name a removed month, or a removed moon or season, are left as written
	// and named in OtherNotes. One-off changes to repeats follow their month
	// (OverridesMoved), stay in a removed month (OverridesRemoved), or give
	// way to a moved one on the same date (OverridesReplaced).
	RulesUpdated      int
	RulesRemovedMonth int
	RulesRemovedRef   int
	OverridesMoved    int
	OverridesRemoved  int
	OverridesReplaced int
	Warnings          []string
	Fingerprint       string
}

// StructureState is everything a structure save is planned from, as
// CalendarRepository reads it: the calendar with Months, Weekdays, Moons,
// Seasons and Eras loaded, every event's dates, every day-weather
// reading's date, every rule event's stored rule and every override.
type StructureState struct {
	Calendar    *Calendar
	Events      []Event
	WeatherDays []DayDate
	Rules       []StructureRule
	Overrides   []StructureOverride
}

// StructureWrite is one atomic structure save for CalendarRepository.
// MonthRemap maps an old month position (1-based) to its new one, holding
// only positions that change; every month column that stores a position
// (events, their end and recurrence-end dates, eras, day weather, and
// overrides) is remapped through it. RuleUpdates are the rewritten rules,
// which name months inside their JSON.
type StructureWrite struct {
	Months        []MonthInput
	Weekdays      []WeekdayInput
	Moons         []MoonInput
	Seasons       []Season
	LeapYearEvery int
	CurrentMonth  int
	CurrentDay    int
	MonthRemap    map[int]int
	RuleUpdates   []RuleUpdate
}

// structurePlan is planStructureEdit's result: the preview plus what the
// write needs.
type structurePlan struct {
	preview      StructurePreview
	remap        map[int]int
	currentMonth int
	currentDay   int
	ruleUpdates  []RuleUpdate
}

// reconcileMonths maps each old month (0-based) to its new 1-based
// position, or 0 when it has no counterpart. A name unique in both lists
// is matched by name wherever it moved. Otherwise a month keeps its
// position when the month now there is either the same name or a name the
// old list never had (a rename in place); anything else has no counterpart.
func reconcileMonths(old []Month, next []MonthInput) []int {
	oldCount := map[string]int{}
	for _, m := range old {
		oldCount[strings.TrimSpace(m.Name)]++
	}
	newCount := map[string]int{}
	newAt := map[string]int{}
	for j, m := range next {
		n := strings.TrimSpace(m.Name)
		newCount[n]++
		newAt[n] = j
	}

	mapping := make([]int, len(old))
	for i, m := range old {
		name := strings.TrimSpace(m.Name)
		if oldCount[name] == 1 && newCount[name] == 1 {
			mapping[i] = newAt[name] + 1
			continue
		}
		if i < len(next) {
			there := strings.TrimSpace(next[i].Name)
			if there == name || oldCount[there] == 0 {
				mapping[i] = i + 1
			}
		}
	}
	return mapping
}

// ordinalLabel is "1st", "2nd", "3rd", "11th" and so on.
func ordinalLabel(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func dateLabel(cal *Calendar, year, month, day int) string {
	return fmt.Sprintf("%s %d, %d", cal.MonthName(month), day, year)
}

// planStructureEdit works out what saving edit over cal would do to its
// months, its events and its current date. cal must have Months, Weekdays,
// Moons and Seasons loaded. Pure: no I/O, so the preview and the save
// compute exactly the same thing from the same state.
func planStructureEdit(cal *Calendar, events []Event, weather []DayDate, rules []StructureRule, overrides []StructureOverride, edit StructureEdit) structurePlan {
	next := &Calendar{
		Mode:           cal.Mode,
		LeapYearEvery:  edit.LeapYearEvery,
		LeapYearOffset: cal.LeapYearOffset,
		Months:         make([]Month, len(edit.Months)),
	}
	for i, m := range edit.Months {
		next.Months[i] = Month{Name: m.Name, Days: m.Days, SortOrder: i, IsIntercalary: m.IsIntercalary, LeapYearDays: m.LeapYearDays}
	}

	mapping := reconcileMonths(cal.Months, edit.Months)
	plan := structurePlan{remap: map[int]int{}}
	p := &plan.preview
	p.CalendarName = cal.Name
	p.Warnings = edit.Warnings

	matched := make([]bool, len(edit.Months))
	for i, m := range cal.Months {
		j := mapping[i]
		if j == 0 {
			p.MonthNotes = append(p.MonthNotes, fmt.Sprintf("%s is removed.", m.Name))
			continue
		}
		matched[j-1] = true
		nm := edit.Months[j-1]
		if j != i+1 {
			plan.remap[i+1] = j
			p.MonthNotes = append(p.MonthNotes, fmt.Sprintf("%s moves from %s to %s.", m.Name, ordinalLabel(i+1), ordinalLabel(j)))
		}
		if strings.TrimSpace(nm.Name) != strings.TrimSpace(m.Name) {
			p.MonthNotes = append(p.MonthNotes, fmt.Sprintf("%s is renamed %s.", m.Name, nm.Name))
		}
		if nm.Days != m.Days {
			p.MonthNotes = append(p.MonthNotes, fmt.Sprintf("%s goes from %d to %d days.", nm.Name, m.Days, nm.Days))
		}
		if nm.LeapYearDays != m.LeapYearDays {
			p.MonthNotes = append(p.MonthNotes, fmt.Sprintf("%s gains %d days in a leap year (was %d).", nm.Name, nm.LeapYearDays, m.LeapYearDays))
		}
	}
	for j, m := range edit.Months {
		if !matched[j] {
			p.MonthNotes = append(p.MonthNotes, fmt.Sprintf("%s is new, %s in the year.", m.Name, ordinalLabel(j+1)))
		}
	}

	p.OtherNotes = otherStructureNotes(cal, edit)

	// Where an old position ends up: its month's new place, or the same
	// number when the month has no counterpart (the row is not rewritten).
	place := func(month int) (int, bool) {
		if month >= 1 && month <= len(mapping) && mapping[month-1] > 0 {
			return mapping[month-1], true
		}
		return month, false
	}
	exists := func(c *Calendar, year, month, day int) bool {
		return month >= 1 && month <= len(c.Months) && day >= 1 && day <= c.MonthDays(month-1, year)
	}

	for _, e := range events {
		// An event already off the calendar before this save is not this
		// save's doing; it is left out rather than reported as new damage.
		if !exists(cal, e.Year, e.Month, e.Day) {
			continue
		}
		change := StructureEventChange{EventID: e.ID, Name: e.Name, Date: dateLabel(cal, e.Year, e.Month, e.Day)}
		nm, followed := place(e.Month)
		if followed && nm != e.Month {
			p.MovedEvents++
		}

		var stranded, redated string
		switch {
		case !followed && nm > len(next.Months):
			stranded = fmt.Sprintf("%s is removed; the event is kept but cannot be shown.", cal.MonthName(e.Month))
		case !exists(next, e.Year, nm, e.Day):
			stranded = fmt.Sprintf("%s will have no day %d in %d; the event is kept but cannot be shown.", next.MonthName(nm), e.Day, e.Year)
		case !followed:
			redated = fmt.Sprintf("%s is removed; the event will show on %s instead.", cal.MonthName(e.Month), dateLabel(next, e.Year, nm, e.Day))
		}
		// An end or repeat-until date has the same three fates as the start:
		// gone, or left on a position that now holds a different month.
		for _, end := range []struct {
			what      string
			y, m, d   *int
			movedVerb string
		}{
			{"end date", e.EndYear, e.EndMonth, e.EndDay, "end"},
			{"last repeat", e.RecurrenceEndYear, e.RecurrenceEndMonth, e.RecurrenceEndDay, "stop repeating"},
		} {
			if stranded != "" || end.y == nil || end.m == nil || end.d == nil || !exists(cal, *end.y, *end.m, *end.d) {
				continue
			}
			em, ef := place(*end.m)
			switch {
			case !exists(next, *end.y, em, *end.d):
				stranded = fmt.Sprintf("its %s, %s, will no longer exist.", end.what, dateLabel(cal, *end.y, *end.m, *end.d))
			case !ef:
				note := fmt.Sprintf("its %s is in %s, which is removed; it will %s on %s instead.",
					end.what, cal.MonthName(*end.m), end.movedVerb, dateLabel(next, *end.y, em, *end.d))
				if redated == "" {
					redated = strings.ToUpper(note[:1]) + note[1:]
				} else {
					redated += " Also, " + note
				}
			}
		}

		switch {
		case stranded != "":
			p.StrandedTotal++
			if len(p.Stranded) < maxStructurePreviewEvents {
				change.Note = stranded
				p.Stranded = append(p.Stranded, change)
			}
		case redated != "":
			p.RedatedTotal++
			if len(p.Redated) < maxStructurePreviewEvents {
				change.Note = redated
				p.Redated = append(p.Redated, change)
			}
		}
	}

	// The current date follows its month like an event does, then is
	// clamped only if it would otherwise point at a day that is gone.
	p.CurrentDate = dateLabel(cal, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
	cm, _ := place(cal.CurrentMonth)
	cd := cal.CurrentDay
	if cm > len(next.Months) {
		cm = len(next.Months)
		p.CurrentDateClamped = true
	}
	if cm < 1 {
		cm = 1
		p.CurrentDateClamped = true
	}
	if last := next.MonthDays(cm-1, cal.CurrentYear); cd > last {
		cd = last
		p.CurrentDateClamped = true
	}
	if cd < 1 {
		cd = 1
		p.CurrentDateClamped = true
	}
	plan.currentMonth, plan.currentDay = cm, cd
	p.NewCurrentDate = dateLabel(next, cal.CurrentYear, cm, cd)

	p.OtherNotes = append(p.OtherNotes, eraNotes(cal, next, place)...)
	planWeather(p, cal, next, weather, place, exists)
	p.OtherNotes = append(p.OtherNotes, weatherNotes(p)...)

	rp := planRepeats(cal, next, edit, rules, overrides, place)
	plan.ruleUpdates = rp.updates
	p.RulesUpdated, p.RulesRemovedMonth, p.RulesRemovedRef = rp.rulesUpdated, rp.rulesRemovedMonth, rp.rulesRemovedRef
	p.OverridesMoved, p.OverridesRemoved, p.OverridesReplaced = rp.overridesMoved, rp.overridesRemoved, rp.overridesReplaced
	p.OtherNotes = append(p.OtherNotes, repeatNotes(rp)...)

	p.Fingerprint = structureFingerprint(cal, events, weather, rules, overrides)
	return plan
}

// eraNotes says which eras follow their month, and which start or end in
// a month that is removed and so will read as a different month.
func eraNotes(cal, next *Calendar, place func(int) (int, bool)) []string {
	var notes []string
	moved := 0
	for _, er := range cal.Eras {
		follows := false
		describe := func(what string, m int) {
			if m < 1 || m > len(cal.Months) {
				return
			}
			nm, ok := place(m)
			switch {
			case ok && nm != m:
				follows = true
			case !ok && nm > len(next.Months):
				notes = append(notes, fmt.Sprintf("The era %s %s in %s, which is removed; that month will no longer exist.", er.Name, what, cal.MonthName(m)))
			case !ok:
				notes = append(notes, fmt.Sprintf("The era %s %s in %s, which is removed; it will read as %s instead.", er.Name, what, cal.MonthName(m), next.MonthName(nm)))
			}
		}
		describe("starts", er.StartMonth)
		if er.EndMonth != nil {
			describe("ends", *er.EndMonth)
		}
		if follows {
			moved++
		}
	}
	if moved > 0 {
		notes = append(notes, fmt.Sprintf("%d %s follow their month to its new place.", moved, nounFor(moved, "era", "eras")))
	}
	return notes
}

// planWeather sorts day-weather readings the way events are sorted. A
// reading left in a month with no counterpart stays put, unless a moved
// reading now claims its day: the moved one wins (ApplyStructure deletes
// the other), since its month still exists under its own name.
func planWeather(p *StructurePreview, cal, next *Calendar, weather []DayDate, place func(int) (int, bool), exists func(*Calendar, int, int, int) bool) {
	claimed := map[DayDate]bool{}
	for _, w := range weather {
		if nm, ok := place(w.Month); ok && nm != w.Month && exists(cal, w.Year, w.Month, w.Day) {
			claimed[DayDate{Year: w.Year, Month: nm, Day: w.Day}] = true
		}
	}
	for _, w := range weather {
		if !exists(cal, w.Year, w.Month, w.Day) {
			continue
		}
		nm, ok := place(w.Month)
		switch {
		case !ok && claimed[w]:
			p.WeatherReplaced++
		case !exists(next, w.Year, nm, w.Day):
			p.WeatherStranded++
		case !ok:
			p.WeatherRedated++
		case nm != w.Month:
			p.WeatherMoved++
		}
	}
}

func weatherNotes(p *StructurePreview) []string {
	var notes []string
	reading := func(n int) string { return fmt.Sprintf("%d day-weather %s", n, nounFor(n, "reading", "readings")) }
	if p.WeatherMoved > 0 {
		notes = append(notes, reading(p.WeatherMoved)+" follow their month to its new place.")
	}
	if p.WeatherRedated > 0 {
		notes = append(notes, reading(p.WeatherRedated)+" in removed months will show in whichever month now holds their place.")
	}
	if p.WeatherReplaced > 0 {
		notes = append(notes, reading(p.WeatherReplaced)+" in removed months will be deleted: a moved month's reading takes their day.")
	}
	if p.WeatherStranded > 0 {
		notes = append(notes, reading(p.WeatherStranded)+" will be on a day that no longer exists; they are kept but not shown.")
	}
	return notes
}

// otherStructureNotes describes the week, leap-rule, moon and season
// changes in words.
func otherStructureNotes(cal *Calendar, edit StructureEdit) []string {
	var notes []string
	if len(edit.Weekdays) != len(cal.Weekdays) {
		notes = append(notes, fmt.Sprintf("The week goes from %d to %d days, so dates will fall on different weekdays.", len(cal.Weekdays), len(edit.Weekdays)))
	} else {
		for i, w := range edit.Weekdays {
			if strings.TrimSpace(w.Name) != strings.TrimSpace(cal.Weekdays[i].Name) {
				notes = append(notes, "Weekday names or their order change.")
				break
			}
		}
	}
	if edit.LeapYearEvery != cal.LeapYearEvery {
		switch {
		case edit.LeapYearEvery <= 0:
			notes = append(notes, "Leap years are turned off.")
		case cal.LeapYearEvery <= 0:
			notes = append(notes, fmt.Sprintf("Leap years are turned on: every %d years.", edit.LeapYearEvery))
		default:
			notes = append(notes, fmt.Sprintf("Leap years change from every %d to every %d years.", cal.LeapYearEvery, edit.LeapYearEvery))
		}
	}
	keptMoons := map[int]bool{}
	for _, m := range edit.Moons {
		if m.ID != nil {
			keptMoons[*m.ID] = true
		}
	}
	for _, m := range cal.Moons {
		if !keptMoons[m.ID] {
			notes = append(notes, fmt.Sprintf("The moon %s is removed.", m.Name))
		}
	}
	keptSeasons := map[int]bool{}
	for _, s := range edit.Seasons {
		if s.ID != 0 {
			keptSeasons[s.ID] = true
		}
	}
	for _, s := range cal.Seasons {
		if !keptSeasons[s.ID] {
			notes = append(notes, fmt.Sprintf("The season %s is removed.", s.Name))
		}
	}
	return notes
}

// structureFingerprint hashes everything planStructureEdit reads, so a
// save can tell that the calendar, its events or its weather changed after
// the preview was shown. Lists are hashed in a fixed order of their own,
// whatever order they were read in.
func structureFingerprint(cal *Calendar, events []Event, weather []DayDate, rules []StructureRule, overrides []StructureOverride) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "leap %d %d|now %d %d %d|", cal.LeapYearEvery, cal.LeapYearOffset, cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay)
	for _, m := range cal.Months {
		_, _ = fmt.Fprintf(h, "m %q %d %d %t|", m.Name, m.Days, m.LeapYearDays, m.IsIntercalary)
	}
	for _, w := range cal.Weekdays {
		_, _ = fmt.Fprintf(h, "w %q|", w.Name)
	}
	ptr := func(v *int) string {
		if v == nil {
			return "-"
		}
		return fmt.Sprint(*v)
	}
	var lines []string
	for _, m := range cal.Moons {
		lines = append(lines, fmt.Sprintf("o %d %q", m.ID, m.Name))
	}
	for _, s := range cal.Seasons {
		lines = append(lines, fmt.Sprintf("s %d %q", s.ID, s.Name))
	}
	for _, er := range cal.Eras {
		lines = append(lines, fmt.Sprintf("r %d %q %d %s", er.ID, er.Name, er.StartMonth, ptr(er.EndMonth)))
	}
	for _, e := range events {
		lines = append(lines, fmt.Sprintf("e %s %q %d %d %d %s %s %s %s %s %s", e.ID, e.Name, e.Year, e.Month, e.Day,
			ptr(e.EndYear), ptr(e.EndMonth), ptr(e.EndDay),
			ptr(e.RecurrenceEndYear), ptr(e.RecurrenceEndMonth), ptr(e.RecurrenceEndDay)))
	}
	for _, w := range weather {
		lines = append(lines, fmt.Sprintf("d %d %d %d", w.Year, w.Month, w.Day))
	}
	for _, r := range rules {
		lines = append(lines, fmt.Sprintf("x %s %q", r.EventID, r.Raw))
	}
	for _, o := range overrides {
		lines = append(lines, fmt.Sprintf("v %s %d %d %d %s", o.EventID, o.Year, o.Month, o.Day, ptr(o.NewMonth)))
	}
	sort.Strings(lines)
	for _, l := range lines {
		h.Write([]byte(l + "|"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// validateStructureEdit holds a structure save to the same limits the bulk
// writers enforce, plus the two a live calendar cannot do without.
func validateStructureEdit(edit StructureEdit) error {
	if len(edit.Months) == 0 {
		return apperror.NewBadRequest("a calendar needs at least one month")
	}
	if len(edit.Weekdays) == 0 {
		return apperror.NewBadRequest("a calendar needs at least one weekday")
	}
	if err := validateMonthInputs(edit.Months); err != nil {
		return err
	}
	if len(edit.Weekdays) > maxCalendarWeekdays {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d weekdays", maxCalendarWeekdays))
	}
	for _, w := range edit.Weekdays {
		if err := validateStructureName("weekday", w.Name, maxCalendarShortNameLength); err != nil {
			return err
		}
	}
	if len(edit.Moons) > maxCalendarMoons {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d moons", maxCalendarMoons))
	}
	for _, m := range edit.Moons {
		if err := validateStructureName("moon", m.Name, maxCalendarShortNameLength); err != nil {
			return err
		}
		if m.CycleDays <= 0 || m.CycleDays > maxCalendarMonthDays {
			return apperror.NewBadRequest(fmt.Sprintf("moon %q needs a cycle between 1 and %d days", m.Name, maxCalendarMonthDays))
		}
	}
	if len(edit.Seasons) > maxCalendarSeasons {
		return apperror.NewBadRequest(fmt.Sprintf("a calendar can have at most %d seasons", maxCalendarSeasons))
	}
	for _, s := range edit.Seasons {
		if err := validateStructureName("season", s.Name, maxCalendarShortNameLength); err != nil {
			return err
		}
		if s.StartMonth < 1 || s.StartMonth > len(edit.Months) || s.EndMonth < 1 || s.EndMonth > len(edit.Months) {
			return apperror.NewBadRequest(fmt.Sprintf("season %q must start and end in one of the calendar's months", s.Name))
		}
	}
	if edit.LeapYearEvery < 0 || edit.LeapYearEvery > 1000 {
		return apperror.NewBadRequest("leap years can be every 1 to 1000 years, or off")
	}
	return nil
}

// structureState loads the plan's input for a calendar in campaignID,
// refusing one whose months follow the real-world calendar.
func (s *calendarService) structureState(ctx context.Context, calendarID, campaignID string) (*StructureState, error) {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	st, err := s.calRepo.GetStructureState(ctx, calendarID)
	if err != nil {
		return nil, err
	}
	if err := refuseRealTime(st.Calendar); err != nil {
		return nil, err
	}
	return st, nil
}

func refuseRealTime(cal *Calendar) error {
	if cal.UsesRealTime() {
		return apperror.NewValidation("this calendar follows the real-world date, so its months and weekdays can't be changed")
	}
	return nil
}

// PreviewStructureEdit computes what saving edit would do, without writing.
func (s *calendarService) PreviewStructureEdit(ctx context.Context, calendarID, campaignID string, edit StructureEdit) (*StructurePreview, error) {
	if err := validateStructureEdit(edit); err != nil {
		return nil, err
	}
	st, err := s.structureState(ctx, calendarID, campaignID)
	if err != nil {
		return nil, err
	}
	plan := planStructureEdit(st.Calendar, st.Events, st.WeatherDays, st.Rules, st.Overrides, edit)
	return &plan.preview, nil
}

// ApplyStructureEdit saves edit in one transaction. The plan is made again
// inside that transaction, from state read under a lock on the calendar
// row, rather than trusted from the preview: if its fingerprint differs
// from the one the owner saw, nothing is written and the error is a
// Conflict returned with the fresh preview, for the caller to show.
func (s *calendarService) ApplyStructureEdit(ctx context.Context, calendarID, campaignID, fingerprint string, edit StructureEdit) (*StructurePreview, error) {
	if err := validateStructureEdit(edit); err != nil {
		return nil, err
	}
	// Which campaign a calendar belongs to never changes, so this check
	// needs no lock.
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return nil, err
	}
	var preview *StructurePreview
	err := s.calRepo.ApplyStructure(ctx, calendarID, func(st *StructureState) (*StructureWrite, error) {
		if err := refuseRealTime(st.Calendar); err != nil {
			return nil, err
		}
		plan := planStructureEdit(st.Calendar, st.Events, st.WeatherDays, st.Rules, st.Overrides, edit)
		preview = &plan.preview
		if fingerprint != plan.preview.Fingerprint {
			return nil, apperror.NewConflict("this calendar changed after the preview was made; check the updated preview before saving")
		}
		return structureWriteFor(edit, plan), nil
	})
	if err != nil {
		if apperror.SafeCode(err) == http.StatusConflict {
			return preview, err
		}
		if _, ok := err.(*apperror.AppError); ok {
			return nil, err
		}
		return nil, fmt.Errorf("apply structure: %w", err)
	}
	return preview, nil
}

// structureWriteFor turns a validated edit and its plan into the write.
func structureWriteFor(edit StructureEdit, plan structurePlan) *StructureWrite {
	moons := make([]MoonInput, len(edit.Moons))
	copy(moons, edit.Moons)
	for i := range moons {
		moons[i].Color = normalizeColor(moons[i].Color)
	}
	seasons := make([]Season, len(edit.Seasons))
	copy(seasons, edit.Seasons)
	for i := range seasons {
		seasons[i].Color = normalizeColor(seasons[i].Color)
	}
	months := make([]MonthInput, len(edit.Months))
	for i, m := range edit.Months {
		m.SortOrder = i
		months[i] = m
	}
	weekdays := make([]WeekdayInput, len(edit.Weekdays))
	for i, w := range edit.Weekdays {
		w.SortOrder = i
		weekdays[i] = w
	}
	return &StructureWrite{
		Months:        months,
		Weekdays:      weekdays,
		Moons:         moons,
		Seasons:       seasons,
		LeapYearEvery: edit.LeapYearEvery,
		CurrentMonth:  plan.currentMonth,
		CurrentDay:    plan.currentDay,
		MonthRemap:    plan.remap,
		RuleUpdates:   plan.ruleUpdates,
	}
}
