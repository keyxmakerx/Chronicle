package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// creatureAddResolver tells foundry_vtt whether an entity is one of the
// campaign's creature pages, by the rule the creature lists use: a system's
// creature preset, or a type named creature or *-monster.
type creatureAddResolver struct {
	entities entities.EntityService
}

// IsCreature is true for a creature page in this campaign; anything else
// (another campaign, a hero, a place) is false.
func (a *creatureAddResolver) IsCreature(ctx context.Context, campaignID, entityID string) (bool, error) {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil || e.CampaignID != campaignID {
		return false, nil
	}
	et, err := a.entities.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil || et == nil {
		return false, nil
	}
	return entities.IsCreatureType(*et), nil
}

// creatureAddPublisher sends creature_add.requested over the campaign's
// WebSocket. RequiresDM keeps it to the GM side, and the recording bus puts
// it in the change feed for a world that is closed.
type creatureAddPublisher struct {
	bus ws.EventBus
}

// PublishCreatureAdd sends the request with the entity id and no payload.
func (p *creatureAddPublisher) PublishCreatureAdd(campaignID, entityID string) {
	msg := ws.NewMessage(ws.MsgCreatureAddRequested, campaignID, entityID, nil)
	msg.RequiresDM = true
	p.bus.Publish(msg)
}
