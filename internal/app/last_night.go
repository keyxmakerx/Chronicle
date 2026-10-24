package app

// The home page's "Last night: write it up" card. It points the table's
// note-taker at the latest game night nobody has written up yet. It lives in
// internal/app because the campaign home may not import the sessions plugin.

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// lastNightNights is the slice of sessions.SessionService the card reads.
type lastNightNights interface {
	LastNightToWriteUp(ctx context.Context, campaignID, today string) (*sessions.Session, error)
}

// lastNightView is the card's content.
type lastNightView struct {
	// Lead is "Last night" for yesterday's game, else the night's date.
	Lead string
	Name string
	Href string
}

// buildLastNight words the card for the viewer's own today; ok is false
// when there is nothing to write up.
func buildLastNight(ctx context.Context, svc lastNightNights, campaignID string, now time.Time, loc *time.Location) (lastNightView, bool, error) {
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	n, err := svc.LastNightToWriteUp(ctx, campaignID, local.Format("2006-01-02"))
	if err != nil || n == nil || n.ScheduledDate == nil {
		return lastNightView{}, false, err
	}
	lead := "Last night"
	if *n.ScheduledDate != local.AddDate(0, 0, -1).Format("2006-01-02") {
		lead = "Game night, " + n.FormatScheduledDate()
	}
	return lastNightView{
		Lead: lead,
		Name: n.Name,
		Href: "/campaigns/" + url.PathEscape(campaignID) + "/sessions/" + url.PathEscape(n.ID) + "?write=1#session-recap",
	}, true, nil
}

// lastNightHandler serves the card. Only someone who may write a recap is
// asked to; everyone else, and a campaign without game nights, gets an
// empty body, which removes the home page's placeholder.
type lastNightHandler struct {
	nights  lastNightNights
	addons  comingUpAddons
	healthy func(slug string) bool
	now     func() time.Time
	// zone is the viewer's stored zone, or nil (UTC) when unknown.
	zone func(ctx context.Context, userID string) *time.Location
}

// Show renders the card.
// GET /campaigns/:id/last-night
func (h *lastNightHandler) Show(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	userID := auth.GetUserID(c)
	// The same bar as the recap's own save route.
	if !cc.IsMember || cc.MemberRole < campaigns.RoleScribe || userID == "" {
		return c.HTML(http.StatusOK, "")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), comingUpTimeout)
	defer cancel()
	on := func(slug string) bool {
		ok, err := h.addons.IsEnabledForCampaign(ctx, cc.Campaign.ID, slug)
		return err == nil && ok && (h.healthy == nil || h.healthy(slug))
	}
	// Game nights need both switches, as the sessions routes do.
	if !on(calendar.PluginSlug) || !on(sessions.SessionsAddonSlug) {
		return c.HTML(http.StatusOK, "")
	}
	var loc *time.Location
	if h.zone != nil {
		loc = h.zone(ctx, userID)
	}
	view, ok, err := buildLastNight(ctx, h.nights, cc.Campaign.ID, h.now(), loc)
	if err != nil {
		logComingUpErr("last night", cc.Campaign.ID, err)
	}
	if !ok {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, lastNightCard(view))
}

// registerLastNightRoutes mounts the card's fragment beside Coming up's.
func registerLastNightRoutes(e *echo.Echo, h *lastNightHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	g := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	g.GET("/last-night", h.Show, campaigns.RequireViewAccess())
}
