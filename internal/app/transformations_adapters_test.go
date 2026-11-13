package app

import (
	"testing"

	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// A page's layers changing goes out GM-only with just the page id: a
// player's browser and the change feed never see it.
func TestTransformationsLayersPublisher_GMOnlyIDOnly(t *testing.T) {
	bus := &spotlightCaptureBus{}
	(&transformationsLayersPublisher{bus: bus}).LayersChanged("camp-1", "hero-1")
	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	m := bus.msgs[0]
	if m.Type != ws.MsgHeroLayersChanged || m.CampaignID != "camp-1" || m.ResourceID != "hero-1" {
		t.Errorf("message = %+v", m)
	}
	if !m.RequiresDM {
		t.Error("layers changed must be RequiresDM")
	}
	if len(m.Payload) != 0 {
		t.Errorf("payload = %s, want none", m.Payload)
	}
}
