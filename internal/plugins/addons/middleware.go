package addons

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// RequireAddon returns middleware that checks whether the specified addon is
// enabled for the current campaign. It queries the AddonService directly
// (one cheap DB query) because the LayoutInjector only runs at render time.
//
// When the addon is disabled:
//   - HTMX requests receive a 404 with a human-readable message
//   - Browser requests redirect to the campaign dashboard
//
// Must be applied AFTER RequireCampaignAccess (needs campaign :id param).
func RequireAddon(svc AddonService, slug string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			campaignID := c.Param("id")
			if campaignID == "" {
				return apperror.NewBadRequest("campaign ID is required")
			}

			enabled, err := svc.IsEnabledForCampaign(c.Request().Context(), campaignID, slug)
			if err != nil {
				// If we can't check, allow through (fail open for DB errors).
				return next(c)
			}
			if enabled {
				return next(c)
			}

			// Addon is disabled — return appropriate response.
			if middleware.IsHTMX(c) {
				return apperror.NewNotFound(slug + " addon is not enabled for this campaign")
			}

			// Full page — redirect to campaign dashboard.
			return c.Redirect(http.StatusSeeOther, "/campaigns/"+campaignID)
		}
	}
}
