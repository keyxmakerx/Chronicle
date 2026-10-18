package entities

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// StandingHandler serves the Standing block's JSON. It binds, checks the page
// is one the caller may open, and calls the service; what a viewer may be told
// is the service's decision.
type StandingHandler struct {
	svc      StandingService
	entities EntityService
}

// NewStandingHandler creates the Standing handler.
func NewStandingHandler(svc StandingService, entities EntityService) *StandingHandler {
	return &StandingHandler{svc: svc, entities: entities}
}

// directorOnly gates the writes. The service checks the role again, so this is
// the early, cheap refusal and the service is the one that cannot be skipped.
func directorOnly() echo.MiddlewareFunc {
	return campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the campaign owner or a granted co-Director may change a standing")
}

// RegisterStandingRoutes mounts the Standing block's routes. Reads need a
// campaign member (the service narrows them further); writes are the
// Director's.
func RegisterStandingRoutes(e *echo.Echo, h *StandingHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		withActorMiddleware,
	)
	player := campaigns.RequireRole(campaigns.RolePlayer)
	cg.GET("/entities/:eid/standing/:subject", h.Get, player)
	cg.POST("/entities/:eid/standing/:subject/reasons", h.AddReason, directorOnly())
	cg.DELETE("/entities/:eid/standing/:subject/reasons/:rid", h.RemoveReason, directorOnly())
	cg.PUT("/entities/:eid/standing-settings", h.SetSettings, directorOnly())
}

// page loads the page the block sits on and checks the caller may open it, so
// an id from another campaign, or a page hidden from this player, reads as not
// found. VisibilityRole is used as on the page itself, so a Co-Director sees
// what the owner sees.
func (h *StandingHandler) page(c echo.Context) (*campaigns.CampaignContext, *Entity, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, nil, apperror.NewMissingContext()
	}
	ctx := c.Request().Context()
	entity, err := h.entities.GetByID(ctx, c.Param("eid"))
	if err != nil {
		return nil, nil, err
	}
	if entity.CampaignID != cc.Campaign.ID {
		return nil, nil, apperror.NewNotFound("page not found")
	}
	access, err := h.entities.CheckEntityAccess(ctx, entity.ID, cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil || !access.CanView {
		return nil, nil, apperror.NewNotFound("page not found")
	}
	return cc, entity, nil
}

// Get returns one subject's standing for the caller.
func (h *StandingHandler) Get(c echo.Context) error {
	cc, entity, err := h.page(c)
	if err != nil {
		return err
	}
	v, err := h.svc.Subject(c.Request().Context(), cc.Campaign.ID, entity.ID, c.Param("subject"),
		cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, v)
}

// standingReasonBody is the JSON a new reason arrives as.
type standingReasonBody struct {
	Direction string `json:"direction"`
	Text      string `json:"text"`
	When      string `json:"when"`
	DMOnly    bool   `json:"dmOnly"`
	Level     *int   `json:"level"`
}

// AddReason adds a reason to a subject's standing and returns the standing.
func (h *StandingHandler) AddReason(c echo.Context) error {
	cc, entity, err := h.page(c)
	if err != nil {
		return err
	}
	var body standingReasonBody
	if err := c.Bind(&body); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	v, err := h.svc.AddReason(c.Request().Context(), AddStandingReasonInput{
		CampaignID: cc.Campaign.ID, EntityID: entity.ID, Subject: c.Param("subject"),
		Direction: body.Direction, Text: body.Text, When: body.When, DMOnly: body.DMOnly, Level: body.Level,
		Role: cc.VisibilityRole(), UserID: auth.GetUserID(c),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, v)
}

// RemoveReason removes a reason and returns the standing that is left.
func (h *StandingHandler) RemoveReason(c echo.Context) error {
	cc, entity, err := h.page(c)
	if err != nil {
		return err
	}
	v, err := h.svc.RemoveReason(c.Request().Context(), cc.Campaign.ID, entity.ID, c.Param("subject"), c.Param("rid"),
		cc.VisibilityRole(), auth.GetUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, v)
}

// standingSettingsBody is the JSON the page switch arrives as. PlayersSee is a
// pointer so a body without it is refused instead of reading as "off".
type standingSettingsBody struct {
	PlayersSee *bool `json:"playersSee"`
}

// SetSettings sets whether players can see the page's standing.
func (h *StandingHandler) SetSettings(c echo.Context) error {
	cc, entity, err := h.page(c)
	if err != nil {
		return err
	}
	var body standingSettingsBody
	if err := c.Bind(&body); err != nil || body.PlayersSee == nil {
		return apperror.NewBadRequest("playersSee is required")
	}
	if err := h.svc.SetPlayersSee(c.Request().Context(), cc.Campaign.ID, entity.ID,
		cc.VisibilityRole(), auth.GetUserID(c), *body.PlayersSee); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"playersSee": *body.PlayersSee})
}
