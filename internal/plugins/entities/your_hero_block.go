package entities

// The campaign home's "Your hero" block: the viewer's own claimed hero with
// its identity facts, or a quiet way to choose one. What it reads and shows is
// what the party block shows for that hero (partyFacts), so a GM-only or
// owner-only field never appears.

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// yourHeroView is what the block draws. Hero is nil when the viewer plays no
// hero yet.
type yourHeroView struct {
	CampaignID string
	Hero       *partyHero
}

// YourHeroBlock draws the block, or an empty answer that removes the
// placeholder for someone with no hero to show or choose (a visitor, or a
// campaign that doesn't let players claim heroes).
// GET /campaigns/:id/your-hero
func (h *Handler) YourHeroBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	ctx := c.Request().Context()
	campaignID := cc.Campaign.ID
	if !cc.IsMember || userID == "" || !h.isAddonEnabled(ctx, campaignID, AddonPlayerCharacterClaiming) {
		return c.HTML(http.StatusOK, "")
	}
	// The Director previewing as a player sees what a player with no special
	// standing would: the block judged as a player.
	role := int(cc.VisibilityRole())
	if campaigns.ViewingAsPlayer(c, cc) {
		role = int(campaigns.RolePlayer)
	}
	var charTypes []int
	if h.charLists != nil {
		var err error
		if charTypes, err = h.charLists.CharacterTypeIDs(ctx, campaignID); err != nil {
			slog.Warn("your hero block: character types failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
			return c.HTML(http.StatusOK, "")
		}
	}
	var mine []CastMember
	for _, m := range filterCastByType(h.buildCastParty(ctx, cc, userID, role), charTypes) {
		if m.IsViewer {
			mine = append(mine, m)
		}
	}
	view := yourHeroView{CampaignID: campaignID}
	if heroes := buildParty(campaignID, mine, h.typesByID(ctx, campaignID)).Heroes; len(heroes) > 0 {
		view.Hero = &heroes[0]
	}
	return middleware.Render(c, http.StatusOK, yourHeroBlock(view))
}
