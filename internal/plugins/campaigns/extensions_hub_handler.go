// extensions_hub_handler.go — C-EXT-HUB Phase 1 handlers for the
// top-level Extensions hub.
//
// Routes (registered in `internal/plugins/campaigns/routes.go`):
//   - GET /campaigns/:id/extensions          → ExtensionsHub (owner)
//   - GET /campaigns/:id/extensions/fragment → ExtensionsHubFragmentAPI (owner)
//
// The hub owns the bare /campaigns/:id/extensions path; the
// extensions plugin's standalone `ListCampaignExtensions` GET (which
// previously owned that path for Content Packs) retires in this PR
// and Content Packs is re-rendered as a card inside the hub via the
// `ContentPacksCardRenderer` interface.

package campaigns

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// ExtensionsHub renders the top-level Extensions hub page for owners.
//
// Catalog source is the same `AddonLister.ListForPluginHub` the
// Features tab + the older /plugins page use; the
// `addonListerAdapter` populates the new HasDashboard / HasEntitySetup
// capability flags from `HasExtensionDashboard` / `HasExtensionEntitySetup`
// (see extensions_hub.go).
//
// The Content Packs card is sourced from the extensions plugin via
// the `ContentPacksCardRenderer` interface; a nil renderer (or a
// render error) degrades gracefully — the hub omits the card and logs
// a warning instead of failing the page.
func (h *Handler) ExtensionsHub(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()

	var addons []PluginHubAddon
	if h.addonLister != nil {
		var err error
		addons, err = h.addonLister.ListForPluginHub(ctx, cc.Campaign.ID)
		if err != nil {
			slog.Warn("extensions hub: list addons failed", slog.Any("error", err))
		}
	}

	var contentPacksCard templ.Component
	if h.contentPacksRenderer != nil {
		card, err := h.contentPacksRenderer.RenderCampaignExtensionList(ctx, cc)
		if err != nil {
			slog.Warn("extensions hub: content packs render failed",
				slog.String("campaign_id", cc.Campaign.ID),
				slog.Any("error", err),
			)
		} else {
			contentPacksCard = card
		}
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK,
		ExtensionsHubPage(cc, addons, csrfToken, contentPacksCard))
}

// ExtensionsHubFragmentAPI returns the catalog grid as an HTMX
// fragment. The hub container in ExtensionsHubPage listens for the
// `extensions-hub-refresh` event and re-fetches via this endpoint;
// the existing addons-toggle handler emits the event via the
// HX-Trigger header when `redirect_to=extensions-hub` is posted
// alongside the toggle.
func (h *Handler) ExtensionsHubFragmentAPI(c echo.Context) error {
	cc := GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	var addons []PluginHubAddon
	if h.addonLister != nil {
		var err error
		addons, err = h.addonLister.ListForPluginHub(c.Request().Context(), cc.Campaign.ID)
		if err != nil {
			slog.Warn("extensions hub fragment: list addons failed", slog.Any("error", err))
		}
	}

	csrfToken := middleware.GetCSRFToken(c)
	return middleware.Render(c, http.StatusOK,
		ExtensionsHubFragment(cc, addons, csrfToken))
}
