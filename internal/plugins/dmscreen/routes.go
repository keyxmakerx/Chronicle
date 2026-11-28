package dmscreen

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CanOpen reports who may open the DM Screen: the owner and any member the
// owner has given DM access (a Co-DM), since that access is for running the
// game. A Scribe sees what players see. The top-bar button uses the same rule.
func CanOpen(cc *campaigns.CampaignContext) bool {
	return cc.CanAuthorDmOnly()
}

// CanOpenPlayers reports who gets the players' screen: a member who cannot
// open the DM Screen (a player or a scribe), or a DM-team member while they
// view the campaign as a player, whose screen is then built as a player's.
// Never a non-member, a site admin who has not joined, or a guest preview.
func CanOpenPlayers(c *echo.Context, cc *campaigns.CampaignContext) bool {
	if !cc.IsMember || cc.MemberRole < campaigns.RolePlayer || cc.InGuestPreview() {
		return false
	}
	return !CanOpen(cc) || campaigns.ViewingAsPlayer(c, cc)
}

// requirePlayers gates the players' screen routes on CanOpenPlayers.
func requirePlayers(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cc := campaigns.GetCampaignContext(c)
		if cc == nil || !CanOpenPlayers(c, cc) {
			return apperror.NewForbidden("the players' screen is for the campaign's players")
		}
		return next(c)
	}
}

// RegisterRoutes mounts the DM Screen routes behind CanOpen; the service
// checks the role again so a wiring mistake can't open it to players. The
// players' screen routes sit behind CanOpenPlayers, checked again the same way.
func RegisterRoutes(e *echo.Echo, h *Handler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	cg.GET("/dm-screen", h.Show, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.POST("/dm-screen/time", h.StepTime, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.GET("/dm-screen/notes", h.ShowNote, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.PUT("/dm-screen/notes", h.SaveNote, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.POST("/dm-screen/reveal/:eid", h.Reveal, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	// Reveal's Undo hides a page again, so it is for the DM team only.
	cg.POST("/dm-screen/hide/:eid", h.UndoReveal, campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return cc.CanAuthorDmOnly()
	}, "only the campaign owner or a member with DM access may do this"))
	// The Adventures tab: DM team only, like the screen itself.
	cg.GET("/dm-screen/adventures/pages", h.PickPages, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.POST("/dm-screen/adventures/pins", h.PinPage, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.POST("/dm-screen/adventures/notes", h.AddBoardNote, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))
	cg.DELETE("/dm-screen/adventures/items/:itemID", h.RemoveBoardItem, campaigns.RequireCapability(CanOpen, "the DM Screen is for the people running this campaign"))

	cg.GET("/player-screen", h.ShowPlayer, requirePlayers)
	cg.GET("/player-screen/notes", h.ShowPlayerNote, requirePlayers)
	cg.PUT("/player-screen/notes", h.SavePlayerNote, requirePlayers)
}
