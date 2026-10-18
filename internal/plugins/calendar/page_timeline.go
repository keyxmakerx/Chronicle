// Package calendar - page_timeline.go answers the time line block's question
// "which dated events belong to this page, and where do they sit between the
// first of them and the campaign's today, or the subject's death?". It reads
// one calendar, the one the campaign uses, so every bead is placed on the
// same day counter.
package calendar

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// maxPageTimelineEvents caps the dots one line draws: past this the dots
// would touch, and the calendar itself lists the rest.
const maxPageTimelineEvents = 200

// pageTimelineLabelGap is how far apart, in percent of the line, two year
// labels must be so they never overlap.
const pageTimelineLabelGap = 9.0

// PageTimeline is one page's events on its campaign's calendar, already
// filtered for the viewer, with the day numbers the line is drawn from.
type PageTimeline struct {
	CalendarID   string
	CalendarName string
	// TodayLabel is the campaign's today, as the calendar writes a date.
	TodayLabel string
	Events     []PageTimelineEvent
	// Total counts the events the viewer may see before the cap.
	Total int
	// Start and End bound the line and Today marks the calendar's current
	// date, all as Calendar.AbsoluteDay numbers.
	Start, End, Today int
	// Dead is set when one of the page's events is a death (isDeathKind);
	// DeathAbs is that day. The line is then lit to the death, not to today.
	Dead     bool
	DeathAbs int
	// OpenURL and OpenLabel are the link under the line: the calendar by
	// default, or a time line page that follows this calendar when the app
	// finds one (the calendar plugin cannot see time lines itself).
	OpenURL, OpenLabel string
}

// PageTimelineEvent is one dot: what the peek says and where it goes.
type PageTimelineEvent struct {
	ID        string
	Name      string
	Year      int
	Month     int
	Day       int
	DateLabel string
	KindName  string
	KindIcon  string
	// Abs is the event's day on the calendar's day counter.
	Abs int
	// DMOnly marks an event only the Director sees; a player's line never
	// holds one.
	DMOnly bool
	// Repeats is true for a repeating event, which sits on its first date.
	Repeats bool
	// EndLabel is the last day of an event that runs over several, as the
	// calendar writes a date; empty for a one-day event.
	EndLabel string
	// Death marks the event that ends a dead subject's line.
	Death bool
	// Labelled says whether the dot carries its year under it; see
	// labelYears.
	Labelled bool
}

// Truncated reports whether the cap left events off the line.
func (t *PageTimeline) Truncated() bool { return t.Total > len(t.Events) }

