package app

import (
	"context"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// npcSpotlightResolver tells foundry_vtt whether an entity is one of the
// campaign's NPC pages, using the same NPC types as the NPC gallery and
// the DM Screen.
type npcSpotlightResolver struct {
	entities entities.EntityService
}

// NPCName returns the page's name and true for an NPC page in this
// campaign; anything else (another campaign, a location, a hero) is false.
func (a *npcSpotlightResolver) NPCName(ctx context.Context, campaignID, entityID string) (string, bool, error) {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil || e.CampaignID != campaignID {
		return "", false, nil
	}
	types, err := a.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return "", false, err
	}
	if !slices.Contains(npcTypeIDs(types), e.EntityTypeID) {
		return "", false, nil
	}
	return e.Name, true, nil
}

// npcSpotlightPublisher sends the spotlight over the campaign's
// WebSocket. RequiresDM keeps it to the GM side: the owner-keyed Foundry
// module and DM-team browsers, which ignore it.
type npcSpotlightPublisher struct {
	bus ws.EventBus
}

// PublishNPCSpotlight sends npc.spotlight with the entity id and no payload.
func (p *npcSpotlightPublisher) PublishNPCSpotlight(campaignID, entityID string) {
	msg := ws.NewMessage(ws.MsgNPCSpotlight, campaignID, entityID, nil)
	msg.RequiresDM = true
	p.bus.Publish(msg)
}
