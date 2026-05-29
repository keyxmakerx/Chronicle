// extension_dashboard.go — C-EXT-HUB Phase 2 calendar plugin
// dashboard registration.
//
// Exposes ExtensionDashboardFactory() returning a closure that the
// campaigns plugin's RegisterExtensionDashboard collects at startup
// wiring (mirrors ai_workspace.SettingsTabFactory at
// internal/app/routes.go:2465). The factory is invoked per request
// with the live CampaignContext, computes the dashboard's view-data
// against the calendar service, and returns an ExtensionDashboard
// whose Content is the V2-styled calendar_extension_dashboard.templ.
//
// Operator's first V2-styling encounter lands inside this dashboard
// fragment (per the audit + the Phase 2 dispatch), so the templ
// uses the same calendar_v2 token/animation vocabulary as the V2
// calendar surface itself.

package calendar

import (
	"context"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// calendarExtensionDashboardData is the projection the dashboard
// templ renders against. Computed eagerly inside the factory closure
// so the templ render is pure (matches the rest of the calendar
// plugin's handler→templ contract).
type calendarExtensionDashboardData struct {
	CampaignID      string
	Active          *Calendar
	HasActive       bool
	Upcoming        []Event
	TierDefinitions []TierDefinitionAlias
}

// ExtensionDashboardFactory returns the factory the campaigns
// plugin registers via RegisterExtensionDashboard. The closure
// captures the calendar Handler (for service access via
// h.svc / h.tierLister) and resolves all per-request data inside
// the campaigns invocation.
//
// Failure handling: every data-load is guarded by a nil-or-error
// fall-through (the V2 calendar handler's own pattern, audit §1.4
// "panic-safe on direct hit"). A campaign with no active calendar
// renders the explicit empty-state branch in the templ; transient
// query errors render the empty-state too so a stale read can't
// 500 the hub. Errors are logged for operators.
func (h *Handler) ExtensionDashboardFactory() func(*campaigns.CampaignContext) campaigns.ExtensionDashboard {
	return func(cc *campaigns.CampaignContext) campaigns.ExtensionDashboard {
		data := buildCalendarExtensionDashboardData(context.Background(), h, cc)
		return campaigns.ExtensionDashboard{
			Slug:    "calendar",
			Content: calendarExtensionDashboard(data),
		}
	}
}

// buildCalendarExtensionDashboardData resolves the per-request data
// for the calendar dashboard card. Split out from the factory
// closure so the test suite can exercise the projection
// independently of the templ render.
//
// The campaigns plugin passes a CampaignContext without a request-
// scoped context.Context (BuildExtensionDashboards invokes factories
// with only the CC), so the data load uses context.Background() with
// the calendar service. The service's role-based event filter is
// driven from cc.VisibilityRole, which is the same pattern
// handler_v2.go's ShowV2 uses (line ~120) so the dashboard's
// visibility matches the V2 calendar page exactly.
func buildCalendarExtensionDashboardData(ctx context.Context, h *Handler, cc *campaigns.CampaignContext) calendarExtensionDashboardData {
	out := calendarExtensionDashboardData{CampaignID: cc.Campaign.ID}

	if h == nil || h.svc == nil {
		return out
	}

	// Active calendar: same resolution as handler_v2.go ShowV2 —
	// per-user pointer with campaign fallback. nil means
	// zero-calendar campaign; explicit empty-state branches.
	userID := ""
	// CampaignContext has no UserID field on its surface; the
	// per-user active-calendar pointer falls back to the campaign's
	// default when userID is empty (matches PR #363's
	// GetActiveCalendar contract).
	active, err := h.svc.GetActiveCalendar(ctx, userID, cc.Campaign.ID)
	if err != nil {
		slog.Warn("calendar extension dashboard: active-calendar lookup failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err),
		)
		return out
	}
	out.Active = active
	out.HasActive = active != nil

	if active == nil {
		return out
	}

	// Upcoming events — service already does role-aware filter +
	// visibility evaluation. Limit kept low (5) to keep this card
	// summary-y; the V2 calendar page is one click away via the
	// "Open V2 Calendar" CTA.
	upcoming, err := h.svc.ListUpcomingEvents(ctx, active.ID, 5, cc.VisibilityRole(), userID)
	if err != nil {
		slog.Warn("calendar extension dashboard: upcoming events query failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.String("calendar_id", active.ID),
			slog.Any("error", err),
		)
	} else {
		out.Upcoming = upcoming
	}

	// Tier definitions for chip coloring (Wave 1.6.5 plumbing).
	// Falls back to the platform default vocabulary inside the
	// templ if the slice is empty (same fallback as h.loadTierDefinitions
	// in handler_v2.go).
	out.TierDefinitions = h.loadTierDefinitions(ctx, cc.Campaign.ID)
	return out
}
