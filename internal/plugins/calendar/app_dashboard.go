// app_dashboard.go — the Calendar app's route entry point.
//
// WHAT THIS FILE IS NOW. GET /campaigns/:id/apps/calendar renders THE BENCH
// (calendar-v4 wave 1 phase B): the assembly is bench.go's, the markup is
// bench.templ's, and this file holds the thin handler, the ?sort control's
// helpers — which the Bench still uses to order its subordinate rows — and the
// RETAINED card-grid view model (see the retention note further down).
//
// WHAT IT WAS. C-APPS-CAL-DASH-W1's Calendars management dashboard: a card grid
// of the campaign's calendars over a detail pane composing the existing CRUD
// surfaces plus a read-only associations panel, with W2's live "see in action"
// embeds. That surface is what the Bench replaces — the operator's screenshot
// of it is what opened the calendar-v4 arc.
package calendar

import (
	"context"
	"sort"
	"strings"

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
	Calendars  []Calendar     // left list (via ListCalendars)
	Selected   *Calendar      // right detail (eager-loaded via GetCalendarByID)
	ActiveID   string         // the user's active calendar, for the "active" badge
	Entities   []EntityTieRef // associated entities (read-only)
	Timelines  []TimelineRef  // associated timelines (read-only)
	LoadError  bool           // true → friendly "couldn't load" state
	IsOwner    bool
	CSRFToken  string
	// W5c: the active sort key for the card grid (owner control). "" =
	// the default (is_default-first, then sort_order).
	Sort string
	// W5d: per-calendar soonest-upcoming + agenda (batch read), keyed by
	// calendar ID — drives the next-event sort + the adaptive widget's agenda.
	Upcoming map[string]CalendarUpcoming

	// W2 live "see in action" embeds (C-APPS-CAL-DASH-W2):
	// SelectedIsActive gates the LIVE worldstate band — the engine binds the
	// campaign's active calendar (one #cal-v2-worldstate per page), so the
	// live ambient only renders when the selected calendar IS the active one
	// (the dispatch's default nuance; no widget surgery). Non-active calendars
	// still get the engine-free month grid.
	SelectedIsActive bool
	// WorldState holds the active calendar's seed (set only when
	// SelectedIsActive) for the reused worldStateBandV2 component.
	WorldState     *WorldStateSeed
	WorldStateJSON string
}

// AppDashboard renders THE BENCH at GET /campaigns/:id/apps/calendar.
//
// SAME ROUTE, NEW SURFACE (calendar-v4 wave 1 phase B, C-CALV4-BENCH-P4). The
// path is unchanged — the nav target and the Extensions hub both carry it as a
// bare string, and any change would force a routes_snapshot.txt regeneration —
// and the handler stays thin: it resolves the request context and delegates the
// whole assembly to buildBench (bench.go) and the whole markup to bench.templ.
//
// The role branch it used to carry lives in buildBench now, unchanged in
// substance: owners get ListCalendars, players get ListVisibleCalendars, and
// UNIFYING THE TWO REOPENS THE W5a LEAK (dispatch §8).
func (h *Handler) AppDashboard(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil || cc.Campaign == nil {
		return apperror.NewMissingContext()
	}
	data := h.buildBench(c.Request().Context(), benchInput{
		Campaign:  cc.Campaign,
		UserID:    auth.GetUserID(c),
		Role:      cc.VisibilityRole(),
		IsOwner:   cc.MemberRole >= campaigns.RoleOwner,
		CSRFToken: middleware.GetCSRFToken(c),
		Sort:      c.QueryParam("sort"),
		// The day card's two authoring gates, resolved HERE because this is
		// where the request context is (C-CALV4-DAYCARD, R2-2a, [DC-9]).
		// CanCreateEvents mirrors RequireRole(RoleScribe)'s own comparison —
		// cc.MemberRole, not VisibilityRole — and CanAuthorDmOnly is the
		// orthogonal co-DM capability the server already enforces.
		CanCreateEvents: cc.MemberRole >= campaigns.RoleScribe,
		CanAuthorDmOnly: cc.CanAuthorDmOnly(),
		// THE MONTH CURSOR (C-CALV4-GAMEREADY §1, [GR-1]). Two query params on
		// this route and nowhere else: not a preference row, not a session
		// value, not a fifth Bench section. `m` is ONE-BASED and both are
		// independently optional; atoiOr's 0 fallback IS the "unnavigated"
		// value resolveView already answers, so garbage (`?y=nope`) reads as
		// "no cursor" rather than as an error page. No bounds check lives here
		// — resolveView (block_service.go:275) owns the clamp and this slice
		// does not duplicate it.
		View: BlockDate{
			Year:  atoiOr(c.QueryParam("y"), 0),
			Month: atoiOr(c.QueryParam("m"), 0),
		},
	})
	return h.renderAppDashboard(c, cc, data)
}

