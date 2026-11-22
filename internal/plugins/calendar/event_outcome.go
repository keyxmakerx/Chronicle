// Package calendar - event_outcome.go is what became of an event: its
// status (planned, happened, moved, called off), the Director's line on
// what happened, the day a moved event was planned for, and the list of
// events whose day the Director has just moved today past, which the
// calendar asks about. The calendar asks when the day passes and stores
// these four words, so the page, the campaign AI and Foundry say the same
// thing.
package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The four things an event can have become. Planned is every event's
// start and stays until someone answers; once its day has passed, an
// unanswered event reads as happened.
const (
	EventStatusPlanned   = "planned"
	EventStatusHappened  = "happened"
	EventStatusMoved     = "moved"
	EventStatusCalledOff = "called_off"
)

// EventStatuses is the accepted set, in the order the event card offers it.
var EventStatuses = []string{EventStatusPlanned, EventStatusHappened, EventStatusMoved, EventStatusCalledOff}

// IsEventStatus reports whether s is one of EventStatuses.
func IsEventStatus(s string) bool {
	for _, ok := range EventStatuses {
		if s == ok {
			return true
		}
	}
	return false
}

// MaxOutcomeChars caps the line on what happened. It is a line, not the
// event's notes, which keep their own much larger limit.
const MaxOutcomeChars = 1000

// storedStatus is the value written to calendar_events.status: an event
// built without one (an older caller, a test) is planned.
func storedStatus(s string) string {
	if s == "" {
		return EventStatusPlanned
	}
	return s
}

// normalizeOutcome trims the line and turns an empty one into none, so a
// cleared field never stores a blank that reads as an answer.
func normalizeOutcome(p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(v) > MaxOutcomeChars {
		return nil, apperror.NewValidation(fmt.Sprintf("what happened can be up to %d characters", MaxOutcomeChars))
	}
	return &v, nil
}

// movedFrom is the day a moved event was planned for, or nil.
func (e *Event) movedFrom() *DayDate {
	if e.MovedFromYear == nil || e.MovedFromMonth == nil || e.MovedFromDay == nil {
		return nil
	}
	return &DayDate{Year: *e.MovedFromYear, Month: *e.MovedFromMonth, Day: *e.MovedFromDay}
}

func (e *Event) setMovedFrom(d *DayDate) {
	if d == nil {
		e.MovedFromYear, e.MovedFromMonth, e.MovedFromDay = nil, nil, nil
		return
	}
	y, m, day := d.Year, d.Month, d.Day
	e.MovedFromYear, e.MovedFromMonth, e.MovedFromDay = &y, &m, &day
}

// mergeEventStatus applies an update's status and line to evt, whose date
// fields already hold the merged date; was is the date stored before the
// update. The rules:
//
//   - Moved needs the event to land on another day; the first move keeps
//     the day it was planned for, and later moves keep that same first
//     day, so the mark stays where the event was meant to be.
//   - A moved event dated back onto its planned day is planned again.
//   - Any other status clears the planned-for day and keeps the date, so
//     setting a moved event back to Planned leaves it where it now is.
//   - A repeating event answers each date with a skip or move override, so
//     it refuses a status or line sent for the whole series; one made to
//     repeat goes back to planned.
func mergeEventStatus(evt *Event, was DayDate, in UpdateEventInput) error {
	status := storedStatus(in.Status.Val(evt.Status))
	if !IsEventStatus(status) {
		return apperror.NewValidation("status must be planned, happened, moved or called_off")
	}
	outcome, err := normalizeOutcome(in.Outcome.Ptr(evt.Outcome))
	if err != nil {
		return err
	}
	if evt.isRepeating() {
		if s, ok := in.Status.Get(); ok && s != EventStatusPlanned {
			return apperror.NewValidation("a repeating event is answered one date at a time: skip or move that date")
		}
		if v, ok := in.Outcome.Get(); ok && strings.TrimSpace(v) != "" {
			return apperror.NewValidation("a repeating event is answered one date at a time, so it takes no line on what happened")
		}
		evt.Status, evt.Outcome = EventStatusPlanned, outcome
		evt.setMovedFrom(nil)
		return nil
	}
	now := DayDate{Year: evt.Year, Month: evt.Month, Day: evt.Day}
	from := evt.movedFrom()
	if status == EventStatusMoved {
		switch {
		case from == nil && now == was:
			return apperror.NewValidation("a moved event needs the day it moved to")
		case from == nil:
			from = &was
		}
		if *from == now {
			status, from = EventStatusPlanned, nil
		}
	} else {
		from = nil
	}
	evt.Status, evt.Outcome = status, outcome
	evt.setMovedFrom(from)
	return nil
}

// buildEventStatus fills a new event's status from a create input, which
// only a campaign backup sets. It is lenient, as an import is: a status it
// cannot use leaves the event planned rather than refusing it.
func buildEventStatus(evt *Event, in CreateEventInput) error {
	outcome, err := normalizeOutcome(in.Outcome)
	if err != nil {
		return err
	}
	evt.Status, evt.Outcome = EventStatusPlanned, outcome
	evt.setMovedFrom(nil)
	if !IsEventStatus(in.Status) || evt.isRepeating() {
		return nil
	}
	if in.Status == EventStatusMoved {
		day := DayDate{Year: evt.Year, Month: evt.Month, Day: evt.Day}
		if in.MovedFrom == nil || *in.MovedFrom == day {
			return nil
		}
		evt.setMovedFrom(in.MovedFrom)
	}
	evt.Status = in.Status
	return nil
}

