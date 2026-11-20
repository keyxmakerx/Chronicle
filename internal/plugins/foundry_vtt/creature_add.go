package foundry_vtt

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CreatureResolver answers whether an entity is one of the campaign's
// creature pages, so "Add to Foundry" never asks for a hero or a place.
// Implemented in app over the entities service (T-B2: no import here).
type CreatureResolver interface {
	IsCreature(ctx context.Context, campaignID, entityID string) (bool, error)
}

// CreatureAddPublisher asks the campaign's Foundry module to make the page's
// actor. The message carries only the entity id, is GM-only, and is a
// change-feed row, so a closed world acts on it when it next connects.
type CreatureAddPublisher interface {
	PublishCreatureAdd(campaignID, entityID string)
}

// SetCreatureAdd wires the "Add to Foundry" button's dependencies. Without
// them the button never renders.
func (h *Handler) SetCreatureAdd(r CreatureResolver, p CreatureAddPublisher) {
	h.creatureResolver = r
	h.creatureAdd = p
}

// CreatureAddView is what the button templ renders.
type CreatureAddView struct {
	CampaignID string
	EntityID   string
	CSRFToken  string
	// Status is the line shown after a press; empty before one.
	Status string
	// StatusOK picks the status colour.
	StatusOK bool
}

// CreatureAddButtonHandler serves "Add to Foundry" for a creature page
// header. Creature pages reach Foundry when a fight uses them; this button
// is for one the DM wants there before that. It renders nothing unless the
// page is a creature and a Foundry module has connected at least once.
//
// GET /campaigns/:id/foundry-vtt/creature-add/:eid
func (h *Handler) CreatureAddButtonHandler(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !h.creatureAddReady(c.Request().Context(), cc.Campaign.ID, c.Param("eid")) {
		return c.NoContent(http.StatusOK)
	}
	return middleware.Render(c, http.StatusOK, CreatureAddButton(CreatureAddView{
		CampaignID: cc.Campaign.ID,
		EntityID:   c.Param("eid"),
		CSRFToken:  middleware.GetCSRFToken(c),
	}))
}

// CreatureAddAPI records the request. Foundry makes the actor at once when
// it is connected, or the next time the world opens; one it already has is
// left as it is.
//
// POST /campaigns/:id/foundry-vtt/creature-add/:eid
func (h *Handler) CreatureAddAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	if !h.creatureAddReady(c.Request().Context(), cc.Campaign.ID, entityID) {
		return apperror.NewNotFound("entity not found")
	}
	h.creatureAdd.PublishCreatureAdd(cc.Campaign.ID, entityID)
	view := CreatureAddView{
		CampaignID: cc.Campaign.ID,
		EntityID:   entityID,
		CSRFToken:  middleware.GetCSRFToken(c),
		StatusOK:   true,
		Status:     "Foundry adds it the next time the world is open.",
	}
	if h.resolvePresence(cc.Campaign.ID).Connected {
		view.Status = "Sent. It's in Foundry's Chronicle: Creatures folder."
	}
	return middleware.Render(c, http.StatusOK, CreatureAddButton(view))
}

// creatureAddReady reports whether the button applies: dependencies wired,
// Foundry seen at least once, and the entity is one of this campaign's
// creature pages.
func (h *Handler) creatureAddReady(ctx context.Context, campaignID, entityID string) bool {
	if h.creatureResolver == nil || h.creatureAdd == nil || entityID == "" {
		return false
	}
	if h.resolvePresence(campaignID).NeverSeen {
		return false
	}
	ok, err := h.creatureResolver.IsCreature(ctx, campaignID, entityID)
	return err == nil && ok
}
