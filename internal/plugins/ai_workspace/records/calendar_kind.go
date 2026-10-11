package records

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// CalendarStructureAPI is the slice of calendar.CalendarService the
// calendar kind uses: making the campaign's calendar, and changing its
// structure, name, year label, current date and eras.
type CalendarStructureAPI interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
	ListCalendars(ctx context.Context, campaignID string, v permissions.Viewer) ([]calendar.Calendar, error)
	CreateCalendarFromImport(ctx context.Context, campaignID string, ir *calendar.ImportResult, opts calendar.CreateCalendarFromImportOptions) (*calendar.Calendar, error)
	SetDefaultCalendar(ctx context.Context, campaignID, calendarID string) error
	UpdateCalendar(ctx context.Context, calendarID, campaignID string, input calendar.UpdateCalendarInput) error
	SetCurrentDate(ctx context.Context, calendarID, campaignID string, year, month, day, hour, minute int) error
	PreviewStructureEdit(ctx context.Context, calendarID, campaignID string, edit calendar.StructureEdit) (*calendar.StructurePreview, error)
	ApplyStructureEdit(ctx context.Context, calendarID, campaignID, fingerprint string, edit calendar.StructureEdit) (*calendar.StructurePreview, error)
	CreateEra(ctx context.Context, calendarID, campaignID string, input calendar.EraInput) (*calendar.Era, error)
	UpdateEra(ctx context.Context, eraID int, calendarID, campaignID string, input calendar.UpdateEraInput) error
}

// ---- finding the calendar the other calendar kinds write to ----

type defaultCalendarReader interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
}

type calendarLister interface {
	ListCalendars(ctx context.Context, campaignID string, v permissions.Viewer) ([]calendar.Calendar, error)
}

// needsCalendarPrefix starts every refusal caused by a missing calendar, so
// the review screen and the result both read as "do this first", not as a
// fault in the block.
const needsCalendarPrefix = "Needs a calendar first: "

type pendingCalendarKey struct{}

// withPendingCalendar lets the rows after a `kind: calendar` create be
// checked against the calendar that row will make. Review only: at commit
// the calendar row has already run, so every row reads the real one.
func withPendingCalendar(ctx context.Context, cal *calendar.Calendar) context.Context {
	return context.WithValue(ctx, pendingCalendarKey{}, cal)
}

func pendingCalendar(ctx context.Context) *calendar.Calendar {
	cal, _ := ctx.Value(pendingCalendarKey{}).(*calendar.Calendar)
	return cal
}

// defaultCalendar is the calendar events, weather and generators write to.
// A pending calendar has no ID; callers skip reads keyed on it.
func defaultCalendar(ctx context.Context, svc defaultCalendarReader, campaignID string, a Actor) (*calendar.Calendar, error) {
	cal, err := svc.GetDefaultCalendarForViewer(ctx, campaignID, a.Viewer())
	if err == nil && cal != nil {
		return cal, nil
	}
	if err != nil && apperror.SafeCode(err) != http.StatusNotFound {
		return nil, apperror.NewBadRequest("could not read the campaign's calendar; try again in a moment")
	}
	if p := pendingCalendar(ctx); p != nil {
		return p, nil
	}
	if l, ok := svc.(calendarLister); ok {
		if cals, lerr := l.ListCalendars(ctx, campaignID, a.Viewer()); lerr == nil && len(cals) > 0 {
			return nil, apperror.NewBadRequest(needsCalendarPrefix + "none of this campaign's calendars is set as the main one. Choose one on the Calendars page.")
		}
	}
	return nil, apperror.NewBadRequest(needsCalendarPrefix + "this campaign has no calendar yet. Put a `kind: calendar` block above this one, or make a calendar on the Calendars page.")
}

// ---- the calendar kind ----

// KindCalendar is the calendar block's `kind:` value.
const (
	KindCalendar = "calendar"
)

// CalendarKind makes the campaign's calendar, or changes the main one.
type CalendarKind struct{ Svc CalendarStructureAPI }

