package audit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// EntityViewGuard resolves an entity's campaign and whether the requesting
// member may view it. Injected from the entities plugin (adapter in
// app/routes.go) so the audit plugin never imports entities directly — mirrors
// the npcs.VisibilityToggler seam. When unset, EntityHistory still applies the
// unconditional campaign scope on the history query (the cross-campaign IDOR
// stays closed); only the per-entity visibility gate is skipped.
type EntityViewGuard interface {
	// ResolveEntityView returns the entity's campaign id and whether a viewer
	// with the given role/user may see it. Returns an apperror (NotFound) when
	// the entity does not exist.
	ResolveEntityView(ctx context.Context, entityID string, role int, userID string) (campaignID string, canView bool, err error)
}

// Handler handles HTTP requests for audit log operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service     AuditService
	entityGuard EntityViewGuard
}

// NewHandler creates a new audit handler.
func NewHandler(service AuditService) *Handler {
	return &Handler{service: service}
}

// SetEntityViewGuard injects the guard EntityHistory uses to enforce campaign
// ownership + per-entity visibility on the requested entity (SEC-IDOR-2).
func (h *Handler) SetEntityViewGuard(g EntityViewGuard) {
	h.entityGuard = g
}

// Activity redirects to the unified settings page Activity tab.
// GET /campaigns/:id/activity
func (h *Handler) Activity(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(fmt.Errorf("missing campaign context"))
	}

	return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/settings?tab=activity", cc.Campaign.ID))
}

// ActivityFragment returns an HTMX fragment with stats and timeline for the
// settings Activity tab. GET /campaigns/:id/activity/fragment
func (h *Handler) ActivityFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(fmt.Errorf("missing campaign context"))
	}

	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}

	ctx := c.Request().Context()
	campaignID := cc.Campaign.ID

	entries, total, err := h.service.GetCampaignActivity(ctx, campaignID, page)
	if err != nil {
		return err
	}

	stats, err := h.service.GetCampaignStats(ctx, campaignID)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, ActivityContent(cc, stats, entries, total, page, perPage))
}

// EmbedActivity returns an HTMX fragment for the dashboard activity feed block.
// Shows recent campaign activity entries in a compact feed format.
// GET /campaigns/:id/activity/embed
func (h *Handler) EmbedActivity(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewInternal(fmt.Errorf("missing campaign context"))
	}

	limit := 10
	if l := c.QueryParam("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v >= 1 {
			limit = v
		}
		if limit > 30 {
			limit = 30
		}
	}

	ctx := c.Request().Context()
	entries, _, err := h.service.GetCampaignActivity(ctx, cc.Campaign.ID, 1)
	if err != nil {
		return err
	}

	// Trim to requested limit.
	if limit < len(entries) {
		entries = entries[:limit]
	}

	return middleware.Render(c, http.StatusOK, ActivityEmbedFragment(cc, entries))
}

// EntityHistory returns JSON history for a specific entity
// (GET /campaigns/:id/entities/:eid/history). Used by HTMX or API clients
// to display per-entity change logs.
func (h *Handler) EntityHistory(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	// IDOR + visibility guard (SEC-IDOR-2): the entity must belong to the
	// caller's campaign and be viewable by them, mirroring entities.GetEntry's
	// GetByID-campaign + CheckEntityAccess gate. The campaign-scoped query below
	// is the unconditional backstop; this adds the per-entity visibility check.
	if h.entityGuard != nil {
		campaignID, canView, err := h.entityGuard.ResolveEntityView(
			c.Request().Context(), entityID, int(cc.MemberRole), auth.GetUserID(c))
		if err != nil {
			return err
		}
		if campaignID != cc.Campaign.ID || !canView {
			return apperror.NewNotFound("entity not found")
		}
	}

	entries, err := h.service.GetEntityHistory(c.Request().Context(), entityID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, entries)
}
