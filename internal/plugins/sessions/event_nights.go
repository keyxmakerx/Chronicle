package sessions

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SessionEventKindSlug is the calendar event kind that marks a session. Kinds
// are the campaign's own list, so this is a convention a campaign follows by
// naming a kind "session"; a campaign without one simply never gets the strip.
const SessionEventKindSlug = "session"

// SessionEvent is a calendar event of the Session kind, as the sessions plugin
// needs it. The calendar's event is a day in the story; a game night is a real
// evening players answer, so the two are separate records and this is only
// what is carried across.
type SessionEvent struct {
	ID         string
	Name       string
	CalendarID string
	Year       int
	Month      int
	Day        int
	// When is the date in the calendar's own words ("14 Hammer, 1492"),
	// worded by the calendar since month names are its.
	When string
	// RealDate is YYYY-MM-DD when the event sits on a real-life calendar, so
	// its date is a real one and can be the night's date. Empty for a story
	// calendar, whose date is not a real day.
	RealDate string
	// EntityID and EntityName are the page the event is linked to, "" if none.
	EntityID   string
	EntityName string
}

// SessionEventSource lists the campaign's Session-kind events. Implemented in
// internal/app over the calendar service, so this plugin never imports the
// calendar plugin. role is the viewer's, so an event they cannot see is never
// offered.
type SessionEventSource interface {
	SessionEvents(ctx context.Context, campaignID string, role int) ([]SessionEvent, error)
}

// SetSessionEventSource wires the lookup behind the Game nights strip.
// Without it the strip is empty and making a night from an event is refused.
func (h *Handler) SetSessionEventSource(s SessionEventSource) {
	h.eventSource = s
}

// normalizeNightName makes two names comparable: lower case, one space
// between words.
func normalizeNightName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// eventHasNight reports whether a game night already stands for the event.
// Nights carry no link to an event (there is no column for one), so the match
// is by what the two share: the same name, or the same day on the same
// calendar, or, on a real-life calendar, the same real date. Cancelled and
// played nights count too: the strip is for events that were never given a
// night, not for ones whose night was dealt with.
func eventHasNight(ev SessionEvent, nights []Session) bool {
	name := normalizeNightName(ev.Name)
	for i := range nights {
		n := &nights[i]
		if name != "" && normalizeNightName(n.Name) == name {
			return true
		}
		if ev.RealDate != "" && n.ScheduledDate != nil && *n.ScheduledDate == ev.RealDate {
			return true
		}
		if n.CalendarID != nil && *n.CalendarID == ev.CalendarID && n.CalendarYear != nil && n.CalendarMonth != nil && n.CalendarDay != nil &&
			*n.CalendarYear == ev.Year && *n.CalendarMonth == ev.Month && *n.CalendarDay == ev.Day {
			return true
		}
	}
	return false
}

// eventsWithoutNight keeps the events no game night stands for, in the order
// given.
func eventsWithoutNight(events []SessionEvent, nights []Session) []SessionEvent {
	out := make([]SessionEvent, 0, len(events))
	for _, ev := range events {
		if !eventHasNight(ev, nights) {
			out = append(out, ev)
		}
	}
	return out
}

// eventNightInput is the night an event becomes: its name, its page, and the
// date the data supports. A story calendar's date is the night's in-world
// date and no real date is invented; a real-life calendar's is the real date.
func eventNightInput(ev SessionEvent, createdBy string) CreateSessionInput {
	in := CreateSessionInput{Name: ev.Name, CreatedBy: createdBy}
	if ev.RealDate != "" {
		d := ev.RealDate
		in.ScheduledDate = &d
		return in
	}
	y, m, d := ev.Year, ev.Month, ev.Day
	in.CalendarYear, in.CalendarMonth, in.CalendarDay = &y, &m, &d
	return in
}

