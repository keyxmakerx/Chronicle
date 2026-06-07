// app_dashboard.go — the Calendars management dashboard (C-APPS-CAL-DASH-W1).
//
// A dedicated page reached from the Extensions hub's per-app "Open dashboard"
// entry for the calendar app. List + detail-pane layout: the campaign's
// calendars on the left, the selected calendar's CRUD-compose actions +
// a READ-ONLY associations panel (associated entities + timelines) on the
// right. Wave 1 COMPOSES the existing calendar CRUD surfaces (settings, setup
// wizard, delete, active-switch) — it does not reimplement any of them — and
// reads associations via two queries added this wave (EntitiesForCalendar +
// the cross-plugin TimelineLister). Live "see in action" embeds are Wave 2;
// inline link/unlink + create-timeline are Wave 3.
package calendar

import (
	"context"
	"log/slog"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TimelineRef is the calendar plugin's view of a timeline for the dashboard's
// associations panel. Kept deliberately minimal (display + count + a link
// target) so the cross-plugin boundary stays narrow. Populated by the
// app-wired adapter over the timeline service — the calendar plugin never
// imports the timeline repo (plugin-isolation convention).
type TimelineRef struct {
	ID         string
	Name       string
	Color      string
	Icon       string
	Visibility string
	EventCount int
}

// TimelineLister is the cross-plugin read the Calendars dashboard needs:
// "which timelines are bound to this calendar". Implemented by an adapter in
// internal/app/routes.go that delegates to the timeline service (mirrors the
// timeline plugin's own calendarLister adapters, in the reverse direction).
type TimelineLister interface {
	ListTimelinesForCalendar(ctx context.Context, calendarID string, role int, userID string) ([]TimelineRef, error)
}

// SetTimelineLister injects the cross-plugin timeline read (optional — the
// dashboard degrades to an empty timelines panel if it's absent).
func (h *Handler) SetTimelineLister(l TimelineLister) { h.timelineLister = l }

// CalendarAppDashboardData is the projection the dashboard templ renders.
type CalendarAppDashboardData struct {
	CampaignID string
	Calendars  []Calendar    // left list (via ListCalendars)
	Selected   *Calendar     // right detail (eager-loaded via GetCalendarByID)
	ActiveID   string        // the user's active calendar, for the "active" badge
	Entities   []EntityTieRef // associated entities (read-only)
	Timelines  []TimelineRef  // associated timelines (read-only)
	LoadError  bool           // true → friendly "couldn't load" state
	IsOwner    bool
	CSRFToken  string
}

// AppDashboard renders the Calendars dashboard. Full page on a normal GET;
// just the detail pane on an HTMX request (selecting a calendar in the list
// swaps #cal-dash-detail). Read-only/compose only this wave.
func (h *Handler) AppDashboard(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	role := cc.VisibilityRole()

	data := CalendarAppDashboardData{
		CampaignID: cc.Campaign.ID,
		IsOwner:    cc.MemberRole >= campaigns.RoleOwner,
		CSRFToken:  middleware.GetCSRFToken(c),
	}

	cals, err := h.svc.ListCalendars(ctx, cc.Campaign.ID)
	if err != nil {
		slog.Warn("calendars dashboard: list failed",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		data.LoadError = true
		return h.renderAppDashboard(c, cc, data)
	}
	data.Calendars = cals

	// The user's active calendar drives the "active" badge + the default
	// selection when the request doesn't name one.
	if active, aerr := h.svc.GetActiveCalendar(ctx, userID, cc.Campaign.ID); aerr == nil && active != nil {
		data.ActiveID = active.ID
	}

	// Resolve the selected calendar: explicit ?calId, else active, else first.
	selID := c.QueryParam("calId")
	if selID == "" {
		if data.ActiveID != "" {
			selID = data.ActiveID
		} else if len(cals) > 0 {
			selID = cals[0].ID
		}
	}
	if selID != "" {
		// Eager-load the full calendar for the detail pane (months for the
		// date label, eras, etc.). Fall back silently if it's gone.
		if sel, serr := h.svc.GetCalendarByID(ctx, selID); serr == nil && sel != nil && sel.CampaignID == cc.Campaign.ID {
			data.Selected = sel
			data.Entities = h.loadCalendarEntities(ctx, sel.ID)
			data.Timelines = h.loadCalendarTimelines(ctx, sel.ID, role, userID)
		}
	}

	return h.renderAppDashboard(c, cc, data)
}

// renderAppDashboard returns the detail fragment for HTMX selection swaps, or
// the full page otherwise.
func (h *Handler) renderAppDashboard(c echo.Context, cc *campaigns.CampaignContext, data CalendarAppDashboardData) error {
	if middleware.IsHTMX(c) {
		return middleware.Render(c, 200, calendarAppDashboardDetail(data))
	}
	return middleware.Render(c, 200, CalendarAppDashboardPage(cc, data))
}

// loadCalendarEntities reads the associated entities, logging+degrading on
// error so a stale read can't 500 the dashboard.
func (h *Handler) loadCalendarEntities(ctx context.Context, calendarID string) []EntityTieRef {
	ents, err := h.svc.EntitiesForCalendar(ctx, calendarID)
	if err != nil {
		slog.Warn("calendars dashboard: entities read failed",
			slog.String("calendar_id", calendarID), slog.Any("error", err))
		return nil
	}
	return ents
}

// loadCalendarTimelines reads the associated timelines via the cross-plugin
// lister (absent → empty panel).
func (h *Handler) loadCalendarTimelines(ctx context.Context, calendarID string, role int, userID string) []TimelineRef {
	if h.timelineLister == nil {
		return nil
	}
	tls, err := h.timelineLister.ListTimelinesForCalendar(ctx, calendarID, role, userID)
	if err != nil {
		slog.Warn("calendars dashboard: timelines read failed",
			slog.String("calendar_id", calendarID), slog.Any("error", err))
		return nil
	}
	return tls
}
