package app

import (
	"testing"

	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type spotlightCaptureBus struct{ msgs []*ws.Message }

func (b *spotlightCaptureBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

// The spotlight goes out GM-only, carries just the entity id, and is
// not a change-feed type (players and the feed never see it).
func TestNPCSpotlightPublisher_GMOnlyIDOnly(t *testing.T) {
	bus := &spotlightCaptureBus{}
	(&npcSpotlightPublisher{bus: bus}).PublishNPCSpotlight("camp-1", "npc-1")
	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	m := bus.msgs[0]
	if m.Type != ws.MsgNPCSpotlight || m.CampaignID != "camp-1" || m.ResourceID != "npc-1" {
		t.Errorf("message = %+v", m)
	}
	if !m.RequiresDM {
		t.Error("spotlight must be RequiresDM")
	}
	if len(m.Payload) != 0 {
		t.Errorf("payload = %s, want none", m.Payload)
	}
}
