package app

// The campaign home's "At a glance" block: the game system, the next game,
// the last session and how many pages the viewer can open. It lives in
// internal/app because the facts come from the addons, sessions and entities
// plugins, which the campaign home may not import. Each fact reads through its
// plugin's own service, and one that fails is left out, never the block.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// glanceAddons lists the campaign's addons: which game system is on, and
// whether game nights are.
type glanceAddons interface {
	ListForCampaign(ctx context.Context, campaignID string) ([]addons.CampaignAddon, error)
}

// glanceNights is the slice of sessions.SessionService the block reads.
type glanceNights interface {
	NextGameNight(ctx context.Context, campaignID string, now time.Time) (*sessions.NextNight, error)
	LastGameNight(ctx context.Context, campaignID, today string) (*sessions.Session, error)
}

// glancePages counts the pages a viewer can open, as the sidebar counts them.
type glancePages interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	CountByType(ctx context.Context, campaignID string, role int, userID string) (map[int]int, error)
}

// glanceFact is one row of the sheet, already worded.
type glanceFact struct {
	Label string
	Value string
	// Note is a quieter second part, such as a session's date.
	Note string
	Href string
}

// glanceRequest says who is looking.
type glanceRequest struct {
	CampaignID string
	UserID     string
	// Role is the viewer's visibility role (a player's while the owner views
	// as one); Director is true for the owner and co-DMs not doing so.
	Role     int
	Director bool
	// Member is true only for a real member: game nights are the table's
	// own business, never shown to a public visitor.
	Member bool
	Now    time.Time
	Loc    *time.Location
}

// glanceSources bundles what the block reads; a nil source is left out.
type glanceSources struct {
	Addons glanceAddons
	Nights glanceNights
	Pages  glancePages
	// NightsHealthy reports the sessions and calendar plugins are up.
	NightsHealthy func() bool
}

// buildAtAGlance words the facts in the signed order. An empty result means
// there is nothing to say and the block is removed.
func buildAtAGlance(ctx context.Context, src glanceSources, req glanceRequest) []glanceFact {
	var facts []glanceFact
	base := "/campaigns/" + url.PathEscape(req.CampaignID)

	var enabled map[string]bool
	if src.Addons != nil {
		list, err := src.Addons.ListForCampaign(ctx, req.CampaignID)
		if err != nil {
			logComingUpErr("at a glance: addons", req.CampaignID, err)
		} else {
			var sys layouts.EnabledSystem
			enabled, sys = navAddonsFrom(list)
			if name := strings.TrimSpace(sys.Name); name != "" {
				facts = append(facts, glanceFact{Label: "System", Value: name})
			}
		}
	}

	// Game nights need both switches, as the sessions routes do.
	nightsOn := req.Member && src.Nights != nil && enabled[calendar.PluginSlug] && enabled[sessions.SessionsAddonSlug] &&
		(src.NightsHealthy == nil || src.NightsHealthy())
	if nightsOn {
		if n, err := src.Nights.NextGameNight(ctx, req.CampaignID, req.Now); err != nil {
			logComingUpErr("at a glance: next game", req.CampaignID, err)
		} else if n != nil {
			facts = append(facts, glanceFact{
				Label: "Next game", Value: glanceNextLabel(n, req.Now),
				Href: base + "/sessions/" + url.PathEscape(n.SessionID),
			})
		}
		loc := req.Loc
		if loc == nil {
			loc = time.UTC
		}
		if s, err := src.Nights.LastGameNight(ctx, req.CampaignID, req.Now.In(loc).Format("2006-01-02")); err != nil {
			logComingUpErr("at a glance: last session", req.CampaignID, err)
		} else if s != nil {
			facts = append(facts, glanceFact{
				Label: "Last session", Value: s.Name, Note: glanceDateLabel(s.ScheduledDate, req.Now),
				Href: base + "/sessions/" + url.PathEscape(s.ID),
			})
		}
	}

	if src.Pages != nil {
		if n, ok := glancePageCount(ctx, src.Pages, req); ok && (n > 0 || req.Director) {
			facts = append(facts, glanceFact{Label: "Pages", Value: glancePagesLabel(n), Href: base + "/entities"})
		}
	}
	return facts
}

