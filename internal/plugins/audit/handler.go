package audit

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

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
		return echo.NewHTTPError(http.StatusInternalServerError, "missing campaign context")
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

// EntityHistory returns JSON history for a specific entity
// (GET /campaigns/:id/entities/:eid/history). Used by HTMX or API clients
// to display per-entity change logs.
func (h *Handler) EntityHistory(c echo.Context) error {
	entityID := c.Param("eid")
	if entityID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "entity ID is required")
	}

	entries, err := h.service.GetEntityHistory(c.Request().Context(), entityID)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, entries)
}