func (CalendarKind) Name() string  { return KindCalendar }
func (CalendarKind) Label() string { return "Calendar" }
func (CalendarKind) Doc() string {
	return "The campaign calendar. `action: create` makes it when the campaign has none; `action: update` changes the main calendar (removing a calendar is done on the Calendars page). Keys, all optional on update:\n" +
		"- `name`; on update, `rename_to` renames it.\n" +
		"- `year_label`: what follows the year, e.g. DR.\n" +
		"- `current_year`, `current_month` (number or name), `current_day`: today's date. Needed on create.\n" +
		"- `months`: a list of `name`, `days`, optional `season`, `intercalary` (true for festival days outside the weeks), `leap_days`. Needed on create.\n" +
		"- `weekdays`: a list of names, or of `name` with `rest_day: true`. Needed on create.\n" +
		"- `seasons`: a list of `name`, `start_month`, `start_day`, `end_month`, `end_day`, `color`. Without it, seasons are made from the months' `season`.\n" +
		"- `moons`: a list of `name`, `cycle` (days from new moon to new moon), `phase_offset` (days), `color`.\n" +
		"- `eras`: a list of `name`, `start_year` (optional `start_month`, `start_day`), optional `end_year`/`end_month`/`end_day`, `color`, `description`.\n" +
		"- `leap_year_every`: years between leap years (0 for none).\n" +
		"The body is the calendar's description. On update, a `months`, `weekdays`, `seasons` or `moons` list replaces the whole list (events follow a month that keeps its name), and a key left out keeps what is there. Moons and eras are matched by name, and a key left out keeps what is there; a moon hidden from players is never removed here. Eras not listed are kept. An end with only `end_year` runs to the end of that year.\n\n" +
		"```\n---\nkind: calendar\nname: Calendar of Harptos\nyear_label: DR\ncurrent_year: 1492\ncurrent_month: Hammer\ncurrent_day: 1\nmonths:\n  - {name: Hammer, days: 30, season: Winter}\n  - {name: Midwinter, days: 1, intercalary: true, season: Winter}\n  - {name: Alturiak, days: 30, season: Winter}\nweekdays: [First-day, Second-day, Third-day]\nmoons:\n  - {name: Selune, cycle: 30.4375, phase_offset: 0, color: \"#e8e8f0\"}\neras:\n  - {name: Dalereckoning, start_year: 1}\n---\n```"
}

// calSpec is a calendar block read and checked against the months it will
// have. Lists are nil when the block left them out.
type calSpec struct {
	months    []calendar.MonthInput
	weekdays  []calendar.WeekdayInput
	seasons   []calendar.Season
	moons     []calendar.MoonInput
	eras      []eraSpec
	leapEvery *int
	yearLabel *string
	next      *calendar.Calendar // the calendar as it will be, for dates
	today     *calDate
	warnings  []string
}

type eraSpec struct {
	in       calendar.EraInput
	keys     Record // the era's own keys, for a partial update
	hasStart bool
}

// listItems reads a list key whose entries are maps (or, where allowed,
// bare names) as Records with lower-cased keys.
func listItems(r Record, key string, names bool) ([]Record, error) {
	raw := r.List(key)
	out := make([]Record, 0, len(raw))
	for i, v := range raw {
		switch t := v.(type) {
		case map[string]any:
			f := make(map[string]any, len(t))
			for k, val := range t {
				f[strings.ToLower(strings.TrimSpace(k))] = val
			}
			out = append(out, Record{Name: strings.TrimSpace(fmt.Sprint(f["name"])), Fields: f})
		case string:
			if !names {
				return nil, badRequestf("%s: entry %d needs keys such as name", key, i+1)
			}
			out = append(out, Record{Name: strings.TrimSpace(t), Fields: map[string]any{"name": t}})
		default:
			return nil, badRequestf("%s: entry %d is not readable", key, i+1)
		}
		if f := out[len(out)-1].Fields["name"]; f == nil || strings.TrimSpace(fmt.Sprint(f)) == "" {
			return nil, badRequestf("%s: entry %d needs a name", key, i+1)
		}
	}
	return out, nil
}

// firstNum reads the first of several key spellings that is present.
func firstNum(r Record, keys ...string) (float64, bool, error) {
	for _, k := range keys {
		if f, ok, err := r.Float(k); ok || err != nil {
			return f, ok, err
		}
	}
	return 0, false, nil
}

// monthIndex resolves a month written as a number or a name against months.
func monthIndex(months []calendar.MonthInput, r Record, key string) (int, bool, error) {
	s := r.Str(key)
	if s == "" {
		return 0, false, nil
	}
	if n, _, err := r.Int(key); err == nil {
		if n < 1 || n > len(months) {
			return 0, true, badRequestf("%s: there is no month %d", key, n)
		}
		return n, true, nil
	}
	for i, m := range months {
		if sameName(m.Name, s) {
			return i + 1, true, nil
		}
	}
	return 0, true, badRequestf("%s: %q is not one of the months", key, s)
}

func monthInputs(ms []calendar.Month) []calendar.MonthInput {
	out := make([]calendar.MonthInput, len(ms))
	for i, m := range ms {
		out[i] = calendar.MonthInput{Name: m.Name, Days: m.Days, SortOrder: i, IsIntercalary: m.IsIntercalary, LeapYearDays: m.LeapYearDays}
	}
	return out
}

