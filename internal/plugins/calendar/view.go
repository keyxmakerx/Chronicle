// view.go — the calendar's own page (V5 part A, #741): GET
// /campaigns/:id/calendars/:calid/view. The handler is thin (bind, call
// service, render); the month grid, day/era cards, event details and moons
// are all rendered client-side by the calendar_view widget from the JSON
// this handler ferries down on first paint (see view.templ's config div).
package calendar

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CalendarViewData is everything CalendarViewPage (the templ component)
// needs to render the page shell and seed the widget's first paint.
type CalendarViewData struct {
	CampaignID string
	CalendarID string
	Calendar   *Calendar
	// CurrentMonthEvents is ListEventsForMonth for the calendar's own
	// current (year, month) — the month the grid opens on — so first paint
	// needs no extra fetch. Later months are fetched by the widget itself.
	CurrentMonthEvents []Event
	// CanEdit gates the whole owner-editing surface (edit mode, the
	// structure/moon drawer, the bulk bar, new-kind-in-place): Owner or a
	// granted co-Director, per campaigns.CampaignContext.CanAuthorDmOnly's
	// doc comment. It does NOT mean every write it exposes will succeed —
	// deleting an event (DELETE .../events/:eid) is gated strictly RoleOwner
	// server-side (routes.go), so a co-Director sees that one affordance
	// 403; the widget checks ViewerRole for it. Event kinds, eras and the
	// moon hidden flag are CanAuthorDmOnly like everything else CanEdit
	// covers, so those affordances work for a co-Director too.
	CanEdit bool
	// CanAuthorDmOnly threads campaigns.CanAuthorDmOnly() through separately
	// from CanEdit (which already implies it) so the widget can tell "may
	// edit at all" from "may also toggle event visibility" if the two ever
	// diverge — they don't today (CanEdit's own definition is this OR
	// RoleOwner), but a future role between the two would need this.
	CanAuthorDmOnly bool
	// ViewerRole is cc.MemberRole as a plain int (campaigns.RoleOwner == 3,
	// RoleScribe == 2, RolePlayer == 1, RoleNone == 0) — ferried to the
	// widget so it can gate the one remaining Owner-only slice of the
	// editing surface (deleting an event) separately from CanEdit's broader
	// co-Director allowance.
	ViewerRole int
}

// CalendarViewPage handles GET /campaigns/:id/calendars/:calid/view.
// Not a business-logic decision beyond what CanEdit computes from the
// already-resolved CampaignContext: bind, call the service twice (the
// calendar, then its current month's events), render. A calendar the viewer
// may not see, or that does not belong to this campaign, comes back as
// whatever error GetCalendarForViewer already returns (NotFound in both
// cases, per its own doc comment) — propagated as-is, no new handling here.
func (h *Handler) CalendarViewPage(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	calID := c.Param("calid")
	v := viewerFrom(c, cc)

	cal, err := h.svc.GetCalendarForViewer(ctx, calID, cc.Campaign.ID, v)
	if err != nil {
		return err
	}

	events, err := h.svc.ListEventsForMonth(ctx, calID, cc.Campaign.ID, cal.CurrentYear, cal.CurrentMonth, v)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, CalendarViewPage(cc, calendarViewDataFor(cc, cal, events)))
}

// calendarViewDataFor is the widget's config for one viewer. The calendar's
// own page and the Calendars page's preview both build it here, so the
// calendar a preview unfolds into is the one its own page would show.
func calendarViewDataFor(cc *campaigns.CampaignContext, cal *Calendar, events []Event) CalendarViewData {
	if events == nil {
		events = []Event{}
	}
	return CalendarViewData{
		CampaignID:         cc.Campaign.ID,
		CalendarID:         cal.ID,
		Calendar:           cal,
		CurrentMonthEvents: events,
		CanEdit:            cc.MemberRole >= campaigns.RoleOwner || cc.CanAuthorDmOnly(),
		CanAuthorDmOnly:    cc.CanAuthorDmOnly(),
		ViewerRole:         int(cc.MemberRole),
	}
}
