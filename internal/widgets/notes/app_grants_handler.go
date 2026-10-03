package notes

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// OriginAllower says whether a web origin may receive a notes grant. It is
// the same list that lets that origin call Chronicle across sites (the site
// address plus the admin's allowed origins), so an admin who has not allowed
// a Foundry address cannot have players' notes handed to it.
type OriginAllower interface {
	OriginAllowed(ctx context.Context, origin string) bool
	// AllowedOrigins lists them, for the frame-ancestors of the notes pages
	// an app shows in a frame.
	AllowedOrigins(ctx context.Context) []string
}

// AppGate says whether a campaign lets outside apps in at all. Wired to the
// campaign's Sync API switch, so an owner who turns it off cuts off every
// player's notebook too, as it cuts off the GM's sync.
type AppGate func(ctx context.Context, campaignID string) (bool, error)

// AppGrantHandler serves the Allow window and the player's list of grants.
type AppGrantHandler struct {
	grants  AppGrantService
	origins OriginAllower
}

// NewAppGrantHandler creates the handler for the Allow window.
func NewAppGrantHandler(grants AppGrantService, origins OriginAllower) *AppGrantHandler {
	return &AppGrantHandler{grants: grants, origins: origins}
}

// originHost is a plain host name or IP with an optional port. url.Parse
// accepts hosts such as "*" or "a.com;x", which in a CSP source list would
// mean "any site" or break the policy, so anything else is refused.
var originHost = regexp.MustCompile(`^([a-z0-9-]+(\.[a-z0-9-]+)*|\[[0-9a-f:.]+\])(:[0-9]{1,5})?$`)

// normalizeOrigin returns raw as a bare scheme://host[:port] origin, or ""
// when it is not one. Paths, queries, credentials and fragments are refused
// rather than stripped, so what the player is shown is exactly what gets
// the token.
func normalizeOrigin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return ""
	}
	if !originHost.MatchString(strings.ToLower(u.Host)) {
		return ""
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// allowedOrigin validates the origin query/form value against the allowed
// origins. Matching is exact, as the CORS check is.
func (h *AppGrantHandler) allowedOrigin(c echo.Context, raw string) (string, bool) {
	origin := normalizeOrigin(raw)
	if origin == "" || h.origins == nil {
		return origin, false
	}
	return origin, h.origins.OriginAllowed(c.Request().Context(), origin)
}

// ShowAllow renders the Allow window (GET /campaigns/:id/notes/allow-app).
// The page talks to the window that opened it, so it must keep its opener:
// the site-wide Cross-Origin-Opener-Policy of same-origin would cut that
// link. Framing stays forbidden by the site-wide headers.
func (h *AppGrantHandler) ShowAllow(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	origin, ok := h.allowedOrigin(c, c.QueryParam("origin"))
	view := AllowAppView{
		Origin:        origin,
		OriginAllowed: ok,
	}
	if s := auth.GetSession(c); s != nil {
		view.DisplayName = s.Name
	}
	c.Response().Header().Set("Cross-Origin-Opener-Policy", "unsafe-none")
	return middleware.Render(c, http.StatusOK, AllowAppPage(cc, view))
}

// Allow makes a grant for the signed-in player (POST
// /campaigns/:id/notes/allow-app) and returns the token once, to the Allow
// window's own script, which hands it to the opener at exactly that origin.
func (h *AppGrantHandler) Allow(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req struct {
		Origin string `json:"origin"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request")
	}
	origin, ok := h.allowedOrigin(c, req.Origin)
	if !ok {
		return apperror.NewForbidden("this address isn't allowed to use Chronicle notes; ask your site admin to add it")
	}
	userID := auth.GetUserID(c)
	token, g, err := h.grants.Issue(c.Request().Context(), cc.Campaign.ID, userID, origin)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusCreated, map[string]any{
		"token":      token,
		"grantId":    g.ID,
		"userId":     userID,
		"campaignId": cc.Campaign.ID,
		"origin":     origin,
	})
}

// List returns the player's live grants (GET /campaigns/:id/notes/app-grants).
func (h *AppGrantHandler) List(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	gs, err := h.grants.List(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, gs)
}

// Revoke ends one of the player's grants
// (DELETE /campaigns/:id/notes/app-grants/:gid).
func (h *AppGrantHandler) Revoke(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.grants.Revoke(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), c.Param("gid")); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// ShowEmbed renders the empty shell an allowed app frames
// (GET /embed/campaigns/:id/notes/:mode, mode journal or jots). It needs no
// sign-in and shows nothing about the campaign: the content arrives later
// over the grant the parent window hands it. Only the allowed origins may
// frame it; everywhere else the site stays unframeable.
func (h *AppGrantHandler) ShowEmbed(c echo.Context) error {
	id := c.Param("id")
	mode := c.Param("mode")
	if !idPattern.MatchString(id) || (mode != "journal" && mode != "jots") {
		return apperror.NewNotFound("page not found")
	}
	var origins []string
	if h.origins != nil {
		origins = h.origins.AllowedOrigins(c.Request().Context())
	}
	// Loosen framing only by swapping the site-wide directive. If that
	// directive ever changes shape the page stays unframeable, which the
	// header test catches, rather than being framable by anyone.
	hdr := c.Response().Header()
	csp := hdr.Get("Content-Security-Policy")
	if loosened := strings.Replace(csp, "frame-ancestors 'none'", frameAncestors(origins), 1); loosened != csp {
		hdr.Set("Content-Security-Policy", loosened)
		hdr.Del("X-Frame-Options")
	}
	return middleware.Render(c, http.StatusOK, NotesEmbedPage(id, mode))
}

// frameAncestors is the CSP directive for the embed pages: the site itself
// plus each allowed origin that is a bare origin (anything else could widen
// or break the policy, so it is dropped).
func frameAncestors(origins []string) string {
	parts := []string{"frame-ancestors 'self'"}
	for _, o := range origins {
		if n := normalizeOrigin(o); n != "" {
			parts = append(parts, n)
		}
	}
	return strings.Join(parts, " ")
}

// RequireAppGrant authenticates a request by a notes grant token in
// Authorization: Bearer and makes it the grant's player for what follows.
// The grant is tied to one campaign; a token used against another campaign
// is refused here, before the campaign's own membership check runs. Must sit
// before campaigns.RequireCampaignAccess, which checks the player is still a
// member at their live role.
func RequireAppGrant(grants AppGrantService, gate AppGate) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authz := c.Request().Header.Get("Authorization")
			token, found := strings.CutPrefix(authz, "Bearer ")
			if !found || token == "" {
				return apperror.NewUnauthorized("notes access not allowed")
			}
			g, err := grants.Authenticate(c.Request().Context(), strings.TrimSpace(token))
			if err != nil {
				return err
			}
			if g.CampaignID != c.Param("id") {
				return apperror.NewForbidden("notes access not allowed for this campaign")
			}
			if gate != nil {
				open, err := gate(c.Request().Context(), g.CampaignID)
				if err != nil {
					return apperror.NewInternal(err)
				}
				if !open {
					return apperror.NewForbidden("this campaign has outside apps turned off")
				}
			}
			// A session carrying only the user id: never site admin, so the
			// campaign check below grants exactly the player's membership.
			auth.SetSession(c, &auth.Session{UserID: g.UserID})
			c.Response().Header().Set("Cache-Control", "no-store")
			return next(c)
		}
	}
}
