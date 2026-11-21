package sessions

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// NoRealDateWords is how every page says a night has no real-life date. A
// night made from a story calendar's event has only an in-world day, and a
// bare "–" there reads as a bug, so the words are shared by every place that
// would otherwise show a blank.
const NoRealDateWords = "No real date yet"

// WorldDateWorder words an in-world date in its calendar's own months ("14
// Hammer, 1492"). Implemented in internal/app over the calendar service, so
// this plugin never imports the calendar plugin. It answers "" when the date
// cannot be worded, and the page then says only that there is no real date.
//
// WorldDateWordsFor reads the campaign's calendars once and returns the
// wording function for that read, so a list of date-less nights costs one
// calendar read rather than one per row.
type WorldDateWorder interface {
	WorldDateWordsFor(ctx context.Context, campaignID string) func(calendarID string, year, month, day int) string
}

// SetWorldDateWorder wires the wording of a date-less night's in-world day.
func (h *Handler) SetWorldDateWorder(w WorldDateWorder) {
	h.worldWords = w
}

// hasRealDate reports whether the night is on a real-life day.
func (s *Session) hasRealDate() bool {
	return s.ScheduledDate != nil && *s.ScheduledDate != ""
}

// noRealDateWhen is what a page shows in place of a date for a night with no
// real one: the plain words, then the in-world day when it has one. It is
// never empty, so no row of a list is left blank.
func (h *Handler) noRealDateWhen(ctx context.Context, s *Session) string {
	return h.noRealDateWhenFn(ctx, s.CampaignID)(s)
}

// noRealDateWhenFn is noRealDateWhen for many nights of one campaign: the
// calendars are read once, on the first night that has an in-world day, and
// never for a list that has none.
func (h *Handler) noRealDateWhenFn(ctx context.Context, campaignID string) func(*Session) string {
	var words func(calendarID string, year, month, day int) string
	return func(s *Session) string {
		if h.worldWords == nil || !s.HasCalendarDate() {
			return NoRealDateWords
		}
		if words == nil {
			words = h.worldWords.WorldDateWordsFor(ctx, campaignID)
		}
		cal := ""
		if s.CalendarID != nil {
			cal = *s.CalendarID
		}
		w := strings.TrimSpace(words(cal, *s.CalendarYear, *s.CalendarMonth, *s.CalendarDay))
		if w == "" {
			return NoRealDateWords
		}
		return NoRealDateWords + " · " + w
	}
}

// NightDateNote is what a night's page says about its date when the date is
// not a plain real one: that there is none yet (with the in-world day), and
// the calendar event the night was made from.
type NightDateNote struct {
	NoRealDate bool
	// Words is "No real date yet · 14 Hammer, 1492" when NoRealDate.
	Words string
	// EventName and EventHref name and open the linked calendar event, empty
	// when the night has none or the viewer may not see that event.
	EventName string
	EventHref string
}

// nightDateNote builds the note for one night's page. The event is read again
// through the viewer's own role, so an event they cannot see is not named.
func (h *Handler) nightDateNote(ctx context.Context, cc *campaigns.CampaignContext, s *Session) NightDateNote {
	var n NightDateNote
	if !s.hasRealDate() {
		n.NoRealDate = true
		n.Words = h.noRealDateWhen(ctx, s)
	}
	if s.CalendarEventID == nil || h.eventSource == nil {
		return n
	}
	events, err := h.eventSource.SessionEvents(ctx, cc.Campaign.ID, cc.VisibilityRole())
	if err != nil {
		return n
	}
	for i := range events {
		ev := &events[i]
		if ev.ID != *s.CalendarEventID {
			continue
		}
		n.EventName = ev.Name
		// The calendar opens an event on its day from ?event=&date=Y-M-D, on
		// any calendar, fantasy months included.
		n.EventHref = fmt.Sprintf("/campaigns/%s/calendars/%s/view?event=%s&date=%s",
			url.PathEscape(cc.Campaign.ID), url.PathEscape(ev.CalendarID), url.QueryEscape(ev.ID),
			url.QueryEscape(fmt.Sprintf("%d-%d-%d", ev.Year, ev.Month, ev.Day)))
		break
	}
	return n
}