// makeNightFromEvent makes the game night for an event through the same
// service a night made by hand or by an AI goes through: planned, every member
// invited, the event's page linked. It sends no email and no bell: the night
// has no real date yet, and the owner's own press is not an announcement to
// the table. The "no night yet" check is repeated here so a double press or
// two tabs make one night.
func makeNightFromEvent(ctx context.Context, svc SessionService, members campaigns.MemberLister, campaignID, userID string, ev SessionEvent) (*Session, error) {
	existing, err := svc.ListSessions(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if eventHasNight(ev, existing) {
		return nil, apperror.NewConflict("this event already has a game night")
	}
	s, err := svc.CreateSession(ctx, campaignID, eventNightInput(ev, userID))
	if err != nil {
		return nil, err
	}
	if members != nil {
		if list, mErr := members.ListMembers(ctx, campaignID); mErr == nil {
			ids := make([]string, 0, len(list))
			for _, m := range list {
				ids = append(ids, m.UserID)
			}
			_ = svc.InviteAll(ctx, s.ID, ids)
		}
	}
	if ev.EntityID != "" {
		// A page that cannot be linked leaves a night without it; the night
		// itself stands, and the owner can link the page by hand.
		_ = svc.LinkEntity(ctx, s.ID, ev.EntityID, EntityRoleKey, campaignID)
	}
	return s, nil
}

// eventNightRow is one line of the Game nights strip.
type eventNightRow struct {
	EventID string `json:"eventId"`
	Name    string `json:"name"`
	When    string `json:"when"`
	Page    string `json:"page,omitempty"`
}

// EventsWithoutNightAPI lists Session-kind calendar events that have no game
// night, for the strip at the top of the calendar's Game nights panel.
// GET /campaigns/:id/sessions/event-candidates
func (h *Handler) EventsWithoutNightAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	if h.eventSource == nil {
		return c.JSON(http.StatusOK, []eventNightRow{})
	}
	events, err := h.eventSource.SessionEvents(ctx, cc.Campaign.ID, cc.VisibilityRole())
	if err != nil {
		return apperror.NewInternal(err)
	}
	nights, err := h.svc.ListSessions(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	rows := []eventNightRow{}
	for _, ev := range eventsWithoutNight(events, nights) {
		rows = append(rows, eventNightRow{EventID: ev.ID, Name: ev.Name, When: ev.When, Page: ev.EntityName})
	}
	return c.JSON(http.StatusOK, rows)
}

// MakeNightFromEventAPI makes the game night for one Session-kind event. The
// request names only the event: its name, date and page are read again here,
// so a request cannot make a night with words the event does not have.
// POST /campaigns/:id/sessions/from-event  {"eventId": "..."}
func (h *Handler) MakeNightFromEventAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	var body struct {
		EventID string `json:"eventId"`
	}
	if err := c.Bind(&body); err != nil || strings.TrimSpace(body.EventID) == "" {
		return apperror.NewBadRequest("name the event to make a game night for")
	}
	if h.eventSource == nil {
		return apperror.NewInternal(errors.New("sessions: event source not wired"))
	}
	events, err := h.eventSource.SessionEvents(ctx, cc.Campaign.ID, cc.VisibilityRole())
	if err != nil {
		return apperror.NewInternal(err)
	}
	var ev *SessionEvent
	for i := range events {
		if events[i].ID == body.EventID {
			ev = &events[i]
			break
		}
	}
	if ev == nil {
		return apperror.NewNotFound("that event is not a Session event on the calendar")
	}
	s, err := makeNightFromEvent(ctx, h.svc, h.memberLister, cc.Campaign.ID, auth.GetUserID(c), *ev)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, madeNightBody(s))
}

// madeNightBody is the reply to making a night from an event. scheduledDate
// is empty for a story calendar's night, so the calendar can say "no real
// date yet" only when that is true.
func madeNightBody(s *Session) map[string]string {
	body := map[string]string{"id": s.ID, "name": s.Name, "scheduledDate": ""}
	if s.ScheduledDate != nil {
		body["scheduledDate"] = *s.ScheduledDate
	}
	return body
}
