package systems

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

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
// GET /campaigns/:id/character-choices/:fieldKey[?as=<userId>]
//
// Players can read it, since a list of ancestries or kits is reference data
// they see in the system pages anyway; whether they may then write the field
// is the entity fields endpoint's decision.
func (h *CharacterChoiceHandler) ChoicesAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	ctx := choiceContext(c, cc)
	list, err := h.svc.CharacterChoiceList(ctx, cc.Campaign.ID, c.Param("fieldKey"))
	if err != nil {
		return err
	}
	// Private and fresh: the list carries the viewer's own drafts and whether
	// they may still make their own, which change as they write.
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, list)
}

// choiceContext says who the list is for. A Director may preview it as one
// player (?as=), which reads it exactly as that player would, rules and all.
// Staff below Director (Scribes) make heroes for the table and are not
// limited. Everyone else is limited by the rules about them.
func choiceContext(c *echo.Context, cc *campaigns.CampaignContext) context.Context {
	ctx := c.Request().Context()
	director := cc.CanAuthorDmOnly()
	as := c.QueryParam("as")
	// A Director with "view as player" on reads the list as a player, rules
	// applied: the sheet's picker is a player's surface.
	if as == "" && campaigns.ViewingAsPlayer(c, cc) {
		as = auth.GetUserID(c)
	}
	if director && as != "" && len(as) <= 36 {
		return WithChoiceFor(WithChoiceViewerUser(WithChoiceViewer(ctx, false), as), as)
	}
	user := auth.GetUserID(c)
	ctx = WithChoiceFor(WithChoiceViewerUser(WithChoiceViewer(ctx, director), user), user)
	if !director && cc.MemberRole >= campaigns.RoleScribe {
		ctx = WithChoiceUnlimited(ctx)
	}
	return ctx
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
