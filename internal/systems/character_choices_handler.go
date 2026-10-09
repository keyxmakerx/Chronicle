package systems

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CharacterChoiceHandler serves the pick lists for character fields.
type CharacterChoiceHandler struct {
	svc CharacterChoiceService
}

// NewCharacterChoiceHandler creates the handler.
func NewCharacterChoiceHandler(svc CharacterChoiceService) *CharacterChoiceHandler {
	return &CharacterChoiceHandler{svc: svc}
}

// ChoicesAPI returns the pick list for one character field.
//
// GET /campaigns/:id/character-choices/:fieldKey
//
// Players can read it, since a list of ancestries or kits is reference data
// they see in the system pages anyway; whether they may then write the field
// is the entity fields endpoint's decision.
func (h *CharacterChoiceHandler) ChoicesAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	list, err := h.svc.CharacterChoiceList(WithChoiceViewer(c.Request().Context(), cc.CanAuthorDmOnly()), cc.Campaign.ID, c.Param("fieldKey"))
	if err != nil {
		return err
	}
	// Private: the list depends on which systems this campaign enabled.
	c.Response().Header().Set("Cache-Control", "private, max-age=300")
	return c.JSON(http.StatusOK, list)
}

// RegisterCharacterChoiceRoutes mounts the pick-list endpoint.
func RegisterCharacterChoiceRoutes(e *echo.Echo, h *CharacterChoiceHandler, authSvc auth.AuthService, campaignSvc campaigns.CampaignService) {
	g := e.Group("/campaigns/:id/character-choices",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
		campaigns.RequireRole(campaigns.RolePlayer),
	)
	g.GET("/:fieldKey", h.ChoicesAPI)
}
