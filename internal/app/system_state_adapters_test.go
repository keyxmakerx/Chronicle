package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// The update goes out GM-only and names the system and key without any state.
func TestSystemStatePublisher_GMOnlyNoContent(t *testing.T) {
	bus := &spotlightCaptureBus{}
	(&systemStatePublisher{bus: bus}).PublishSystemStateUpdated("camp-1", "npc-1", "drawsteel", "negotiation")
	if len(bus.msgs) != 1 {
		t.Fatalf("published %d messages, want 1", len(bus.msgs))
	}
	m := bus.msgs[0]
	if m.Type != ws.MsgSystemStateUpdated || string(m.Type) != "system_state.updated" {
		t.Errorf("type = %q", m.Type)
	}
	if m.CampaignID != "camp-1" || m.ResourceID != "npc-1" {
		t.Errorf("message = %+v", m)
	}
	if !m.RequiresDM {
		t.Error("must be RequiresDM")
	}
	var payload map[string]string
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 2 || payload["systemId"] != "drawsteel" || payload["key"] != "negotiation" {
		t.Errorf("payload = %v, want only systemId and key", payload)
	}
}

func TestSystemStateSystemChecker(t *testing.T) {
	tests := []struct {
		name string
		sys  systems.System
		id   string
		want bool
	}{
		{"matching system", fakeSystem{&systems.SystemManifest{ID: "drawsteel"}}, "drawsteel", true},
		{"another system id", fakeSystem{&systems.SystemManifest{ID: "drawsteel"}}, "dnd5e", false},
		{"no system enabled", nil, "drawsteel", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&systemStateSystemChecker{systems: fakeEnabled{tt.sys}}).IsSystemEnabled(context.Background(), "camp-1", tt.id)
			if err != nil || got != tt.want {
				t.Errorf("got %v, %v want %v", got, err, tt.want)
			}
		})
	}
}
