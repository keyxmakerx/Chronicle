package audit

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler handles HTTP requests for audit log operations. Handlers are thin:
// bind request, call service, render response. No business logic lives here.
type Handler struct {
	service AuditService
}

// NewHandler creates a new audit handler.
func NewHandler(service AuditService) *Handler {
	return &Handler{service: service}
}

// Activity renders the campaign activity page showing audit stats and a
// timeline of recent actions (GET /campaigns/:id/activity). Restricted to
// campaign owners (role >= 3) via route middleware.
func (h *Handler) Activity(c echo.Context) error {
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

	// Fetch activity feed and campaign stats in sequence. Both are needed
	// for the full page render.
	entries, total, err := h.service.GetCampaignActivity(ctx, campaignID, page)
	if err != nil {
		return err
	}

	stats, err := h.service.GetCampaignStats(ctx, campaignID)
	if err != nil {
		return err
	}

	return middleware.Render(c, http.StatusOK, ActivityPage(cc, stats, entries, total, page, perPage))
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
	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	entries, err := h.service.GetEntityHistory(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, entries)
}
