// handler.go provides HTTP endpoints for the NPC gallery. Thin handlers
// that bind request parameters, call the NPC service, and render templ
// components or JSON responses.
package npcs

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// VisibilityToggler toggles an entity's is_private flag and returns the new state.
// Injected from the entities plugin to avoid circular imports.
type VisibilityToggler interface {
	TogglePrivate(ctx context.Context, entityID string) (newPrivate bool, err error)
}

// Handler serves NPC gallery endpoints.
type Handler struct {
	svc        NPCService
	visToggler VisibilityToggler
}

// NewHandler creates a new NPC gallery handler.
func NewHandler(svc NPCService) *Handler {
	return &Handler{svc: svc}
}

// SetVisibilityToggler injects the entity visibility toggler.
func (h *Handler) SetVisibilityToggler(vt VisibilityToggler) {
	h.visToggler = vt
}

// Index renders the NPC gallery page at GET /campaigns/:id/npcs.
// Returns a full page or an HTMX fragment depending on the request header.
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	opts := DefaultNPCListOptions()
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			opts.Page = n
		}
	}
	if pp := c.QueryParam("per_page"); pp != "" {
		if n, err := strconv.Atoi(pp); err == nil && n > 0 && n <= 100 {
			opts.PerPage = n
		}
	}
	if s := c.QueryParam("sort"); s != "" {
		opts.Sort = s
	}
	if q := c.QueryParam("q"); q != "" {
		opts.Search = q
	}
	if t := c.QueryParam("tag"); t != "" {
		opts.Tag = t
	}

	userID := auth.GetUserID(c)
	cards, total, err := h.svc.ListNPCs(c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), userID, opts)
	if err != nil {
		return apperror.NewInternal(err)
	}

	csrfToken := middleware.GetCSRFToken(c)

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, NPCGalleryContent(cc, cards, total, opts, csrfToken))
	}
	return middleware.Render(c, http.StatusOK, NPCGalleryPage(cc, cards, total, opts, csrfToken))
}

// ToggleReveal handles POST /campaigns/:id/npcs/:eid/reveal.
// Toggles an NPC's is_private flag. Only Scribe+ can reveal/hide NPCs.
func (h *Handler) ToggleReveal(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	if h.visToggler == nil {
		return apperror.NewInternal(nil)
	}

	newPrivate, err := h.visToggler.TogglePrivate(c.Request().Context(), entityID)
	if err != nil {
		return apperror.NewInternal(err)
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, RevealBadge(entityID, cc.Campaign.ID, newPrivate, middleware.GetCSRFToken(c)))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"entity_id":  entityID,
		"is_private": newPrivate,
	})
}

// CountAPI returns the NPC count as JSON at GET /campaigns/:id/npcs/count.
// Used by the sidebar badge and layout blocks.
func (h *Handler) CountAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	userID := auth.GetUserID(c)
	count, err := h.svc.CountNPCs(c.Request().Context(), cc.Campaign.ID, int(cc.MemberRole), userID)
	if err != nil {
		return apperror.NewInternal(err)
	}

	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// GalleryBlock fetches NPC cards for embedding in entity page layout blocks.
// Returns a compact card list limited by the block config.
func (h *Handler) GalleryBlock(ctx context.Context, campaignID string, role int, userID string, limit int) ([]NPCCard, error) {
	opts := DefaultNPCListOptions()
	opts.PerPage = limit

	cards, _, err := h.svc.ListNPCs(ctx, campaignID, role, userID, opts)
	return cards, err
}