// glancePageCount totals the pages the viewer can open across the categories
// the viewer's sidebar shows. CountByType has already added each
// sub-category into its parent, so only top-level categories are summed.
func glancePageCount(ctx context.Context, src glancePages, req glanceRequest) (int, bool) {
	types, err := src.GetEntityTypes(ctx, req.CampaignID)
	if err != nil {
		logComingUpErr("at a glance: categories", req.CampaignID, err)
		return 0, false
	}
	counts, err := src.CountByType(ctx, req.CampaignID, req.Role, req.UserID)
	if err != nil {
		logComingUpErr("at a glance: pages", req.CampaignID, err)
		return 0, false
	}
	total := 0
	for _, t := range entities.TypesForViewer(types, req.Role) {
		if t.ParentTypeID == nil {
			total += counts[t.ID]
		}
	}
	return total, true
}

// glanceNextLabel words the next night as "Today, 7 pm", "Friday, 7 pm" or,
// a week or more out, "Friday 4 December". It reads the night's own zone, as
// the header's countdown does.
func glanceNextLabel(n *sessions.NextNight, now time.Time) string {
	loc := n.At.Location()
	nowLocal := now.In(loc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	night := time.Date(n.At.Year(), n.At.Month(), n.At.Day(), 0, 0, 0, 0, loc)
	days := int(night.Sub(today).Hours()/24 + 0.5)
	var day string
	switch {
	case days <= 0:
		day = "Today"
	case days == 1:
		day = "Tomorrow"
	case days < 7:
		day = n.At.Format("Monday")
	default:
		day = n.At.Format("Monday 2 January")
	}
	if n.Time == "" {
		return day
	}
	clock := strings.Replace(strings.ToLower(n.At.Format("3:04 pm")), ":00", "", 1)
	return day + ", " + clock
}

// glanceDateLabel words a session's YYYY-MM-DD date as "7 November", adding
// the year when it is not this year's; "" for no date.
func glanceDateLabel(date *string, now time.Time) string {
	if date == nil {
		return ""
	}
	d, err := time.Parse("2006-01-02", *date)
	if err != nil {
		return ""
	}
	if d.Year() != now.Year() {
		return d.Format("2 January 2006")
	}
	return d.Format("2 January")
}

// glancePagesLabel is the page count as a figure.
func glancePagesLabel(n int) string {
	if n == 1 {
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}

// --- handler ---

// atAGlanceHandler serves the block's fragment.
type atAGlanceHandler struct {
	src glanceSources
	now func() time.Time
	// zone is the viewer's stored zone, or nil (UTC) when unknown.
	zone func(ctx context.Context, userID string) *time.Location
}

// Show renders the sheet, or an empty body that removes the placeholder.
// GET /campaigns/:id/at-a-glance
func (h *atAGlanceHandler) Show(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx, cancel := context.WithTimeout(c.Request().Context(), comingUpTimeout)
	defer cancel()
	userID := auth.GetUserID(c)
	asPlayer := campaigns.ViewingAsPlayer(c, cc)
	req := glanceRequest{
		CampaignID: cc.Campaign.ID,
		UserID:     userID,
		Role:       int(cc.VisibilityRole()),
		Director:   cc.CanAuthorDmOnly() && !asPlayer,
		Member:     cc.IsMember,
		Now:        h.now(),
	}
	if asPlayer {
		req.Role = int(campaigns.RolePlayer)
	}
	if h.zone != nil && userID != "" {
		req.Loc = h.zone(ctx, userID)
	}
	facts := buildAtAGlance(ctx, h.src, req)
	if len(facts) == 0 {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, atAGlanceFragment(facts))
}

// registerAtAGlanceRoutes mounts the block's fragment beside Coming up's.
// Public-capable like the home; what a visitor sees is decided per fact.
func registerAtAGlanceRoutes(e *echo.Echo, h *atAGlanceHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	g := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	g.GET("/at-a-glance", h.Show, campaigns.RequireViewAccess())
}
