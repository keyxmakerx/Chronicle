package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/systemstate"
	"github.com/keyxmakerx/chronicle/internal/systems"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// systemStateEntityLookup tells systemstate which campaign an entity belongs
// to, over the entities service, so the state plugin never imports entities.
type systemStateEntityLookup struct {
	entities entities.EntityService
}

// EntityCampaignID returns the entity's campaign; a missing entity surfaces as
// the service's NotFound error.
func (a *systemStateEntityLookup) EntityCampaignID(ctx context.Context, entityID string) (string, error) {
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil {
		return "", err
	}
	return e.CampaignID, nil
}

// enabledSystemResolver is the one systems capability the adapters need: which
// system, if any, is enabled for a campaign.
type enabledSystemResolver interface {
	EnabledSystem(ctx context.Context, campaignID string) systems.System
}

// systemStateSystemChecker reports whether a system is the campaign's enabled
// one, so a disabled system cannot accumulate state.
type systemStateSystemChecker struct {
	systems enabledSystemResolver
}

// IsSystemEnabled is true only when the campaign's enabled system has this id.
func (a *systemStateSystemChecker) IsSystemEnabled(ctx context.Context, campaignID, systemID string) (bool, error) {
	sys := a.systems.EnabledSystem(ctx, campaignID)
	if sys == nil {
		return false, nil
	}
	return sys.Info().ID == systemID, nil
}

// systemStatePublisher announces a write over the campaign's WebSocket. The
// payload names the system and key only, and RequiresDM keeps it to the GM
// side, so no state content and no player audience.
type systemStatePublisher struct {
	bus ws.EventBus
}

// PublishSystemStateUpdated sends system_state.updated for the entity.
func (p *systemStatePublisher) PublishSystemStateUpdated(campaignID, entityID, systemID, key string) {
	msg := ws.NewMessage(ws.MsgSystemStateUpdated, campaignID, entityID, map[string]string{
		"systemId": systemID,
		"key":      key,
	})
	msg.RequiresDM = true
	p.bus.Publish(msg)
}

// systemStateSyncReader adapts the system-state service to the sync API's
// read seam.
type systemStateSyncReader struct {
	svc systemstate.Service
}

// ReadSystemState returns both halves; the sync handler decides which leave.
func (a *systemStateSyncReader) ReadSystemState(ctx context.Context, campaignID, entityID, systemID, key string) (json.RawMessage, json.RawMessage, *time.Time, error) {
	st, err := a.svc.Get(ctx, campaignID, entityID, systemID, key)
	if err != nil {
		return nil, nil, nil, err
	}
	return st.GM, st.Public, st.UpdatedAt, nil
}