// Percent places a day on the line, 0 at Start and 100 at End. A line one
// day long puts everything in the middle.
func (t *PageTimeline) Percent(abs int) float64 {
	if t.End <= t.Start {
		return 50
	}
	p := float64(abs-t.Start) / float64(t.End-t.Start) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// FillPercent is how far the filled part of the line runs: up to today, or
// up to the death on a dead subject's line.
func (t *PageTimeline) FillPercent() float64 {
	if t.Dead {
		return t.Percent(t.DeathAbs)
	}
	if t.Today < t.Start {
		return 0
	}
	return t.Percent(t.Today)
}

// ShowsToday says whether the line marks today: a living subject's line that
// reaches it.
func (t *PageTimeline) ShowsToday() bool {
	return !t.Dead && t.Today >= t.Start && t.Today <= t.End
}

// Later says whether an event is still to come. A dead subject's line has
// no today, so nothing on it is.
func (t *PageTimeline) Later(e PageTimelineEvent) bool {
	return !t.Dead && e.Abs > t.Today
}

// isDeathKind says whether an event's kind is a death. Chronicle stores no
// death date for a page, so a Death event on the calendar is what ends a
// subject's line; "Died" is accepted for campaigns that named the kind so.
func isDeathKind(e Event) bool {
	return e.KindSlug == "death" || strings.EqualFold(strings.TrimSpace(e.KindName), "death") || strings.EqualFold(strings.TrimSpace(e.KindName), "died")
}

// newPageTimeline lays events (already filtered) on a line from the first of
// them to today, or to the last when some lie ahead. A page with a death
// event runs from the first to the death (or a later event), whatever today
// is.
func newPageTimeline(cal *Calendar, events []Event) *PageTimeline {
	t := &PageTimeline{
		CalendarID:   cal.ID,
		CalendarName: cal.Name,
		TodayLabel:   cal.FullDateLabel(),
		Today:        cal.CurrentAbsoluteDay(),
		Total:        len(events),
		OpenURL:      fmt.Sprintf("/campaigns/%s/calendars/%s/view", cal.CampaignID, cal.ID),
		OpenLabel:    "Open the calendar",
	}
	for _, e := range events {
		pe := PageTimelineEvent{
			ID:        e.ID,
			Name:      e.Name,
			Year:      e.Year,
			Month:     e.Month,
			Day:       e.Day,
			DateLabel: fmt.Sprintf("%s %d, %d", cal.MonthName(e.Month), e.Day, e.Year),
			KindName:  e.KindName,
			KindIcon:  e.KindIcon,
			Abs:       cal.AbsoluteDay(e.Year, e.Month, e.Day),
			DMOnly:    e.Visibility == "dm_only",
			Repeats:   e.IsRecurring,
			Death:     isDeathKind(e),
		}
		if e.EndYear != nil && e.EndMonth != nil && e.EndDay != nil {
			pe.EndLabel = fmt.Sprintf("%s %d, %d", cal.MonthName(*e.EndMonth), *e.EndDay, *e.EndYear)
		}
		t.Events = append(t.Events, pe)
	}
	sort.SliceStable(t.Events, func(i, j int) bool { return t.Events[i].Abs < t.Events[j].Abs })
	// The latest death wins, so a subject raised and slain again ends at the second.
	for i := len(t.Events) - 1; i >= 0; i-- {
		if t.Events[i].Death {
			t.Dead, t.DeathAbs = true, t.Events[i].Abs
			break
		}
	}
	for i := range t.Events {
		if t.Events[i].Death && t.Events[i].Abs != t.DeathAbs {
			t.Events[i].Death = false
		}
	}
	if len(t.Events) > maxPageTimelineEvents {
		t.Events = t.Events[:maxPageTimelineEvents]
	}
	if len(t.Events) == 0 {
		t.Start, t.End = t.Today, t.Today
		return t
	}
	t.Start = t.Events[0].Abs
	t.End = t.Events[len(t.Events)-1].Abs
	if t.Dead {
		if t.DeathAbs > t.End {
			t.End = t.DeathAbs
		}
	} else {
		if t.Today < t.Start {
			t.Start = t.Today
		}
		if t.Today > t.End {
			t.End = t.Today
		}
	}
	labelYears(t)
	return t
}

// labelYears marks which dots carry their year: the first dot, then any dot
// in a new year far enough along the line from the last label to fit.
func labelYears(t *PageTimeline) {
	last := -100.0
	lastYear := 0
	for i := range t.Events {
		e := &t.Events[i]
		p := t.Percent(e.Abs)
		if (i == 0 || e.Year != lastYear) && p-last >= pageTimelineLabelGap {
			e.Labelled = true
			last, lastYear = p, e.Year
		}
	}
}

// PageTimeline: see the CalendarService doc comment.
func (s *calendarService) PageTimeline(ctx context.Context, campaignID, entityID string, hideDMOnly bool, v permissions.Viewer) (*PageTimeline, error) {
	cal, err := s.calRepo.GetDefaultByCampaignID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if cal == nil {
		all, err := s.calRepo.ListByCampaignID(ctx, campaignID)
		if err != nil {
			return nil, err
		}
		if len(all) > 0 {
			cal = &all[0]
		}
	}
	if cal == nil || cal.CampaignID != campaignID || !calendarVisibleToViewer(*cal, v) {
		return nil, apperror.NewNotFound("calendar not found")
	}
	if err := s.loadCalendarGeometry(ctx, cal); err != nil {
		return nil, err
	}
	events, err := s.eventRepo.ListEventsForPage(ctx, cal.ID, entityID, v.Role())
	if err != nil {
		return nil, fmt.Errorf("list events for page: %w", err)
	}
	events = filterEventsByUser(events, v)
	if !v.SkipsPerUserRules() {
		if events, err = s.hideFromPlayer(ctx, cal, campaignID, events); err != nil {
			return nil, err
		}
	}
	if hideDMOnly {
		kept := events[:0]
		for _, e := range events {
			if e.Visibility != "dm_only" {
				kept = append(kept, e)
			}
		}
		events = kept
	}
	return newPageTimeline(cal, events), nil
}