// readCalSpec reads every key of a calendar block. base is the calendar
// being changed, nil on create.
func readCalSpec(r Record, base *calendar.Calendar) (*calSpec, error) {
	s := &calSpec{}
	if r.Has("year_label") {
		v := r.Str("year_label")
		s.yearLabel = &v
	}
	if n, ok, err := r.Int("leap_year_every"); err != nil {
		return nil, err
	} else if ok {
		if n < 0 || n > 1000 {
			return nil, apperror.NewBadRequest("leap_year_every must be 0 to 1000")
		}
		s.leapEvery = &n
	}

	var monthSeasons []string
	if r.Has("months") {
		items, err := listItems(r, "months", false)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, apperror.NewBadRequest("months: a calendar needs at least one month")
		}
		s.months = make([]calendar.MonthInput, 0, len(items))
		for _, it := range items {
			days, ok, err := it.Int("days")
			if err != nil {
				return nil, err
			}
			if !ok || days < 1 || days > calendar.MaxMonthDays {
				return nil, badRequestf("month %q needs days, 1 to %d", it.Name, calendar.MaxMonthDays)
			}
			leap, _, err := it.Int("leap_days")
			if err != nil {
				return nil, err
			}
			inter, _ := it.Bool("intercalary")
			s.months = append(s.months, calendar.MonthInput{Name: it.Name, Days: days, SortOrder: len(s.months), IsIntercalary: inter, LeapYearDays: leap})
			monthSeasons = append(monthSeasons, it.Str("season"))
		}
	}
	months := s.months
	if months == nil && base != nil {
		months = monthInputs(base.Months)
	}

	if r.Has("weekdays") {
		items, err := listItems(r, "weekdays", true)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, apperror.NewBadRequest("weekdays: a calendar needs at least one weekday")
		}
		s.weekdays = make([]calendar.WeekdayInput, len(items))
		for i, it := range items {
			rest, _ := it.Bool("rest_day")
			s.weekdays[i] = calendar.WeekdayInput{Name: it.Name, SortOrder: i, IsRestDay: rest}
		}
	}

	switch {
	case r.Has("seasons"):
		if r.Has("months") && hasAny(monthSeasons) {
			s.warnings = append(s.warnings, "The months' season keys are ignored because a seasons list is given")
		}
		items, err := listItems(r, "seasons", false)
		if err != nil {
			return nil, err
		}
		s.seasons = make([]calendar.Season, 0, len(items))
		for _, it := range items {
			sm, ok1, err := monthIndex(months, it, "start_month")
			if err != nil {
				return nil, err
			}
			em, ok2, err := monthIndex(months, it, "end_month")
			if err != nil {
				return nil, err
			}
			if !ok1 || !ok2 {
				return nil, badRequestf("season %q needs start_month and end_month", it.Name)
			}
			sd, ok, err := it.Int("start_day")
			if err != nil {
				return nil, err
			}
			if !ok {
				sd = 1
			}
			ed, ok, err := it.Int("end_day")
			if err != nil {
				return nil, err
			}
			if !ok {
				ed = months[em-1].Days
			}
			s.seasons = append(s.seasons, calendar.Season{Name: it.Name, StartMonth: sm, StartDay: sd, EndMonth: em, EndDay: ed, Color: it.Str("color")})
		}
	case hasAny(monthSeasons):
		s.seasons = seasonsFromMonths(s.months, monthSeasons)
	}

	if r.Has("moons") {
		items, err := listItems(r, "moons", false)
		if err != nil {
			return nil, err
		}
		s.moons = make([]calendar.MoonInput, len(items))
		for i, it := range items {
			cycle, ok, err := firstNum(it, "cycle", "cycle_days", "cycle_length")
			if err != nil {
				return nil, err
			}
			old := baseMoon(base, it.Name)
			if !ok && old != nil {
				cycle = old.CycleDays
			} else if !ok || cycle <= 0 {
				return nil, badRequestf("moon %q needs a cycle in days", it.Name)
			}
			offset, ok, err := firstNum(it, "phase_offset", "offset")
			if err != nil {
				return nil, err
			}
			color := it.Str("color")
			// A key left out keeps what the moon already has.
			if old != nil {
				if !ok {
					offset = old.PhaseOffset
				}
				if color == "" {
					color = old.Color
				}
			}
			s.moons[i] = calendar.MoonInput{Name: it.Name, CycleDays: cycle, PhaseOffset: offset, Color: color}
		}
	}

	next := &calendar.Calendar{Mode: calendar.ModeFantasy, Name: r.Name}
	if base != nil {
		cp := *base
		next = &cp
	}
	if s.months != nil {
		next.Months = make([]calendar.Month, len(s.months))
		for i, m := range s.months {
			next.Months[i] = calendar.Month{Name: m.Name, Days: m.Days, SortOrder: i, IsIntercalary: m.IsIntercalary, LeapYearDays: m.LeapYearDays}
		}
	}
	if s.leapEvery != nil {
		next.LeapYearEvery = *s.leapEvery
	}
	s.next = next

	if r.Has("eras") {
		items, err := listItems(r, "eras", false)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			e, err := readEra(next, it)
			if err != nil {
				return nil, err
			}
			s.eras = append(s.eras, e)
		}
	}

	y, m, d, dated, err := readDate(next, r, "current_")
	if err != nil {
		return nil, err
	}
	if dated {
		s.today = &calDate{y, m, d}
	}
	return s, nil
}

