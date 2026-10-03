package foundry_vtt

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// NPCResolver answers whether an entity is one of the campaign's NPC
// pages, so the spotlight never targets a location, item or hero.
// Implemented in app over the entities service (T-B2: no import here).
type NPCResolver interface {
	NPCName(ctx context.Context, campaignID, entityID string) (name string, ok bool, err error)
}

// SpotlightPublisher sends the spotlight to the campaign's Foundry
// connection. The message carries only the entity id and is GM-only.
type SpotlightPublisher interface {
	PublishNPCSpotlight(campaignID, entityID string)
}

// SetNPCSpotlight wires the "Show in Foundry" button's dependencies.
// Without them the button never renders.
func (h *Handler) SetNPCSpotlight(r NPCResolver, p SpotlightPublisher) {
	h.npcResolver = r
	h.spotlight = p
}

// NPCSpotlightView is what the button templ renders.
type NPCSpotlightView struct {
	CampaignID string
	EntityID   string
	CSRFToken  string
	// Status is the line shown after a press; empty before one.
	Status string
	// StatusOK picks the status colour.
	StatusOK bool
}

// NPCSpotlightButtonHandler serves the "Show in Foundry" button for an
// NPC page header. It renders nothing unless the page is an NPC and a
// Foundry module has connected to this campaign at least once, so
// campaigns that don't use Foundry never see it.
//
// GET /campaigns/:id/foundry-vtt/npc-spotlight/:eid
func (h *Handler) NPCSpotlightButtonHandler(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !h.spotlightReady(c.Request().Context(), cc.Campaign.ID, c.Param("eid")) {
		return c.NoContent(http.StatusOK)
	}
	return middleware.Render(c, http.StatusOK, NPCSpotlightButton(NPCSpotlightView{
		CampaignID: cc.Campaign.ID,
		EntityID:   c.Param("eid"),
		CSRFToken:  middleware.GetCSRFToken(c),
	}))
}

// NPCSpotlightAPI asks Foundry to spotlight the NPC's token. Foundry
// decides who sees it: a hidden token or one on another scene only
// tells the GM there.
//
// POST /campaigns/:id/foundry-vtt/npc-spotlight/:eid
func (h *Handler) NPCSpotlightAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	if !h.spotlightReady(c.Request().Context(), cc.Campaign.ID, entityID) {
		return apperror.NewNotFound("entity not found")
	}
	view := NPCSpotlightView{
		CampaignID: cc.Campaign.ID,
		EntityID:   entityID,
		CSRFToken:  middleware.GetCSRFToken(c),
	}
	if !h.resolvePresence(cc.Campaign.ID).Connected {
		view.Status = "Foundry isn't connected right now."
		return middleware.Render(c, http.StatusOK, NPCSpotlightButton(view))
	}
	h.spotlight.PublishNPCSpotlight(cc.Campaign.ID, entityID)
	view.Status = "Sent to Foundry."
	view.StatusOK = true
	return middleware.Render(c, http.StatusOK, NPCSpotlightButton(view))
}

// spotlightReady reports whether the button applies: dependencies wired,
// Foundry seen at least once, and the entity is one of this campaign's NPCs.
func (h *Handler) spotlightReady(ctx context.Context, campaignID, entityID string) bool {
	if h.npcResolver == nil || h.spotlight == nil || entityID == "" {
		return false
	}
	if h.resolvePresence(campaignID).NeverSeen {
		return false
	}
	_, ok, err := h.npcResolver.NPCName(ctx, campaignID, entityID)
	return err == nil && ok
}
