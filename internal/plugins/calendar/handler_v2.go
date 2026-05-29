// handler_v2.go — V2 calendar shell HTTP handlers (Wave 1 PR 1 /
// C-CAL-V2-SHELL-FOUNDATION). Lives alongside the V1 Handler methods
// in handler.go so the V1 surface continues to serve existing
// /campaigns/:id/calendars/... routes during the migration. V2 routes
// nest under /campaigns/:id/calendar/v2/...; cutover happens when
// feature parity lands in later Wave 1 PRs.

package calendar

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ShowV2 is the entry handler for /campaigns/:id/calendar/v2[/:calId]
// [/:view]. Resolves the active calendar, loads the campaign's calendar
// list (for the switcher dropdown), and renders the shell with the
// requested view's placeholder.
//
// URL forms accepted (in priority order):
//   - /campaigns/:id/calendar/v2/:calId/:view   — explicit cal + view
//   - /campaigns/:id/calendar/v2/:calId          — explicit cal, default view = month
//   - /campaigns/:id/calendar/v2                 — active calendar, default view = month
//
// Active-calendar resolution comes from the service layer; see
// CalendarService.GetActiveCalendar for the lookup-and-fallback chain.
func (h *Handler) ShowV2(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	calID := c.Param("calId")
	view := c.Param("view")
	if view == "" {
		view = "month"
	}

	// Load the calendar list once — populates the switcher and lets us
	// validate `:calId` belongs to the campaign.
	allCalendars, err := h.svc.ListCalendars(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}

	// Resolve the active calendar:
	//   - explicit :calId wins (after IDOR check)
	//   - otherwise fall through to service-side GetActiveCalendar
	var active *Calendar
	if calID != "" {
		cal, err := h.requireCalendarInCampaign(c, calID, cc.Campaign.ID)
		if err != nil {
			return err
		}
		active = cal
	} else {
		active, err = h.svc.GetActiveCalendar(ctx, userID, cc.Campaign.ID)
		if err != nil {
			return err
		}
	}

	// Sidebar pin preference (Wave 1.7A §G). Default TRUE; nil-safe.
	sidebarPinned := true
	if pinned, perr := h.svc.GetSidebarPinned(ctx, userID, cc.Campaign.ID); perr == nil {
		sidebarPinned = pinned
	}

	data := CalendarV2ViewData{
		ActiveCalendar:  active,
		AllCalendars:    allCalendars,
		View:            view,
		CampaignID:      cc.Campaign.ID,
		UserID:          userID,
		IsOwner:         cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:        cc.MemberRole >= campaigns.RoleScribe,
		CSRFToken:       middleware.GetCSRFToken(c),
		TierDefinitions: h.loadTierDefinitions(ctx, cc.Campaign.ID),
		SidebarPinned:   sidebarPinned,
	}

	// Cursor (year/month/day) — fall back to the calendar's stored
	// in-world clock when the URL omits them. Zero-calendar campaigns
	// skip cursor population since there's no calendar to anchor to.
	if active != nil {
		data.Year = active.CurrentYear
		data.Month = active.CurrentMonth
		data.Day = active.CurrentDay
		data.TodayYear = active.CurrentYear
		data.TodayMonth = active.CurrentMonth
		data.TodayDay = active.CurrentDay
		if q := c.QueryParam("year"); q != "" {
			if v, err := strconv.Atoi(q); err == nil {
				data.Year = v
			}
		}
		if q := c.QueryParam("month"); q != "" {
			if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= len(active.Months) {
				data.Month = v
			}
		}
		if q := c.QueryParam("day"); q != "" {
			if v, err := strconv.Atoi(q); err == nil && v >= 1 {
				data.Day = v
			}
		}
	}

	// Load events for the visible window (Wave 1 PR 4 — Month/Week/Day
	// views render real events via the calendar_v2 widget layer).
	// Zero-calendar campaigns skip event load (no calendar to scope by).
	if active != nil {
		role := cc.VisibilityRole()
		switch view {
		case "week", "day":
			// Week + Day load a date-range; Day = single day, Week =
			// 7 days centered on the cursor. The handler defers to
			// service which already normalizes the end-of-range.
			startMonth, startDay := data.Month, data.Day
			endMonth, endDay := data.Month, data.Day
			if view == "week" {
				// Compute the 7-day window: start-3, end+3 (simple
				// "near the cursor" baseline; PR 5 refines).
				endMonth, endDay = addDaysSimple(active, startMonth, startDay, 6)
			}
			if events, err := h.svc.ListEventsForDateRange(ctx, active.ID, data.Year, startMonth, startDay, endMonth, endDay, role, userID); err == nil {
				data.Events = events
			}
		default:
			if events, err := h.svc.ListEventsForMonth(ctx, active.ID, data.Year, data.Month, role, userID); err == nil {
				data.Events = events
			}
		}
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, CalendarV2ViewFragment(cc, data))
	}
	return middleware.Render(c, http.StatusOK, CalendarV2Page(cc, data))
}