func baseMoon(base *calendar.Calendar, name string) *calendar.Moon {
	if base == nil {
		return nil
	}
	for i := range base.Moons {
		if sameName(base.Moons[i].Name, name) {
			return &base.Moons[i]
		}
	}
	return nil
}

func hasAny(ss []string) bool {
	for _, s := range ss {
		if s != "" {
			return true
		}
	}
	return false
}

// seasonsFromMonths turns each run of months sharing a season name into
// one season covering those months whole. A season that ends the year and
// also starts it is one season that wraps round.
func seasonsFromMonths(months []calendar.MonthInput, names []string) []calendar.Season {
	var out []calendar.Season
	for i := 0; i < len(months); i++ {
		if names[i] == "" {
			continue
		}
		j := i
		for j+1 < len(months) && sameName(names[j+1], names[i]) {
			j++
		}
		out = append(out, calendar.Season{Name: names[i], StartMonth: i + 1, StartDay: 1, EndMonth: j + 1, EndDay: months[j].Days})
		i = j
	}
	if n := len(out); n > 1 && out[0].StartMonth == 1 && out[n-1].EndMonth == len(months) && sameName(out[0].Name, out[n-1].Name) {
		out[0].StartMonth, out[0].StartDay = out[n-1].StartMonth, 1
		out = out[:n-1]
	}
	return out
}

// readEra reads one era entry against the months the calendar will have.
// A start or end with only a year begins on its first day.
func readEra(cal *calendar.Calendar, it Record) (eraSpec, error) {
	e := eraSpec{keys: it, in: calendar.EraInput{Name: it.Name, Color: it.Str("color")}}
	read := func(prefix string) (y, m, d int, ok bool, err error) {
		y, ok, err = it.Int(prefix + "year")
		if err != nil || !ok {
			if !ok && (it.Has(prefix+"month") || it.Has(prefix+"day")) {
				err = badRequestf("era %q: %smonth needs %syear", it.Name, prefix, prefix)
			}
			return
		}
		m, d = 1, 1
		if it.Has(prefix + "month") {
			sub := Record{Fields: map[string]any{"year": y, "month": it.Fields[prefix+"month"], "day": it.Fields[prefix+"day"]}}
			if !it.Has(prefix + "day") {
				sub.Fields["day"] = 1
			}
			y, m, d, _, err = readDate(cal, sub, "")
			if err != nil {
				err = badRequestf("era %q: %s", it.Name, planError(err))
			}
		}
		return
	}
	sy, sm, sd, ok, err := read("start_")
	if err != nil {
		return e, err
	}
	e.hasStart = ok
	e.in.StartYear, e.in.StartMonth, e.in.StartDay = sy, sm, sd
	ey, em, ed, ended, err := read("end_")
	if err != nil {
		return e, err
	}
	if ended {
		e.in.EndYear = &ey
		// An end with only a year runs to that year's last day, as the
		// calendar reads an era with no end month.
		if it.Has("end_month") {
			e.in.EndMonth, e.in.EndDay = &em, &ed
		} else {
			em, ed = len(cal.Months)+1, 0
		}
		if ok && (calDate{ey, em, ed}).Before(calDate{sy, sm, sd}) {
			return e, badRequestf("era %q ends before it starts", it.Name)
		}
	}
	if s := it.Str("description"); s != "" {
		e.in.Description = &s
	}
	return e, nil
}

// importResult is the create path's input for the calendar service.
func (s *calSpec) importResult(r Record) *calendar.ImportResult {
	ir := &calendar.ImportResult{
		CalendarName: r.Name,
		Months:       s.months,
		Weekdays:     s.weekdays,
		Moons:        s.moons,
		Seasons:      s.seasons,
	}
	for _, e := range s.eras {
		ir.Eras = append(ir.Eras, e.in)
	}
	ir.Settings.EpochName = s.yearLabel
	if b := strings.TrimSpace(r.Body); b != "" {
		ir.Settings.Description = &b
	}
	if s.leapEvery != nil {
		ir.Settings.LeapYearEvery = *s.leapEvery
	}
	if s.today != nil {
		m, d := s.today.M, s.today.D
		ir.Today = calendar.ImportedToday{Year: s.today.Y, Month: &m, Day: &d}
	}
	return ir
}

func (CalendarKind) ownerOnly(a Actor) string {
	if a.Role < permissions.RoleOwner {
		return "only the campaign owner can change the calendar"
	}
	return ""
}

