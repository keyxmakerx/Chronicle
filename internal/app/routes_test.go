package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

// captureBus implements ws.EventBus by storing the most recent Publish.
// Used by the adapter tests to pin the eventType → MessageType mapping.
type captureBus struct {
	last *ws.Message
}

func (b *captureBus) Publish(msg *ws.Message) { b.last = msg }

// TestPublishLayerEvent_RoutesByEventType locks in the contract that
// every layer-lifecycle event gets its own MessageType, instead of
// flattening to MsgLayerUpdated. Pre-C-MAP-EVT the adapter silently
// dropped created/deleted into "updated," which forced Foundry into a
// pessimistic refetch. Test guards against the flatten regressing.
func TestPublishLayerEvent_RoutesByEventType(t *testing.T) {
	cases := []struct {
		event   string
		wantMsg ws.MessageType
	}{
		{"created", ws.MsgLayerCreated},
		{"updated", ws.MsgLayerUpdated},
		{"deleted", ws.MsgLayerDeleted},
	}
	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			bus := &captureBus{}
			a := &mapEventPublisherAdapter{bus: bus}
			a.PublishLayerEvent(tc.event, "camp-1", &maps.Layer{ID: "layer-1"})
			if bus.last == nil {
				t.Fatal("expected Publish to be called")
			}
			if bus.last.Type != tc.wantMsg {
				t.Errorf("event %q: want type %q, got %q", tc.event, tc.wantMsg, bus.last.Type)
			}
		})
	}
}

// TestPublishLayerEvent_UnknownEventDropped ensures an unrecognized
// lifecycle string doesn't accidentally generate a stray message —
// the adapter should drop it (silent return) rather than guessing.
func TestPublishLayerEvent_UnknownEventDropped(t *testing.T) {
	bus := &captureBus{}
	a := &mapEventPublisherAdapter{bus: bus}
	a.PublishLayerEvent("gibberish", "camp-1", &maps.Layer{ID: "layer-1"})
	if bus.last != nil {
		t.Errorf("expected unknown eventType to be dropped; got %v", bus.last)
	}
}

// TestPublishFogEvent_RoutesByEventType mirrors the layer test for fog.
// Includes the "reset" alias since DrawingService.ResetFog emits that
// event string (see drawing_service.go's ResetFog) — the adapter maps
// it onto MsgFogUpdated so clients treat a full reset as a generic
// update they can refetch on.
func TestPublishFogEvent_RoutesByEventType(t *testing.T) {
	cases := []struct {
		event   string
		wantMsg ws.MessageType
	}{
		{"created", ws.MsgFogCreated},
		{"updated", ws.MsgFogUpdated},
		{"deleted", ws.MsgFogDeleted},
		{"reset", ws.MsgFogUpdated},
	}
	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			bus := &captureBus{}
			a := &mapEventPublisherAdapter{bus: bus}
			a.PublishFogEvent(tc.event, "camp-1", "map-1", &maps.FogRegion{ID: "fog-1"})
			if bus.last == nil {
				t.Fatal("expected Publish to be called")
			}
			if bus.last.Type != tc.wantMsg {
				t.Errorf("event %q: want type %q, got %q", tc.event, tc.wantMsg, bus.last.Type)
			}
			if !bus.last.RequiresDM {
				t.Errorf("fog event should be RequiresDM=true (mask shape is sensitive); got false")
			}
		})
	}
}

// TestPublishFogEvent_UnknownEventDropped mirrors the layer guard.
func TestPublishFogEvent_UnknownEventDropped(t *testing.T) {
	bus := &captureBus{}
	a := &mapEventPublisherAdapter{bus: bus}
	a.PublishFogEvent("gibberish", "camp-1", "map-1", nil)
	if bus.last != nil {
		t.Errorf("expected unknown eventType to be dropped; got %v", bus.last)
	}
}
