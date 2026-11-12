package entities

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Entity List widget limits; the widget's settings clamp to the same range.
const (
	entityListDefaultLimit = 8
	entityListMaxLimit     = 20
)

// EntityListHandler serves the Entity List home widget: one category's pages,
// as far as the viewer may open them.
type EntityListHandler struct {
	svc EntityService
}

// NewEntityListHandler creates the Entity List widget handler.
func NewEntityListHandler(svc EntityService) *EntityListHandler {
	return &EntityListHandler{svc: svc}
}

// RegisterEntityListRoutes mounts the widget. Like the home it sits on, it is
// public-capable; what a visitor sees is decided by the visibility filter.
func RegisterEntityListRoutes(e *echo.Echo, h *EntityListHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	pub := e.Group("/campaigns/:id",
		auth.OptionalAuth(authSvc),
		campaigns.AllowPublicCampaignAccess(campaignSvc),
	)
	pub.GET("/entity-list", h.Widget, campaigns.RequireViewAccess())
}

// Widget draws the list for ?type=<category id>&limit=<n>. A Director looking
// as a player reads pages as a player, so the home previews what players see.
func (h *EntityListHandler) Widget(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := c.Request().Context()

	typeID, _ := strconv.Atoi(c.QueryParam("type"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > entityListMaxLimit {
		limit = entityListDefaultLimit
	}

	// A category from another campaign is treated as not chosen, so its name
	// never reaches this home.
	var et *EntityType
	if typeID > 0 {
		if t, err := h.svc.GetEntityTypeByID(ctx, typeID); err == nil && t != nil && t.CampaignID == cc.Campaign.ID {
			et = t
		}
	}
	if et == nil {
		return middleware.Render(c, http.StatusOK, EntityListWidget(cc, nil, nil))
	}

	role := int(cc.VisibilityRole())
	if campaigns.ViewingAsPlayer(c, cc) {
		role = int(campaigns.RolePlayer)
	}
	pages, _, err := h.svc.List(ctx, cc.Campaign.ID, et.ID, role, auth.GetUserID(c), ListOptions{Page: 1, PerPage: limit})
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, EntityListWidget(cc, et, pages))
}
