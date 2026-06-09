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
	"encoding/json"
	"log/slog"
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
		Sort:       normalizeCalendarSort(c.QueryParam("sort")),
	}

	// W5c role-branch: owners manage ALL calendars; players see only the
	// calendars visible to them (per-calendar visibility, W5a). Enforced in the
	// service layer (ListVisibleCalendars), not by hiding cards in the view.
	var (
		cals []Calendar
		err  error
	)
	if data.IsOwner {
		cals, err = h.svc.ListCalendars(ctx, cc.Campaign.ID)
	} else {
		cals, err = h.svc.ListVisibleCalendars(ctx, cc.Campaign.ID, role, userID)
	}
	if err != nil {
		slog.Warn("calendars dashboard: list failed",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		data.LoadError = true
		return h.renderAppDashboard(c, cc, data)
	}
	sortDashboardCalendars(cals, data.Sort)
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
		// date label, eras, etc.). Fall back silently if it's gone. W5c: a player
		// must not open a calendar hidden from them via an explicit ?calId, so the
		// detail only loads when the viewer may see it (owner/co-DM always can).
		if sel, serr := h.svc.GetCalendarByID(ctx, selID); serr == nil && sel != nil &&
			sel.CampaignID == cc.Campaign.ID && calendarVisibleTo(sel, role, userID) {
			data.Selected = sel
			data.Entities = h.loadCalendarEntities(ctx, sel.ID)
			data.Timelines = h.loadCalendarTimelines(ctx, sel.ID, role, userID)

			// W2: the LIVE worldstate band renders only for the active
			// calendar (engine binds the active calendar — see the struct
			// doc). Build its seed here so the reused worldStateBandV2 paints
			// on full page load (the engine inits in prod mode from the seed).
			data.SelectedIsActive = data.ActiveID == sel.ID
			if data.SelectedIsActive {
				if seed, berr := h.svc.BuildWorldStateSeed(ctx, sel.ID, sel.CurrentYear, sel.CurrentMonth, sel.CurrentDay, role, userID); berr == nil && seed != nil {
					data.WorldState = seed
					if b, merr := json.Marshal(seed); merr == nil {
						data.WorldStateJSON = string(b)
					}
				}
			}
		}
	}

	return h.renderAppDashboard(c, cc, data)
}

// renderAppDashboard returns a fragment for HTMX swaps (the card grid for a
// sort change `?grid=1`, else the detail pane for a list selection), or the
// full page otherwise.
func (h *Handler) renderAppDashboard(c echo.Context, cc *campaigns.CampaignContext, data CalendarAppDashboardData) error {
	if middleware.IsHTMX(c) {
		// W5c: a sort control swaps just the grid section (it carries ?grid=1).
		if c.QueryParam("grid") == "1" {
			return middleware.Render(c, 200, calendarAppDashboardGridSection(data))
		}
		return middleware.Render(c, 200, calendarAppDashboardDetail(data))
	}
	return middleware.Render(c, 200, CalendarAppDashboardPage(cc, data))
}

// calendarSortKeys are the supported card-grid sort keys (W5c). "" is the
// default (is_default-first, then sort_order). next-event sort is a follow-up
// (it needs a per-calendar "soonest upcoming event" batch read).
var calendarSortKeys = map[string]bool{"": true, "name": true, "created": true, "updated": true}

// normalizeCalendarSort clamps an arbitrary ?sort value to a supported key,
// defaulting unknown values to "" (the is_default/sort_order order).
func normalizeCalendarSort(s string) string {
	if calendarSortKeys[s] {
		return s
	}
	return ""
}

// sortDashboardCalendars orders the dashboard cards in place (W5c). Default
// ("") = is_default-first, then sort_order; name = A→Z; created/updated =
// most-recent first. Stable so equal keys keep their incoming order.
func sortDashboardCalendars(cals []Calendar, key string) {
	switch key {
	case "name":
		sort.SliceStable(cals, func(i, j int) bool {
			return strings.ToLower(cals[i].Name) < strings.ToLower(cals[j].Name)
		})
	case "created":
		sort.SliceStable(cals, func(i, j int) bool { return cals[i].CreatedAt.After(cals[j].CreatedAt) })
	case "updated":
		sort.SliceStable(cals, func(i, j int) bool { return cals[i].UpdatedAt.After(cals[j].UpdatedAt) })
	default: // is_default first, then sort_order
		sort.SliceStable(cals, func(i, j int) bool {
			if cals[i].IsDefault != cals[j].IsDefault {
				return cals[i].IsDefault // true sorts before false
			}
			return cals[i].SortOrder < cals[j].SortOrder
		})
	}
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
