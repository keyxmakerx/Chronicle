package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// CalendarAPI is the slice of calendar.CalendarService the event, weather
// and generator kinds use. Every call is campaign-scoped by the service.
type CalendarAPI interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
	ListEventsForCalendar(ctx context.Context, campaignID, calendarID string, role int) ([]calendar.Event, error)
	ListEventsForMonth(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]calendar.Event, error)
	CreateEvent(ctx context.Context, calendarID, campaignID string, input calendar.CreateEventInput) (*calendar.Event, error)
	UpdateEvent(ctx context.Context, eventID, calendarID, campaignID string, input calendar.UpdateEventInput, v permissions.Viewer) error
	DeleteEvent(ctx context.Context, eventID, calendarID, campaignID string, v permissions.Viewer) error
	ListDayWeather(ctx context.Context, calendarID, campaignID string, year, month int, v permissions.Viewer) ([]calendar.DayWeather, error)
	SetDayWeather(ctx context.Context, calendarID, campaignID string, days []calendar.DayWeatherInput) error
	ClearDayWeather(ctx context.Context, calendarID, campaignID string, dates []calendar.DayDate) error
}

func defaultCalendar(ctx context.Context, svc CalendarAPI, campaignID string, a Actor) (*calendar.Calendar, error) {
	cal, err := svc.GetDefaultCalendarForViewer(ctx, campaignID, a.Viewer())
	if err != nil || cal == nil {
		return nil, apperror.NewBadRequest("this campaign has no calendar yet")
	}
	return cal, nil
}

// readDate reads year/month/day keys (with a prefix such as "end_"). Month
// may be a number or one of the calendar's month names.
func readDate(cal *calendar.Calendar, r Record, prefix string) (y, m, d int, present bool, err error) {
	y, yok, err := r.Int(prefix + "year")
	if err != nil {
		return 0, 0, 0, true, err
	}
	d, dok, err := r.Int(prefix + "day")
	if err != nil {
		return 0, 0, 0, true, err
	}
	ms := r.Str(prefix + "month")
	if !yok && !dok && ms == "" {
		return 0, 0, 0, false, nil
	}
	if !yok || !dok || ms == "" {
		return 0, 0, 0, true, badRequestf("%syear, %smonth and %sday are all needed", prefix, prefix, prefix)
	}
	m, _, err = r.Int(prefix + "month")
	if err != nil {
		m = 0
		for i, mo := range cal.Months {
			if sameName(mo.Name, ms) {
				m = i + 1
			}
		}
		if m == 0 {
			return 0, 0, 0, true, badRequestf("%smonth: %q is not a month of %s", prefix, ms, cal.Name)
		}
	}
	if m < 1 || m > len(cal.Months) {
		return 0, 0, 0, true, badRequestf("%smonth: %d is not a month of %s", prefix, m, cal.Name)
	}
	if d < 1 || d > cal.MonthDays(m-1, y) {
		return 0, 0, 0, true, badRequestf("%sday: %s has no day %d", prefix, cal.Months[m-1].Name, d)
	}
	return y, m, d, true, nil
}

func dateLabel(cal *calendar.Calendar, y, m, d int) string {
	if m >= 1 && m <= len(cal.Months) {
		return fmt.Sprintf("%s %d, %d", cal.Months[m-1].Name, d, y)
	}
	return fmt.Sprintf("%d-%d-%d", y, m, d)
}