// PassedEvent is one event the calendar asks about after today moves past
// its day: a one-off event still planned whose last day is now behind, or
// one date of a repeating event. Year/Month/Day is the event's start (for a
// repeating event, the date in question).
type PassedEvent struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	Year       int    `json:"year"`
	Month      int    `json:"month"`
	Day        int    `json:"day"`
	EndYear    *int   `json:"end_year,omitempty"`
	EndMonth   *int   `json:"end_month,omitempty"`
	EndDay     *int   `json:"end_day,omitempty"`
	// Repeating marks one date of a repeating event, answered with a skip
	// or move override rather than the event's own status.
	Repeating bool    `json:"repeating"`
	Outcome   *string `json:"outcome,omitempty"`
}

// PassedEvents is the sheet's list; Truncated says more passed than it
// holds, which the sheet tells the Director in words.
type PassedEvents struct {
	Events    []PassedEvent `json:"events"`
	Truncated bool          `json:"truncated"`
}

// The sheet's bounds: a jump of years asks about its first stretch only,
// and says so, rather than reading every month in between.
const (
	maxPassedMonths = 24
	maxPassedEvents = 60
)

// ListPassedEvents lists what the calendar asks about when today moves from
// from to to: the events whose day is in [from, to), as v can see them. A
// one-off event is asked about once its last day has passed and only while
// it is still planned; a repeating event's dates are asked about one by
// one, skipped dates left out.
func (s *calendarService) ListPassedEvents(ctx context.Context, calendarID, campaignID string, from, to DayDate, v permissions.Viewer) (*PassedEvents, error) {
	cal, err := s.calendarInCampaignForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return nil, err
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}
	for _, d := range []DayDate{from, to} {
		if !validCalendarDay(cal, d) {
			return nil, apperror.NewValidation("from and to must be days of this calendar")
		}
	}
	out := &PassedEvents{Events: []PassedEvent{}}
	if !from.before(to) {
		return out, nil
	}
	seen := map[string]bool{}
	y, m := from.Year, from.Month
	for n := 0; y < to.Year || (y == to.Year && m <= to.Month); n++ {
		if n == maxPassedMonths {
			out.Truncated = true
			break
		}
		evs, err := s.ListEventsForMonth(ctx, calendarID, campaignID, y, m, v)
		if err != nil {
			return nil, err
		}
		for i := range evs {
			out.Events = appendPassed(out.Events, &evs[i], from, to, seen)
		}
		if m++; m > len(cal.Months) {
			y, m = y+1, 1
		}
	}
	sort.SliceStable(out.Events, func(i, j int) bool {
		a, b := out.Events[i], out.Events[j]
		if a.Year != b.Year || a.Month != b.Month || a.Day != b.Day {
			return dateLess(a.Year, a.Month, a.Day, b.Year, b.Month, b.Day)
		}
		return a.Name < b.Name
	})
	if len(out.Events) > maxPassedEvents {
		out.Events, out.Truncated = out.Events[:maxPassedEvents], true
	}
	return out, nil
}

// appendPassed adds e's passed dates in [from, to) to list, once each.
func appendPassed(list []PassedEvent, e *Event, from, to DayDate, seen map[string]bool) []PassedEvent {
	in := func(d DayDate) bool { return !d.before(from) && d.before(to) }
	if e.isRepeating() || len(e.Occurrences) > 0 {
		for _, o := range e.Occurrences {
			d := DayDate{Year: o.Year, Month: o.Month, Day: o.Day}
			key := fmt.Sprintf("%s|%d-%d-%d", e.ID, d.Year, d.Month, d.Day)
			if o.Skipped || !in(d) || seen[key] {
				continue
			}
			seen[key] = true
			list = append(list, PassedEvent{ID: e.ID, Name: e.Name, Visibility: e.Visibility,
				Year: d.Year, Month: d.Month, Day: d.Day, Repeating: true})
		}
		return list
	}
	if storedStatus(e.Status) != EventStatusPlanned || seen[e.ID] {
		return list
	}
	last := DayDate{Year: e.Year, Month: e.Month, Day: e.Day}
	if e.EndYear != nil && e.EndMonth != nil && e.EndDay != nil {
		last = DayDate{Year: *e.EndYear, Month: *e.EndMonth, Day: *e.EndDay}
	}
	if !in(last) {
		return list
	}
	seen[e.ID] = true
	return append(list, PassedEvent{ID: e.ID, Name: e.Name, Visibility: e.Visibility,
		Year: e.Year, Month: e.Month, Day: e.Day, EndYear: e.EndYear, EndMonth: e.EndMonth, EndDay: e.EndDay,
		Outcome: e.Outcome})
}

// validCalendarDay reports whether d is a day cal has. cal must carry its
// months.
func validCalendarDay(cal *Calendar, d DayDate) bool {
	if d.Month < 1 || d.Month > len(cal.Months) {
		return false
	}
	return d.Day >= 1 && d.Day <= cal.MonthDays(d.Month-1, d.Year)
}

// SetAskWhenDayPasses turns the calendar's question on or off. Owner only
// at the route, like the rest of Calendar settings.
func (s *calendarService) SetAskWhenDayPasses(ctx context.Context, calendarID, campaignID string, ask bool) error {
	if _, err := s.calendarInCampaign(ctx, calendarID, campaignID); err != nil {
		return err
	}
	return s.publishAfter(s.calRepo.SetAskWhenDayPasses(ctx, calendarID, ask), PubStructureUpdated, campaignID, calendarID)
}
