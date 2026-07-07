// handler_v2.go — V2 calendar shell HTTP handlers (Wave 1 PR 1 /
// C-CAL-V2-SHELL-FOUNDATION). Lives alongside the V1 Handler methods
// in handler.go so the V1 surface continues to serve existing
// /campaigns/:id/calendars/... routes during the migration. V2 routes
// nest under /campaigns/:id/calendar/v2/...; cutover happens when
// feature parity lands in later Wave 1 PRs.

package calendar

import (
	"context"
	"encoding/json"
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
	// validate `:calId` belongs to the campaign. W5a: filtered to the
	// calendars this viewer may see (owners/co-DM get all), so the switcher
	// never lists a calendar hidden from a player.
	allCalendars, err := h.svc.ListVisibleCalendars(ctx, cc.Campaign.ID, cc.VisibilityRole(), userID)
	if err != nil {
		return err
	}

	// Resolve the active calendar:
	//   - explicit :calId wins (after IDOR check)
	//   - otherwise fall through to service-side GetActiveCalendar
	var active *Calendar
	if calID != "" {
		// W5a: requireVisibleCalendar 404s a calendar hidden from this viewer
		// (don't leak existence), so a player can't open one by guessing its ID.
		cal, err := h.requireVisibleCalendar(c, calID, cc.Campaign.ID)
		if err != nil {
			return err
		}
		active = cal
	} else {
		// W5a: resolve to a calendar the viewer may see — if their active/default
		// is hidden from them, fall back to their first visible one.
		active, err = h.svc.GetActiveVisibleCalendar(ctx, cc.Campaign.ID, cc.VisibilityRole(), userID)
		if err != nil {
			return err
		}
		// W1 (R3): GetActiveCalendar returns a SHALLOW row (no Months/Weekdays).
		// The grid + week/day math need them eager-loaded — without this the
		// no-:calId Week view indexes empty cal.Months and the Month grid renders
		// against an empty structure. Re-fetch eagerly, matching the explicit
		// :calId path (requireCalendarInCampaign → GetCalendarByID → eagerLoad).
		// Best-effort: keep the shallow row on a re-fetch miss (addDaysSimple is
		// guarded regardless).
		if active != nil {
			if full, ferr := h.svc.GetCalendarByID(ctx, active.ID); ferr == nil && full != nil {
				active = full
			}
		}
	}

	// Sidebar pin preference (Wave 1.7A §G). Default TRUE; nil-safe.
	sidebarPinned := true
	if pinned, perr := h.svc.GetSidebarPinned(ctx, userID, cc.Campaign.ID); perr == nil {
		sidebarPinned = pinned
	}

	data := CalendarV2ViewData{
		ActiveCalendar:       active,
		AllCalendars:         allCalendars,
		View:                 view,
		CampaignID:           cc.Campaign.ID,
		UserID:               userID,
		IsOwner:              cc.MemberRole >= campaigns.RoleOwner,
		IsScribe:             cc.MemberRole >= campaigns.RoleScribe,
		CanControlWorldState: cc.CanControlWorldState(),
		CanAuthorDmOnly:      cc.CanAuthorDmOnly(),
		CSRFToken:            middleware.GetCSRFToken(c),
		TierDefinitions:      h.loadTierDefinitions(ctx, cc.Campaign.ID),
		SidebarPinned:        sidebarPinned,
	}

	// Entity types for the drawer's "Create entity from event" action
	// (C-CAL-EDITOR-EXPANSION PR1). Only Scribes get the drawer, and the seam is
	// optional — best-effort, an empty list just hides the action.
	if data.IsScribe && h.entityCreator != nil {
		data.EntityTypes, _ = h.entityCreator.ListEntityTypes(ctx, cc.Campaign.ID)
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
		case "ledger":
			// The Ledger loads a ONE-YEAR window: the full displayed year
			// (Jan 1 → last day of the last month). ListEventsForDateRange
			// (NOT ListEventsForYear) is used deliberately — it projects
			// recurrence via Event.OccursOn across the window, per the
			// C-CAL-TIMELINE-V2-W1 data-layer ruling. filterEventsByUser runs
			// inside the service, so dm_only rows are absent for players.
			lastMonth, lastDay := ledgerYearWindowEnd(active, data.Year)
			if events, err := h.svc.ListEventsForDateRange(ctx, active.ID, data.Year, 1, 1, lastMonth, lastDay, role, userID); err == nil {
				data.Events = events
			}
		default:
			if events, err := h.svc.ListEventsForMonth(ctx, active.ID, data.Year, data.Month, role, userID); err == nil {
				data.Events = events
			}
		}

		// Live ambient worldState (C-CAL-WORLDSTATE-PRODUCTION-PORT, 2a).
		// Build the CATALOG Part-8 seed for the cursor date (dm_only
		// celestial events filtered by role) and stash both the struct (for
		// server-side container rendering) and its JSON (for the engine).
		// Best-effort: a seed failure must not break the calendar grid.
		if seed, serr := h.svc.BuildWorldStateSeed(ctx, active.ID, data.Year, data.Month, data.Day, role, userID); serr == nil {
			data.WorldState = seed
			if raw, jerr := json.Marshal(seed); jerr == nil {
				data.WorldStateJSON = string(raw)
			}
		} else {
			slog.Warn("build worldstate seed failed; calendar_v2 renders without ambient layer",
				slog.String("calendar_id", active.ID), slog.Any("error", serr))
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
// --- V1 → V2 cutover redirects (C-CAL-V1-V2-CUTOVER) ----------------------
//
// The V1 calendar VIEW routes (Show / week / day, and the bare /calendar) 301
// to their V2 equivalents so every old link + bookmark lands on V2. The V1
// view handlers/templates are left intact (unrouted) for a follow-on retire —
// the calendar.templ view tree is large + entangled with the PRESERVED setup /
// timeline / upcoming surfaces, so deleting it is its own pass. The shared API
// + data routes (events / settings / world-state / export) are unchanged — V2
// uses them. The timeline + standalone embed have NO V2 equivalent yet and are
// preserved (see routes.go).

// v2CalendarRedirect 301s a retired V1 calendar view to V2, preserving :calId
// and the view segment (month is the V2 default, so "" → /calendar/v2/:calId).
func (h *Handler) v2CalendarRedirect(c echo.Context, view string) error {
	cc := campaigns.GetCampaignContext(c)
	target := "/campaigns/" + cc.Campaign.ID + "/calendar/v2"
	if calID := c.Param("calId"); calID != "" {
		target += "/" + calID
		if view != "" {
			target += "/" + view
		}
	}
	return c.Redirect(http.StatusMovedPermanently, target)
}

// RedirectShowV2 / RedirectWeekV2 / RedirectDayV2 are the route targets for the
// retired V1 view routes.
func (h *Handler) RedirectShowV2(c echo.Context) error { return h.v2CalendarRedirect(c, "") }
func (h *Handler) RedirectWeekV2(c echo.Context) error { return h.v2CalendarRedirect(c, "week") }
func (h *Handler) RedirectDayV2(c echo.Context) error  { return h.v2CalendarRedirect(c, "day") }

func addDaysSimple(cal *Calendar, month, day, n int) (int, int) {
	// W1 (R3 crash-guard): a calendar resolved without eager-loaded
	// sub-resources (the no-:calId active path) has zero Months, and the
	// trailing `cal.Months[month-1]` would then index cal.Months[-1] → panic.
	// Bail to the input date; the eager-load in ShowV2 makes this unreachable
	// on real data, but the guard stands on its own.
	if cal == nil || len(cal.Months) == 0 {
		return month, day
	}
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