func bodyHTML(body string) (*string, error) {
	if strings.TrimSpace(body) == "" {
		return nil, nil
	}
	h, err := importer.MarkdownToHTML(body)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// ---- calendar events ----

// EventKind adds, changes and removes calendar events on the default calendar.
type EventKind struct{ Svc CalendarAPI }

func (EventKind) Name() string  { return "event" }
func (EventKind) Label() string { return "Calendar event" }
func (EventKind) Doc() string {
	return "A calendar event, matched by `name`. Keys: `year`, `month` (number or month name), `day`; optional `end_year`/`end_month`/`end_day`, `hour`, `minute` (whole numbers), `all_day` (true/false), `visibility` (`everyone` or `dm_only`), `color` (#hex), `icon` (Font Awesome name), `rename_to`. The body is the description. When two events share a name, give the date too. On `action: update`, `year`/`month`/`day` say which event to change; to move it to another date add `move_to_year`/`move_to_month`/`move_to_day` (all three).\n\n```\n---\nkind: event\nname: Midwinter Feast\nyear: 1492\nmonth: Hammer\nday: 21\nvisibility: everyone\n---\nThe lords of the city open their halls.\n```"
}

// find returns the event this record names; a date narrows same-named events.
func (k EventKind) find(ctx context.Context, campaignID string, a Actor, cal *calendar.Calendar, r Record) (*calendar.Event, error) {
	// Role-filtered, so the operator matches only events they can see.
	evs, err := k.Svc.ListEventsForCalendar(ctx, campaignID, cal.ID, a.Role)
	if err != nil {
		return nil, apperror.NewBadRequest("could not read the calendar's events")
	}
	y, m, d, dated, _ := readDate(cal, r, "")
	var hits []calendar.Event
	for _, e := range evs {
		if !sameName(e.Name, r.Name) {
			continue
		}
		if dated && (e.Year != y || e.Month != m || e.Day != d) {
			continue
		}
		hits = append(hits, e)
	}
	switch len(hits) {
	case 0:
		return nil, nil
	case 1:
		return &hits[0], nil
	}
	return nil, badRequestf("%d events are called %q; add year, month and day to say which", len(hits), r.Name)
}

func (k EventKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Name == "" {
		return Plan{Error: "an event needs a name"}
	}
	cal, err := defaultCalendar(ctx, k.Svc, campaignID, a)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	vis := r.Str("visibility")
	if vis != "" && vis != "everyone" && vis != "dm_only" {
		return Plan{Error: "visibility must be everyone or dm_only"}
	}
	if vis == "dm_only" && !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can add hidden events"}
	}
	existing, err := k.find(ctx, campaignID, a, cal, r)
	if err != nil && r.Action != ActionCreate {
		return Plan{Error: planError(err)}
	}
	y, m, d, dated, derr := readDate(cal, r, "")
	if derr != nil {
		return Plan{Error: planError(derr)}
	}
	if _, _, _, _, err := readDate(cal, r, "end_"); err != nil {
		return Plan{Error: planError(err)}
	}
	// A non-numeric hour or minute would otherwise be read as 0 and saved.
	for _, key := range []string{"hour", "minute"} {
		if _, _, err := r.Int(key); err != nil {
			return Plan{Error: planError(err)}
		}
	}
	my, mm, md, moving, merr := readDate(cal, r, "move_to_")
	if merr != nil {
		return Plan{Error: planError(merr)}
	}
	if moving && r.Action != ActionUpdate {
		return Plan{Error: "move_to_year, move_to_month and move_to_day only apply to action: update"}
	}
	switch r.Action {
	case ActionCreate:
		if !dated {
			return Plan{Error: "a new event needs year, month and day"}
		}
		p := Plan{Summary: dateLabel(cal, y, m, d) + " on " + cal.Name}
		if existing != nil {
			p.Warnings = append(p.Warnings, "An event with this name already exists; this adds another")
		}
		return p
	case ActionUpdate:
		if existing == nil {
			return Plan{Error: "no event called " + quote(r.Name) + " to change"}
		}
		if moving {
			return Plan{Summary: "moves the event on " + dateLabel(cal, existing.Year, existing.Month, existing.Day) + " to " + dateLabel(cal, my, mm, md)}
		}
		return Plan{Summary: "changes the event on " + dateLabel(cal, existing.Year, existing.Month, existing.Day)}
	default:
		if existing == nil {
			return Plan{Error: "no event called " + quote(r.Name) + " to remove"}
		}
		return Plan{Summary: "removes the event on " + dateLabel(cal, existing.Year, existing.Month, existing.Day)}
	}
}

func (k EventKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	cal, err := defaultCalendar(ctx, k.Svc, campaignID, a)
	if err != nil {
		return err
	}
	desc, err := bodyHTML(r.Body)
	if err != nil {
		return err
	}
	y, m, d, _, _ := readDate(cal, r, "")
	ey, em, ed, ended, _ := readDate(cal, r, "end_")
	if r.Action == ActionCreate {
		in := calendar.CreateEventInput{
			Name: r.Name, Year: y, Month: m, Day: d,
			DescriptionHTML: desc, Visibility: r.Str("visibility"),
			CreatedBy: a.UserID, CanAuthorDmOnly: a.CanAuthorDmOnly(), Author: a.Viewer(),
		}
		if ended {
			in.EndYear, in.EndMonth, in.EndDay = &ey, &em, &ed
		}
		if h, ok, _ := r.Int("hour"); ok {
			in.StartHour = &h
			mi, _, _ := r.Int("minute")
			in.StartMinute = &mi
		}
		if v, ok := r.Bool("all_day"); ok {
			in.AllDay = v
		}
		if s := r.Str("color"); s != "" {
			in.Color = &s
		}
		if s := r.Str("icon"); s != "" {
			in.Icon = &s
		}
		_, err := k.Svc.CreateEvent(ctx, cal.ID, campaignID, in)
		return err
	}
	ev, err := k.find(ctx, campaignID, a, cal, r)
	if err != nil {
		return err
	}
	if ev == nil {
		return apperror.NewBadRequest("the event is gone")
	}
	if r.Action == ActionDelete {
		return k.Svc.DeleteEvent(ctx, ev.ID, cal.ID, campaignID, a.Viewer())
	}
	// Partial update: only keys the AI wrote change (absent keeps).
	var in calendar.UpdateEventInput
	if s := r.Str("rename_to"); s != "" {
		in.Name = patch.Of(s)
	}
	if desc != nil {
		in.DescriptionHTML = patch.Of(*desc)
	}
	// year/month/day only pick the event; move_to_* is the new date.
	if my, mm, md, moving, _ := readDate(cal, r, "move_to_"); moving {
		in.Year, in.Month, in.Day = patch.Of(my), patch.Of(mm), patch.Of(md)
	}
	if ended {
		in.EndYear, in.EndMonth, in.EndDay = patch.Of(ey), patch.Of(em), patch.Of(ed)
	}
	if h, ok, _ := r.Int("hour"); ok {
		in.StartHour = patch.Of(h)
	}
	if mi, ok, _ := r.Int("minute"); ok {
		in.StartMinute = patch.Of(mi)
	}
	if s := r.Str("visibility"); s != "" {
		in.Visibility = patch.Of(s)
	}
	if s := r.Str("color"); s != "" {
		in.Color = patch.Of(s)
	}
	if s := r.Str("icon"); s != "" {
		in.Icon = patch.Of(s)
	}
	if v, ok := r.Bool("all_day"); ok {
		in.AllDay = patch.Of(v)
	}
	return k.Svc.UpdateEvent(ctx, ev.ID, cal.ID, campaignID, in, a.Viewer())
}