// addDaysSimple steps (startMonth, startDay) forward by `n` days
// using the calendar's per-month day count. Wraps into the next
// month when the day exceeds month length. Stops at year-end without
// rolling over (PR 4 keeps Week-view in a single calendar year; PR 5
// can refine for year-boundary spans).
func addDaysSimple(cal *Calendar, month, day, n int) (int, int) {
	for n > 0 && month <= len(cal.Months) {
		remaining := cal.Months[month-1].Days - day
		if n <= remaining {
			day += n
			break
		}
		n -= remaining + 1
		day = 1
		month++
	}
	if month > len(cal.Months) {
		month = len(cal.Months)
		day = cal.Months[month-1].Days
	}
	return month, day
}

// SidebarPinAPI toggles the per-user sidebar pin preference.
// POST /campaigns/:id/calendar/v2/sidebar-pin  body: {"pinned": true|false}
//
// Wave 1.7A §G. Returns 200 with the persisted pin state. UI uses
// the response to confirm the toggle landed; on error, the toggle
// reverts client-side.
func (h *Handler) SidebarPinAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if err := h.svc.SetSidebarPinned(ctx, userID, cc.Campaign.ID, req.Pinned); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": "ok",
		"pinned": req.Pinned,
	})
}

// SwitchActiveCalendarAPI persists the user's calendar choice.
// POST /campaigns/:id/calendar/v2/switch  body: {"calendar_id": "..."}
//
// Returns 200 with the new active-calendar id. The client (Alpine
// dropdown in the shell) reloads after success so the shell re-renders
// against the new active calendar. Audit emission is service-side via
// SwitchActiveCalendar (no write to calendar_active is audited today;
// dispatch §"Failure handling" pattern applies if extended later).
func (h *Handler) SwitchActiveCalendarAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	if userID == "" {
		return apperror.NewUnauthorized("authentication required")
	}

	var req struct {
		CalendarID string `json:"calendar_id" form:"calendar_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	if req.CalendarID == "" {
		return apperror.NewBadRequest("calendar_id is required")
	}

	if err := h.svc.SwitchActiveCalendar(ctx, userID, cc.Campaign.ID, req.CalendarID); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{
		"status":      "ok",
		"calendar_id": req.CalendarID,
	})
}

// loadTierDefinitions fetches campaign-aware tier vocabulary for V2
// rendering. Wave 1.6.5: activates PR #370 Phase 2 overlay
// end-to-end. Returns nil when:
//   - tierLister is not wired (early init order; safe fall-back)
//   - lookup errors (slog.Warn + nil; widget falls back to platform default)
//   - campaign has no event_tier_definitions configured (empty slice
//     surfaces as nil per Go convention; same fall-back)
//
// Per dispatch §"Error handling" — every failure mode degrades
// gracefully to platform-default tier rendering. No operator-visible
// crash; lookup failures show up only in server logs.
func (h *Handler) loadTierDefinitions(ctx context.Context, campaignID string) []TierDefinitionAlias {
	if h.tierLister == nil {
		return nil
	}
	defs, err := h.tierLister.GetEventTierDefinitions(ctx, campaignID)
	if err != nil {
		slog.Warn("load tier definitions failed; falling back to platform defaults",
			slog.String("campaign_id", campaignID),
			slog.Any("error", err),
		)
		return nil
	}
	if len(defs) == 0 {
		return nil
	}
	out := make([]TierDefinitionAlias, len(defs))
	for i, d := range defs {
		out[i] = TierDefinitionAlias{
			Slug:       d.Slug,
			Name:       d.Name,
			Color:      d.Color,
			Prominence: d.Prominence,
		}
	}
	return out
}