// renderAppDashboard returns the Bench's row-grid section for the sort
// control's HTMX swap (?sort=…&grid=1 — the ONLY HTMX on this page, and it adds
// no route), or the full page otherwise.
func (h *Handler) renderAppDashboard(c echo.Context, cc *campaigns.CampaignContext, data BenchData) error {
	if middleware.IsHTMX(c) && c.QueryParam("grid") == "1" {
		return middleware.Render(c, 200, benchRowGridSection(data))
	}
	return middleware.Render(c, 200, BenchPage(cc, data))
}

// calendarSortKeys are the supported card-grid sort keys. "" is the default
// (is_default-first, then sort_order); nextevent (W5d) orders by each
// calendar's soonest upcoming event.
var calendarSortKeys = map[string]bool{"": true, "name": true, "created": true, "updated": true, "nextevent": true}

// normalizeCalendarSort clamps an arbitrary ?sort value to a supported key,
// defaulting unknown values to "" (the is_default/sort_order order).
func normalizeCalendarSort(s string) string {
	if calendarSortKeys[s] {
		return s
	}
	return ""
}

// sortDashboardCalendars orders the dashboard cards in place. Default ("") =
// is_default-first, then sort_order; name = A→Z; created/updated = most-recent
// first; nextevent (W5d) = soonest upcoming event first (calendars with none
// last), using the batch upcoming map. Stable so equal keys keep input order.
func sortDashboardCalendars(cals []Calendar, key string, upcoming map[string]CalendarUpcoming) {
	switch key {
	case "name":
		sort.SliceStable(cals, func(i, j int) bool {
			return strings.ToLower(cals[i].Name) < strings.ToLower(cals[j].Name)
		})
	case "created":
		sort.SliceStable(cals, func(i, j int) bool { return cals[i].CreatedAt.After(cals[j].CreatedAt) })
	case "updated":
		sort.SliceStable(cals, func(i, j int) bool { return cals[i].UpdatedAt.After(cals[j].UpdatedAt) })
	case "nextevent":
		sort.SliceStable(cals, func(i, j int) bool {
			di, hi := nextEventApproxDaysUntil(cals[i], upcoming[cals[i].ID])
			dj, hj := nextEventApproxDaysUntil(cals[j], upcoming[cals[j].ID])
			if hi != hj {
				return hi // a calendar WITH an upcoming event sorts before one without
			}
			if !hi {
				return false // both have none → keep input order (stable)
			}
			return di < dj
		})
	default: // is_default first, then sort_order
		sort.SliceStable(cals, func(i, j int) bool {
			if cals[i].IsDefault != cals[j].IsDefault {
				return cals[i].IsDefault // true sorts before false
			}
			return cals[i].SortOrder < cals[j].SortOrder
		})
	}
}

// THE CARD-GRID VIEW IS RETAINED BUT NO LONGER ROUTED.
//
// CalendarAppDashboardData and the components in app_dashboard.templ
// (CalendarAppDashboardPage, the card grid, the detail pane, the "see in
// action" embeds, adaptiveCalendarWidget and calendarPermissionsModal) all stay
// exactly where they are. Two of them are still live: the Bench renders
// calendarAppDashboardEmpty for a campaign with no calendars and
// calendarPermissionsModal for an owner, and C-CALV4-HOST-P3 references
// adaptiveCalendarWidget. The rest is retained deliberately — dead-code removal
// is a post-wave slice (dispatch Bounds), and their tests still exercise them.
//
// The two loader helpers that fed ONLY the retired detail pane
// (loadCalendarEntities / loadCalendarTimelines) were removed with their only
// caller rather than left as unreachable code the `unused` lint would flag on
// every subsequent PR. The reads they wrapped are untouched: the service's
// EntitiesForCalendar keeps its viewer-context seam and its test, and
// SetTimelineLister keeps its composition-root wiring for the next surface that
// wants the cross-plugin timeline read.
