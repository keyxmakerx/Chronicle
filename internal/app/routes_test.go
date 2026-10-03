package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// captureBus implements ws.EventBus by storing the most recent Publish.
// Used by the adapter tests to pin the eventType → MessageType mapping.
type captureBus struct {
	last *ws.Message
}

func (b *captureBus) Publish(msg *ws.Message) { b.last = msg }

// TestPublishLayerEvent_RoutesByEventType locks in the contract that every
// layer-lifecycle event gets its own MessageType instead of flattening to
// MsgLayerUpdated, which would force Foundry into a pessimistic refetch.
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
			a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}}
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
	a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}}
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
			a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}}
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
	a := &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}}
	a.PublishFogEvent("gibberish", "camp-1", "map-1", nil)
	if bus.last != nil {
		t.Errorf("expected unknown eventType to be dropped; got %v", bus.last)
	}
}

// TestPublishRelationEvent pins the message type per event, the source
// entity as resourceId (a character's inventory is its own relations) and
// the DM-only gate: a relation can name a private entity.
func TestPublishRelationEvent(t *testing.T) {
	rel := &relations.Relation{ID: 7, CampaignID: "camp-1", SourceEntityID: "hero", TargetEntityID: "item", RelationType: "Has Item"}
	for event, want := range map[string]ws.MessageType{
		relations.RelationEventCreated:         ws.MsgRelationCreated,
		relations.RelationEventDeleted:         ws.MsgRelationDeleted,
		relations.RelationEventMetadataUpdated: ws.MsgRelationMetadataUpdated,
	} {
		t.Run(event, func(t *testing.T) {
			bus := &captureBus{}
			(&relationEventPublisherAdapter{bus: bus}).PublishRelationEvent(event, rel)
			if bus.last == nil {
				t.Fatal("expected Publish to be called")
			}
			if bus.last.Type != want || bus.last.ResourceID != "hero" || bus.last.CampaignID != "camp-1" {
				t.Errorf("got %s %s %s", bus.last.Type, bus.last.CampaignID, bus.last.ResourceID)
			}
			if !bus.last.RequiresDM {
				t.Error("relation message must set RequiresDM")
			}
		})
	}
	bus := &captureBus{}
	(&relationEventPublisherAdapter{bus: bus}).PublishRelationEvent("gibberish", rel)
	(&relationEventPublisherAdapter{bus: bus}).PublishRelationEvent(relations.RelationEventCreated, &relations.Relation{})
	if bus.last != nil {
		t.Errorf("unknown event or missing campaign must be dropped; got %v", bus.last)
	}
}

// TestPublishEntityEvent_DMOnly pins that every entity message is gated to
// DM-equivalent sockets: the payload is the unfiltered entity, so a player's
// socket must never receive it.
func TestPublishEntityEvent_DMOnly(t *testing.T) {
	for _, event := range []string{"created", "updated", "deleted"} {
		t.Run(event, func(t *testing.T) {
			bus := &captureBus{}
			a := &entityEventPublisherAdapter{bus: bus}
			a.PublishEntityEvent(event, "camp-1", "ent-1", &entities.Entity{ID: "ent-1", IsPrivate: true})
			if bus.last == nil {
				t.Fatal("expected Publish to be called")
			}
			if !bus.last.RequiresDM {
				t.Errorf("event %q: entity message must set RequiresDM", event)
			}
		})
	}
}
