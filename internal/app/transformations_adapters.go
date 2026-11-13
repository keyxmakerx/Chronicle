package app

// transformations_adapters.go bridges hero layers to the entities service.
// The plugin owns the narrow interface; the adapter lives here so neither
// imports the other.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
	"github.com/keyxmakerx/chronicle/internal/plugins/transformations"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// transformationsPageAdapter implements transformations.PageDirectory with
// the same page rules the encounters and Tests blocks use.
type transformationsPageAdapter struct{ pages *encountersPageAdapter }

func (a *transformationsPageAdapter) Access(ctx context.Context, campaignID, entityID string, v transformations.Viewer) (transformations.PageAccess, error) {
	got, err := a.pages.Access(ctx, campaignID, entityID, encounters.Viewer{UserID: v.UserID, VisibilityRole: v.VisibilityRole})
	return transformations.PageAccess{Exists: got.Exists, CanView: got.CanView, CanEdit: got.CanEdit}, err
}

// transformationsLayersSyncAdapter serves a page's layers on the Sync API
// through the same service, so the page rules hold there too.
type transformationsLayersSyncAdapter struct{ svc transformations.Service }

func (a *transformationsLayersSyncAdapter) PageLayers(ctx context.Context, campaignID, entityID, userID string, role int) (any, error) {
	return a.svc.ListForPage(ctx, campaignID, entityID, transformations.Viewer{UserID: userID, VisibilityRole: role})
}

// transformationsLayersPublisher tells the GM side over the WebSocket that a
// page's layers changed. RequiresDM keeps it to the owner-keyed Foundry
// module and DM-team browsers; the message names the page only.
type transformationsLayersPublisher struct{ bus ws.EventBus }

func (p *transformationsLayersPublisher) LayersChanged(campaignID, entityID string) {
	msg := ws.NewMessage(ws.MsgHeroLayersChanged, campaignID, entityID, nil)
	msg.RequiresDM = true
	p.bus.Publish(msg)
}
