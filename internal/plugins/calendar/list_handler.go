// Package calendar — list_handler.go serves the campaign calendars list
// page and the per-card preview fragment. Thin handlers only:
// bind, call the service, render — see CLAUDE.md's layering rule.
package calendar

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// upcomingEventsLimit caps the preview dialog's "Coming up" list.
const upcomingEventsLimit = 5

// CalendarsListData is the view model for the calendars list page.
type CalendarsListData struct {
	CampaignID   string
	CampaignName string
	Calendars    []Calendar
	IsOwner      bool
	CSRFToken    string
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
	full := make([]Calendar, 0, len(cals))
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
	}

	data := CalendarsListData{
		CampaignID:   cc.Campaign.ID,
		CampaignName: cc.Campaign.Name,
		Calendars:    full,
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

// CalendarPreviewData is the view model for the per-card preview dialog.
type CalendarPreviewData struct {
	CampaignID string
	Calendar   *Calendar
	Upcoming   []Event
	// MonthEvents is the current month's events (for the grid's event-count
	// dots) — cheap to include since ListEventsForMonth already exists and
	// this dialog only ever shows the current month (see calendar_preview.templ).
	MonthEvents []Event
}

// Preview renders the calendar preview fragment
// (GET /campaigns/:id/calendars/:calid/preview), opened from a card into the
// list page's preview dialog. Fragment-only: there is no standalone page
// for it; its "Full calendar view" link opens the calendar's own page
// (/calendars/:calid/view).
func (h *Handler) Preview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	v := viewerFrom(c, cc)

	cal, err := h.svc.GetCalendarForViewer(ctx, c.Param("calid"), cc.Campaign.ID, v)
	if err != nil {
		return err
	}
	upcoming, err := h.svc.ListUpcomingEvents(ctx, cal.ID, cc.Campaign.ID, upcomingEventsLimit, v)
	if err != nil {
		return err
	}
	monthEvents, err := h.svc.ListEventsForMonth(ctx, cal.ID, cc.Campaign.ID, cal.CurrentYear, cal.CurrentMonth, v)
	if err != nil {
		return err
	}

	data := CalendarPreviewData{
		CampaignID:  cc.Campaign.ID,
		Calendar:    cal,
		Upcoming:    upcoming,
		MonthEvents: monthEvents,
	}
	return middleware.Render(c, http.StatusOK, CalendarPreviewFragment(data))
}