// existing is the main calendar, or nil when there is none.
func (k CalendarKind) existing(ctx context.Context, campaignID string, a Actor) (*calendar.Calendar, error) {
	cal, err := k.Svc.GetDefaultCalendarForViewer(ctx, campaignID, a.Viewer())
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return nil, nil
		}
		return nil, apperror.NewBadRequest("could not read the campaign's calendar; try again in a moment")
	}
	return cal, nil
}

func (k CalendarKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, _ := k.plan(ctx, campaignID, a, r)
	return p
}

// plan checks the block and, for a create, returns the calendar it would
// make so the rows after it can be checked against that calendar.
func (k CalendarKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (Plan, *calendar.Calendar) {
	if msg := k.ownerOnly(a); msg != "" {
		return Plan{Error: msg}, nil
	}
	cal, err := k.existing(ctx, campaignID, a)
	if err != nil {
		return Plan{Error: planError(err)}, nil
	}
	switch r.Action {
	case ActionDelete:
		return Plan{Error: "AI Import doesn't remove calendars; remove one on the Calendars page"}, nil
	case ActionCreate:
		return k.planCreate(ctx, campaignID, a, r, cal)
	}
	if cal == nil {
		return Plan{Error: "this campaign has no calendar to change; use action: create to make one"}, nil
	}
	return k.planUpdate(ctx, campaignID, r, cal), nil
}

func (k CalendarKind) planCreate(ctx context.Context, campaignID string, a Actor, r Record, cal *calendar.Calendar) (Plan, *calendar.Calendar) {
	if cal != nil {
		return Plan{Error: fmt.Sprintf("this campaign already has the calendar %s; use action: update to change it", quote(cal.Name))}, nil
	}
	if r.Name == "" {
		return Plan{Error: "a new calendar needs a name"}, nil
	}
	s, err := readCalSpec(r, nil)
	if err != nil {
		return Plan{Error: planError(err)}, nil
	}
	switch {
	case s.months == nil:
		return Plan{Error: "a new calendar needs months"}, nil
	case s.weekdays == nil:
		return Plan{Error: "a new calendar needs weekdays"}, nil
	case s.today == nil:
		return Plan{Error: "a new calendar needs current_year, current_month and current_day"}, nil
	}
	ir := s.importResult(r)
	if err := calendar.PrepareImport(ir); err != nil {
		return Plan{Error: planError(err)}, nil
	}
	p := Plan{Summary: createSummary(s, ir), Warnings: append(s.warnings, ir.Warnings...)}
	cals, err := k.Svc.ListCalendars(ctx, campaignID, a.Viewer())
	for _, c := range cals {
		if sameName(c.Name, r.Name) {
			return Plan{Error: fmt.Sprintf("a calendar called %s already exists but is not the main one; make it the main one on the Calendars page", quote(r.Name))}, nil
		}
	}
	if err == nil && len(cals) > 0 {
		p.Warnings = append(p.Warnings, "The campaign has other calendars, but none is the main one; this one becomes the main calendar")
	}
	next := s.next
	next.Weekdays = make([]calendar.Weekday, len(ir.Weekdays))
	for i, w := range ir.Weekdays {
		next.Weekdays[i] = calendar.Weekday{Name: w.Name, SortOrder: i, IsRestDay: w.IsRestDay}
	}
	next.CurrentYear, next.CurrentMonth, next.CurrentDay = s.today.Y, s.today.M, s.today.D
	next.Seasons = ir.Seasons
	return p, next
}

func createSummary(s *calSpec, ir *calendar.ImportResult) string {
	parts := []string{
		plural(len(ir.Months), "month", "months"),
		fmt.Sprintf("a %d-day week", len(ir.Weekdays)),
	}
	if n := len(ir.Seasons); n > 0 {
		parts = append(parts, plural(n, "season", "seasons"))
	}
	if n := len(ir.Moons); n > 0 {
		parts = append(parts, plural(n, "moon", "moons"))
	}
	if n := len(ir.Eras); n > 0 {
		parts = append(parts, plural(n, "era", "eras"))
	}
	today := dateLabel(s.next, s.today.Y, s.today.M, s.today.D)
	if s.yearLabel != nil && *s.yearLabel != "" {
		today += " " + *s.yearLabel
	}
	return "makes the calendar with " + strings.Join(parts, ", ") + "; today is " + today
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// structureEdit is the whole-structure save an update needs, or nil when
// the block changes no months, weekdays, seasons, moons or leap rule.
// Moons and seasons keep their ids where a name matches, so a moon's
// hidden flag and look survive.
func structureEdit(s *calSpec, cal *calendar.Calendar) (*calendar.StructureEdit, error) {
	if s.months == nil && s.weekdays == nil && s.seasons == nil && s.moons == nil && s.leapEvery == nil {
		return nil, nil
	}
	edit := &calendar.StructureEdit{Months: s.months, Weekdays: s.weekdays, LeapYearEvery: cal.LeapYearEvery}
	if edit.Months == nil {
		edit.Months = monthInputs(cal.Months)
	}
	if edit.Weekdays == nil {
		for i, w := range cal.Weekdays {
			edit.Weekdays = append(edit.Weekdays, calendar.WeekdayInput{Name: w.Name, SortOrder: i, IsRestDay: w.IsRestDay})
		}
	}
	if s.leapEvery != nil {
		edit.LeapYearEvery = *s.leapEvery
	}
	if s.moons != nil {
		for _, m := range s.moons {
			for _, old := range cal.Moons {
				if sameName(old.Name, m.Name) {
					id := old.ID
					m.ID = &id
					break
				}
			}
			edit.Moons = append(edit.Moons, m)
		}
		// A moon hidden from players is never in a player-safe export, so a
		// list written from one would remove it; hidden moons stay unless
		// removed on the calendar's own settings.
		for _, old := range cal.Moons {
			if old.HiddenFromPlayers && baseMoonIn(edit.Moons, old.Name) == nil {
				id := old.ID
				edit.Moons = append(edit.Moons, calendar.MoonInput{ID: &id, Name: old.Name, CycleDays: old.CycleDays, PhaseOffset: old.PhaseOffset, Color: old.Color})
			}
		}
	} else {
		for _, m := range cal.Moons {
			id := m.ID
			edit.Moons = append(edit.Moons, calendar.MoonInput{ID: &id, Name: m.Name, CycleDays: m.CycleDays, PhaseOffset: m.PhaseOffset, Color: m.Color})
		}
	}
	if s.seasons != nil {
		for _, sn := range s.seasons {
			for _, old := range cal.Seasons {
				if sameName(old.Name, sn.Name) {
					sn.ID, sn.Description, sn.WeatherEffect = old.ID, old.Description, old.WeatherEffect
					if sn.Color == "" {
						sn.Color = old.Color
					}
					break
				}
			}
			edit.Seasons = append(edit.Seasons, sn)
		}
	} else if s.months != nil {
		// Seasons are stored by month number, so when the months change each
		// kept season follows its months by name.
		for _, sn := range cal.Seasons {
			sm, em := movedMonth(cal, s.months, sn.StartMonth), movedMonth(cal, s.months, sn.EndMonth)
			if sm == 0 || em == 0 {
				return nil, badRequestf("season %q starts or ends in a month this block removes; give seasons too", sn.Name)
			}
			sn.StartMonth, sn.EndMonth = sm, em
			edit.Seasons = append(edit.Seasons, sn)
		}
	} else {
		edit.Seasons = append(edit.Seasons, cal.Seasons...)
	}
	return edit, nil
}

// movedMonth is the new number of old month n, matched by name, or 0.
func movedMonth(cal *calendar.Calendar, months []calendar.MonthInput, n int) int {
	if n < 1 || n > len(cal.Months) {
		return 0
	}
	for i, m := range months {
		if sameName(m.Name, cal.Months[n-1].Name) {
			return i + 1
		}
	}
	return 0
}

func baseMoonIn(ms []calendar.MoonInput, name string) *calendar.MoonInput {
	for i := range ms {
		if sameName(ms[i].Name, name) {
			return &ms[i]
		}
	}
	return nil
}

func (k CalendarKind) planUpdate(ctx context.Context, campaignID string, r Record, cal *calendar.Calendar) Plan {
	if r.Name != "" && !sameName(r.Name, cal.Name) {
		return Plan{Error: fmt.Sprintf("the main calendar is %s, not %s; to rename it, use rename_to", quote(cal.Name), quote(r.Name))}
	}
	s, err := readCalSpec(r, cal)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	var did []string
	warn := s.warnings
	if to := r.Str("rename_to"); to != "" {
		did = append(did, "renames it "+quote(to))
	}
	if s.yearLabel != nil {
		did = append(did, "sets the year label to "+quote(*s.yearLabel))
	}
	if strings.TrimSpace(r.Body) != "" {
		did = append(did, "replaces its description")
	}
	edit, err := structureEdit(s, cal)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if edit != nil {
		if cal.UsesRealTime() {
			return Plan{Error: "this calendar follows the real-world date, so its months, weekdays, seasons and moons can't be changed"}
		}
		pv, err := k.Svc.PreviewStructureEdit(ctx, cal.ID, campaignID, *edit)
		if err != nil {
			return Plan{Error: planError(err)}
		}
		for _, key := range []string{"months", "weekdays", "seasons", "moons"} {
			if r.Has(key) || (key == "seasons" && s.seasons != nil) {
				did = append(did, "replaces the "+key)
			}
		}
		if s.leapEvery != nil {
			did = append(did, "changes the leap rule")
		}
		warn = append(warn, structureWarnings(pv)...)
	}
	if s.today != nil {
		if cal.UsesRealTime() {
			return Plan{Error: "this calendar follows the real-world date, so its current date can't be set"}
		}
		did = append(did, "sets today to "+dateLabel(s.next, s.today.Y, s.today.M, s.today.D))
	}
	if len(s.eras) > 0 {
		added, changed := 0, 0
		for _, e := range s.eras {
			if findEra(cal, e.in.Name) != nil {
				changed++
			} else if !e.hasStart {
				return Plan{Error: fmt.Sprintf("era %q is new, so it needs start_year", e.in.Name)}
			} else {
				added++
			}
		}
		if added > 0 {
			did = append(did, "adds "+plural(added, "era", "eras"))
		}
		if changed > 0 {
			did = append(did, "changes "+plural(changed, "era", "eras"))
		}
	}
	if len(did) == 0 {
		return Plan{Error: "this block changes nothing; give the keys to change"}
	}
	return Plan{Summary: strings.Join(did, "; ") + " on " + cal.Name, Warnings: warn}
}

// structureWarnings turns the structure preview into review-row warnings.
func structureWarnings(pv *calendar.StructurePreview) []string {
	var out []string
	out = append(out, pv.MonthNotes...)
	out = append(out, pv.OtherNotes...)
	if pv.MovedEvents > 0 {
		out = append(out, plural(pv.MovedEvents, "event follows its month", "events follow their months")+" to the month's new place")
	}
	if pv.RedatedTotal > 0 {
		out = append(out, plural(pv.RedatedTotal, "event now falls", "events now fall")+" in a different month")
	}
	if pv.StrandedTotal > 0 {
		out = append(out, plural(pv.StrandedTotal, "event is", "events are")+" on a day that will no longer exist; kept, but not shown")
	}
	if pv.CurrentDateClamped {
		out = append(out, "Today's date no longer exists and moves to "+pv.NewCurrentDate)
	}
	return append(out, pv.Warnings...)
}

func findEra(cal *calendar.Calendar, name string) *calendar.Era {
	for i := range cal.Eras {
		if sameName(cal.Eras[i].Name, name) {
			return &cal.Eras[i]
		}
	}
	return nil
}

func (k CalendarKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, _ := k.plan(ctx, campaignID, a, r)
	if p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	if r.Action == ActionCreate {
		return k.applyCreate(ctx, campaignID, r)
	}
	cal, err := k.existing(ctx, campaignID, a)
	if err != nil {
		return err
	}
	if cal == nil {
		return apperror.NewBadRequest("the calendar is gone")
	}
	return k.applyUpdate(ctx, campaignID, r, cal)
}

func (k CalendarKind) applyCreate(ctx context.Context, campaignID string, r Record) error {
	s, err := readCalSpec(r, nil)
	if err != nil {
		return err
	}
	ir := s.importResult(r)
	if err := calendar.PrepareImport(ir); err != nil {
		return err
	}
	cal, err := k.Svc.CreateCalendarFromImport(ctx, campaignID, ir, calendar.CreateCalendarFromImportOptions{})
	if err != nil {
		return err
	}
	// The service never marks a new calendar as the main one, and every
	// other block (and the calendar page) reads the main one.
	return k.Svc.SetDefaultCalendar(ctx, campaignID, cal.ID)
}

// applyUpdate writes the structure first, so the current date and eras
// are checked against the months the calendar now has.
func (k CalendarKind) applyUpdate(ctx context.Context, campaignID string, r Record, cal *calendar.Calendar) error {
	s, err := readCalSpec(r, cal)
	if err != nil {
		return err
	}
	edit, err := structureEdit(s, cal)
	if err != nil {
		return err
	}
	if edit != nil {
		pv, err := k.Svc.PreviewStructureEdit(ctx, cal.ID, campaignID, *edit)
		if err != nil {
			return err
		}
		if _, err := k.Svc.ApplyStructureEdit(ctx, cal.ID, campaignID, pv.Fingerprint, *edit); err != nil {
			return err
		}
	}
	in := calendar.UpdateCalendarInput{Name: cal.Name}
	changed := false
	if to := r.Str("rename_to"); to != "" {
		in.Name, changed = to, true
	}
	if s.yearLabel != nil {
		in.EpochName, changed = patch.Of(*s.yearLabel), true
	}
	if b := strings.TrimSpace(r.Body); b != "" {
		in.Description, changed = patch.Of(b), true
	}
	if changed {
		if err := k.Svc.UpdateCalendar(ctx, cal.ID, campaignID, in); err != nil {
			return err
		}
	}
	if s.today != nil {
		if err := k.Svc.SetCurrentDate(ctx, cal.ID, campaignID, s.today.Y, s.today.M, s.today.D, cal.CurrentHour, cal.CurrentMinute); err != nil {
			return err
		}
	}
	for _, e := range s.eras {
		old := findEra(cal, e.in.Name)
		if old == nil {
			if _, err := k.Svc.CreateEra(ctx, cal.ID, campaignID, e.in); err != nil {
				return err
			}
			continue
		}
		if err := k.Svc.UpdateEra(ctx, old.ID, cal.ID, campaignID, eraUpdate(e)); err != nil {
			return err
		}
	}
	return nil
}

// eraUpdate changes only what the era entry wrote.
func eraUpdate(e eraSpec) calendar.UpdateEraInput {
	in := calendar.UpdateEraInput{Name: e.in.Name}
	if e.hasStart {
		in.StartYear, in.StartMonth, in.StartDay = patch.Of(e.in.StartYear), patch.Of(e.in.StartMonth), patch.Of(e.in.StartDay)
	}
	if e.in.EndYear != nil {
		in.EndYear = patch.Of(*e.in.EndYear)
		if e.in.EndMonth != nil {
			in.EndMonth, in.EndDay = patch.Of(*e.in.EndMonth), patch.Of(*e.in.EndDay)
		} else {
			in.EndMonth, in.EndDay = patch.Null[int](), patch.Null[int]()
		}
	}
	if e.in.Color != "" {
		in.Color = patch.Of(e.in.Color)
	}
	if e.in.Description != nil {
		in.Description = patch.Of(*e.in.Description)
	}
	return in
}

// Export lists the main calendar as a block, so an AI can change it by
// name and see the months its dates must use.
func (k CalendarKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	cal, err := k.existing(ctx, campaignID, a)
	if err != nil || cal == nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\nkind: calendar\naction: update\nname: " + quote(cal.Name) + "\n")
	if cal.EpochName != nil && *cal.EpochName != "" {
		b.WriteString("year_label: " + quote(*cal.EpochName) + "\n")
	}
	if len(cal.Months) > 0 {
		fmt.Fprintf(&b, "current_year: %d\ncurrent_month: %s\ncurrent_day: %d\n", cal.CurrentYear, quote(cal.Months[clampIdx(cal.CurrentMonth-1, len(cal.Months))].Name), cal.CurrentDay)
	}
	if cal.LeapYearEvery > 0 {
		fmt.Fprintf(&b, "leap_year_every: %d\n", cal.LeapYearEvery)
	}
	b.WriteString("months:\n")
	for _, m := range cal.Months {
		fmt.Fprintf(&b, "  - {name: %s, days: %d", quote(m.Name), m.Days)
		if m.IsIntercalary {
			b.WriteString(", intercalary: true")
		}
		if m.LeapYearDays > 0 {
			fmt.Fprintf(&b, ", leap_days: %d", m.LeapYearDays)
		}
		b.WriteString("}\n")
	}
	b.WriteString("weekdays:\n")
	for _, w := range cal.Weekdays {
		if w.IsRestDay {
			fmt.Fprintf(&b, "  - {name: %s, rest_day: true}\n", quote(w.Name))
		} else {
			b.WriteString("  - " + quote(w.Name) + "\n")
		}
	}
	if len(cal.Seasons) > 0 {
		b.WriteString("seasons:\n")
		for _, s := range cal.Seasons {
			fmt.Fprintf(&b, "  - {name: %s, start_month: %d, start_day: %d, end_month: %d, end_day: %d, color: %s}\n", quote(s.Name), s.StartMonth, s.StartDay, s.EndMonth, s.EndDay, quote(s.Color))
		}
	}
	if len(cal.Moons) > 0 {
		b.WriteString("moons:\n")
		for _, m := range cal.Moons {
			fmt.Fprintf(&b, "  - {name: %s, cycle: %v, phase_offset: %v, color: %s}\n", quote(m.Name), m.CycleDays, m.PhaseOffset, quote(m.Color))
		}
	}
	// Eras are calendar structure only the Director sees; the service has
	// already left them out for anyone else.
	if len(cal.Eras) > 0 {
		b.WriteString("eras:\n")
		for _, e := range cal.Eras {
			fmt.Fprintf(&b, "  - {name: %s, start_year: %d, start_month: %d, start_day: %d", quote(e.Name), e.StartYear, e.StartMonth, e.StartDay)
			if e.EndYear != nil {
				fmt.Fprintf(&b, ", end_year: %d", *e.EndYear)
				if e.EndMonth != nil && e.EndDay != nil {
					fmt.Fprintf(&b, ", end_month: %d, end_day: %d", *e.EndMonth, *e.EndDay)
				}
			}
			b.WriteString("}\n")
		}
	}
	b.WriteString("---\n\n")
	return b.String(), nil
}

func clampIdx(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}
