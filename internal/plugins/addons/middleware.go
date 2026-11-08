package addons

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// RequireAddon returns middleware that checks whether the specified addon is
// enabled for the current campaign. It queries the AddonService directly
// (one cheap DB query) because the LayoutInjector only runs at render time.
//
// When the addon is disabled:
//   - HTMX requests receive a 404 with a human-readable message
//   - fetch()/JSON callers (widgets that parse every response as JSON, e.g.
//     entity_notes.js) receive a 404 JSON body instead of an HTML redirect
//   - Other browser requests redirect to the campaign dashboard
//
// Must be applied AFTER RequireCampaignAccess (needs campaign :id param).
func RequireAddon(svc AddonService, slug string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
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

			// fetch()/JSON callers get a JSON refusal, not a redirect they'd
			// follow and then fail to parse as JSON (issue #619).
			if middleware.IsAPIRequest(c) {
				return apperror.NewNotFound(slug + " addon is not enabled for this campaign")
			}

			// Full page — redirect to campaign dashboard.
			return c.Redirect(http.StatusSeeOther, "/campaigns/"+campaignID)
		}
	}
}
