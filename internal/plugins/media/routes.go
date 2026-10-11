package media

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MaxUploadResolver returns the per-request upload size cap in bytes.
// The body-limit middleware calls this on every /media/upload to honor
// the live admin setting (and any per-user override) instead of being
// frozen at process start. Returning <= 0 means "no extra cap" — the
// middleware skips the size check.
//
// Receives the echo context so the resolver can extract the user ID
// (and, for handlers that already parsed it, campaign ID) and consult
// settings.GetEffectiveLimits — exactly what the application-layer
// quota check does inside the upload handler.
type MaxUploadResolver func(c echo.Context) int64

// RegisterRoutes sets up all media-related routes on the given Echo instance.
// resolveMaxUpload is consulted on every upload to determine the body-size
// cap for that request (so the admin can change the global limit without
// requiring a server restart). serveRateLimit controls the max media serve
// requests per minute per IP (0 = 300 default).
func RegisterRoutes(e *echo.Echo, h *Handler, authSvc auth.AuthService, resolveMaxUpload MaxUploadResolver, serveRateLimit int) {
	// Serve routes are protected by:
	// 1. HMAC-signed URLs (handler-level, verifies cryptographic signature)
	// 2. Campaign membership check for private campaigns (handler-level)
	// 3. Per-IP rate limiting (middleware-level, prevents scraping/DoS)
	// 4. OptionalAuth for session-based fallback access during migration
	if serveRateLimit <= 0 {
		serveRateLimit = 300
	}
	serveRL := middleware.RateLimit(serveRateLimit, time.Minute)
	authOptional := auth.OptionalAuth(authSvc)
	e.GET("/media/:id", h.Serve, authOptional, serveRL)
	e.GET("/media/:id/thumb/:size", h.ServeThumbnail, authOptional, serveRL)

	// Authenticated routes.
	authMw := auth.RequireAuth(authSvc)

	// Rate limit uploads: 30 per minute per IP.
	uploadRateLimit := middleware.RateLimit(30, time.Minute)

	// Limit upload body size to prevent memory exhaustion. Resolved per-
	// request so admin changes to the global cap take effect immediately
	// without restarting the server.
	bodyLimit := dynamicBodyLimitMiddleware(resolveMaxUpload)

	e.POST("/media/upload", h.Upload, authMw, uploadRateLimit, bodyLimit)
	e.GET("/media/:fileID/info", h.Info, authMw)
	e.DELETE("/media/:fileID", h.Delete, authMw)
}

// RegisterCampaignRoutes sets up campaign-scoped media management routes.
// The media browser (browse/delete) is Owner-only and gated behind the
// media-gallery addon. When the addon is disabled for a campaign, these
// routes return 404.
//
// /media/:mid/refs ("where is this used") is deliberately NOT given a
// route-level role gate (ADR-058 decision 4): campaigns.RequireRole checks
// the raw MemberRole, which would reject a co-DM (Player role + a DM grant)
// who must see this exactly like the DM does. The handler itself gates on
// the promoted cc.VisibilityRole() (>= Scribe) and filters the list to
// entities the viewer may see — see CampaignMediaRefs. It still requires
// real campaign membership and the media-gallery addon via the group below.
//
// The /media/list JSON endpoint is registered separately (without addon
// gating) — picking existing media is core editor functionality, not an
// addon-gated feature. Used by the media-picker widget.
func RegisterCampaignRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, addonSvc addons.AddonService) {
	gallery := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		addons.RequireAddon(addonSvc, "media-gallery"),
	)

	gallery.GET("/media", h.CampaignMedia, campaigns.RequireRole(campaigns.RoleOwner))
	gallery.DELETE("/media/:mid", h.CampaignDeleteMedia, campaigns.RequireRole(campaigns.RoleOwner))
	gallery.GET("/media/:mid/refs", h.CampaignMediaRefs)

	// Picker JSON endpoint — NOT addon-gated, Scribe+ for editing
	// surfaces (map settings, entity images) that consume the picker.
	picker := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	picker.GET("/media/list", h.CampaignMediaList, campaigns.RequireRole(campaigns.RoleScribe))
}

// RegisterPageFileRoutes sets up the Files section of a page. Every route needs
// a signed-in campaign member; what a member may do with a given page and file
// is decided by PageFileService from the page itself, so the routes carry no
// role gate beyond membership (an editor-player must get through, a Scribe
// must not on a page they cannot see).
//
// Uploads go through the same per-request size cap the media upload route
// uses, and downloads are rate limited like every other media serve.
func RegisterPageFileRoutes(e *echo.Echo, h *PageFileHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService, resolveMaxUpload MaxUploadResolver, serveRateLimit int) {
	if serveRateLimit <= 0 {
		serveRateLimit = 300
	}
	g := e.Group("/campaigns/:id/entities/:eid",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
	)
	g.GET("/files", h.Section)
	g.POST("/files", h.Upload, middleware.RateLimit(30, time.Minute), dynamicBodyLimitMiddleware(resolveMaxUpload))
	g.GET("/files/:fid/download", h.Download, middleware.RateLimit(serveRateLimit, time.Minute))
	g.PUT("/files/:fid/visibility", h.SetVisibility)
	g.DELETE("/files/:fid", h.Remove)
}

// dynamicBodyLimitMiddleware rejects request bodies exceeding the cap
// returned by the resolver. Adds a 10% margin above the resolver's value
// to absorb multipart-encoding overhead — the application-layer quota
// check enforces the exact byte limit. A resolver returning <= 0 (or a
// nil resolver) disables the middleware-level cap, leaving only the
// handler-level enforcement in place.
func dynamicBodyLimitMiddleware(resolver MaxUploadResolver) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if resolver == nil {
				return next(c)
			}
			cap := resolver(c)
			if cap <= 0 {
				// Resolver opted out — no middleware-level cap. Handler
				// still applies the application-layer quota check.
				return next(c)
			}
			maxBytes := cap + cap/10
			if c.Request().ContentLength > maxBytes {
				return &apperror.AppError{
					Code:    http.StatusRequestEntityTooLarge,
					Type:    "request_too_large",
					Message: fmt.Sprintf("request body too large; maximum is %d MB", cap/(1024*1024)),
				}
			}
			c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBytes)
			return next(c)
		}
	}
}