// Export is empty: the Calendar events category of the export already
// lists events, filtered by the chosen privacy mode.
func (EventKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

// ---- day weather ----

// WeatherKind sets or clears one day's weather by hand. A hand-set day is
// what a later generated run leaves alone.
type WeatherKind struct{ Svc CalendarAPI }

func (WeatherKind) Name() string  { return "weather" }
func (WeatherKind) Label() string { return "Weather" }
func (WeatherKind) Doc() string {
	return "One day's weather on the default calendar. Keys: `year`, `month`, `day`; `label` (e.g. Light rain), optional `icon`, `color`, `temperature` (°C), `wind_kph`, `precipitation` (rain, snow, sleet, hail), `description`. `action: delete` clears the day. `name` is optional.\n\n```\n---\nkind: weather\nyear: 1492\nmonth: Hammer\nday: 3\nlabel: Blizzard\ntemperature: -12\nprecipitation: snow\n---\n```"
}

func (k WeatherKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can set weather"}
	}
	cal, err := defaultCalendar(ctx, k.Svc, campaignID, a)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	y, m, d, dated, err := readDate(cal, r, "")
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if !dated {
		return Plan{Error: "weather needs year, month and day"}
	}
	for _, key := range []string{"temperature", "wind_kph"} {
		if _, _, err := r.Float(key); err != nil {
			return Plan{Error: planError(err)}
		}
	}
	warn := k.dayWarning(ctx, campaignID, a, cal, y, m, d, r.Action == ActionDelete)
	if r.Action == ActionDelete {
		return Plan{Summary: "clears the weather on " + dateLabel(cal, y, m, d), Warnings: warn}
	}
	if r.Str("label") == "" && r.Str("description") == "" {
		return Plan{Error: "weather needs a label or a description"}
	}
	return Plan{Summary: r.Str("label") + " on " + dateLabel(cal, y, m, d), Warnings: warn}
}

// dayWarning says when the record would overwrite a day the owner locked or
// set by hand. A hand-written record saves as manual and so also unlocks the
// day, which the owner may not expect from an import. A failed read just
// means no warning; the write itself is unaffected.
func (k WeatherKind) dayWarning(ctx context.Context, campaignID string, a Actor, cal *calendar.Calendar, y, m, d int, clearing bool) []string {
	days, err := k.Svc.ListDayWeather(ctx, cal.ID, campaignID, y, m, a.Viewer())
	if err != nil {
		return nil
	}
	verb := "replaces it"
	if clearing {
		verb = "clears it"
	}
	for _, day := range days {
		if day.Year != y || day.Month != m || day.Day != d {
			continue
		}
		switch {
		case day.Locked != nil && *day.Locked:
			return []string{"This day is locked; this " + verb + " and unlocks it"}
		case day.Source == calendar.WeatherSourceManual:
			return []string{"This day's weather was set by hand; this " + verb}
		}
	}
	return nil
}

func (k WeatherKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	cal, err := defaultCalendar(ctx, k.Svc, campaignID, a)
	if err != nil {
		return err
	}
	y, m, d, _, _ := readDate(cal, r, "")
	if r.Action == ActionDelete {
		return k.Svc.ClearDayWeather(ctx, cal.ID, campaignID, []calendar.DayDate{{Year: y, Month: m, Day: d}})
	}
	w := calendar.WeatherInput{}
	str := func(key string) *string {
		if s := r.Str(key); s != "" {
			return &s
		}
		return nil
	}
	w.PresetLabel, w.Icon, w.Color, w.PrecipitationType, w.Description = str("label"), str("icon"), str("color"), str("precipitation"), str("description")
	if f, ok, _ := r.Float("temperature"); ok {
		w.TemperatureCelsius = &f
	}
	if f, ok, _ := r.Float("wind_kph"); ok {
		w.WindSpeedKPH = &f
	}
	return k.Svc.SetDayWeather(ctx, cal.ID, campaignID, []calendar.DayWeatherInput{{
		Year: y, Month: m, Day: d, Source: calendar.WeatherSourceManual, WeatherInput: w,
	}})
}

// Export is empty on purpose: a year of days is too long to paste, and
// days are addressed by date, so the AI never needs the list to change one.
func (WeatherKind) Export(context.Context, string, Actor) (string, error) { return "", nil }
