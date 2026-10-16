// Package calendar - page_timeline.go answers the time line block's question
// "which dated events belong to this page, and where do they sit between the
// first of them and the campaign's today?". It reads one calendar, the one
// the campaign uses, so every dot is placed on the same day counter.
package calendar

import (
	"context"
	"fmt"
	"sort"

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

// FillPercent is how far the filled part of the line runs: up to today.
func (t *PageTimeline) FillPercent() float64 {
	if t.Today < t.Start {
		return 0
	}
	return t.Percent(t.Today)
}

// newPageTimeline lays events (already filtered and in date order) on a
// line from the first of them to today, or to the last when some lie ahead.
func newPageTimeline(cal *Calendar, events []Event) *PageTimeline {
	t := &PageTimeline{
		CalendarID:   cal.ID,
		CalendarName: cal.Name,
		TodayLabel:   cal.FullDateLabel(),
		Today:        cal.CurrentAbsoluteDay(),
		Total:        len(events),
	}
	for _, e := range events {
		t.Events = append(t.Events, PageTimelineEvent{
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
		})
	}
	sort.SliceStable(t.Events, func(i, j int) bool { return t.Events[i].Abs < t.Events[j].Abs })
	if len(t.Events) > maxPageTimelineEvents {
		t.Events = t.Events[:maxPageTimelineEvents]
	}
	if len(t.Events) == 0 {
		t.Start, t.End = t.Today, t.Today
		return t
	}
	t.Start = t.Events[0].Abs
	t.End = t.Events[len(t.Events)-1].Abs
	if t.Today < t.Start {
		t.Start = t.Today
	}
	if t.Today > t.End {
		t.End = t.Today
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
