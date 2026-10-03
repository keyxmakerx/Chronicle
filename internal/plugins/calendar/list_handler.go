// Package calendar — list_handler.go serves the campaign calendars list
// page. Thin handlers only: bind, call the service, render — see
// CLAUDE.md's layering rule.
package calendar

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CalendarsListData is the view model for the calendars list page.
type CalendarsListData struct {
	CampaignID   string
	CampaignName string
	Calendars    []Calendar
	// MonthEvents is each calendar's current-month events, keyed by
	// calendar id, for the month its card peeks on hover. A calendar whose
	// read failed is absent and peeks with no marks.
	MonthEvents map[string][]Event
	IsOwner     bool
	CSRFToken   string
	// JustCreated/NewCalendarID announce the calendar the wizard just made
	// (its redirect carries ?new=<id>). Both are "" unless that id is one of
	// the calendars listed here; the name is the stored one, never taken from
	// the URL, so a crafted link can't make the page announce arbitrary text.
	JustCreated   string
	NewCalendarID string
}

// Index renders the campaign's calendar list (GET /campaigns/:id/calendars).
// Replaces the "calendar is being rebuilt" placeholder: app/routes.go no
// longer registers a handler for this exact path (see its own comment on
// the removed line).
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	v := viewerFrom(c, cc)

	cals, err := h.svc.ListCalendars(ctx, cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	// Each card shows the calendar's named current date and main moon, which
	// ListCalendars deliberately does not eager-load (its own doc comment:
	// list reads stay cheap). A campaign's calendar count is small — this
	// page exists to pick ONE calendar to work with — so an extra
	// GetCalendarForViewer per card is an acceptable trade against adding a
	// batch-load path only this page would ever use.
	// The same goes for each card's peek, which needs that calendar's
	// current month of events.
	full := make([]Calendar, 0, len(cals))
	monthEvents := make(map[string][]Event, len(cals))
	for _, summary := range cals {
		detailed, err := h.svc.GetCalendarForViewer(ctx, summary.ID, cc.Campaign.ID, v)
		if err != nil {
			// Still don't fail the whole page over one calendar's error —
			// but a genuine DB error here would otherwise vanish a
			// calendar from the list with no trace at all, indistinguishable
			// from "it was deleted between the list and detail reads".
			slog.Error("calendar: failed to load calendar for viewer while building the list page",
				slog.String("calendar_id", summary.ID),
				slog.String("campaign_id", cc.Campaign.ID),
				slog.Any("error", err))
			continue
		}
		full = append(full, *detailed)
		events, err := h.svc.ListEventsForMonth(ctx, detailed.ID, cc.Campaign.ID, detailed.CurrentYear, detailed.CurrentMonth, v)
		if err != nil {
			// The peek is a glance; it shows the month without its marks
			// rather than taking the page down.
			slog.Error("calendar: failed to load a card's month of events for its peek",
				slog.String("calendar_id", detailed.ID),
				slog.String("campaign_id", cc.Campaign.ID),
				slog.Any("error", err))
			continue
		}
		monthEvents[detailed.ID] = events
	}

	data := CalendarsListData{
		CampaignID:   cc.Campaign.ID,
		CampaignName: cc.Campaign.Name,
		Calendars:    full,
		MonthEvents:  monthEvents,
		IsOwner:      cc.MemberRole >= campaigns.RoleOwner,
		CSRFToken:    middleware.GetCSRFToken(c),
	}
	if newID := c.QueryParam("new"); newID != "" {
		for _, cal := range full {
			if cal.ID == newID {
				data.JustCreated, data.NewCalendarID = cal.Name, cal.ID
				break
			}
		}
	}
	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CalendarsListFragment(data))
	}
	return middleware.Render(c, http.StatusOK, CalendarsListPage(data))
}
