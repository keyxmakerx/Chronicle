package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
	"github.com/keyxmakerx/chronicle/internal/plugins/quests"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// TestPublishers_PlayerReach pins, through the hub's own delivery gate, which
// live updates a plain player's socket receives. The hub treats a message with
// no chosen audience as DM-only, so every publisher of player-visible content
// has to say so; this is where a publisher that forgets shows up.
func TestPublishers_PlayerReach(t *testing.T) {
	adv := entities.PresetCategoryAdventures
	newMaps := func(bus ws.EventBus) *mapEventPublisherAdapter {
		return &mapEventPublisherAdapter{bus: bus, shadows: fixedShadowLookup{}, fog: fixedFogLookup{}}
	}
	tests := []struct {
		name string
		send func(bus ws.EventBus)
		want bool
	}{
		{"entity", func(b ws.EventBus) {
			(&entityEventPublisherAdapter{bus: b}).PublishEntityEvent("updated", "c1", "e1", &entities.Entity{ID: "e1"})
		}, false},
		{"relation", func(b ws.EventBus) {
			(&relationEventPublisherAdapter{bus: b}).PublishRelationEvent(relations.RelationEventCreated, &relations.Relation{CampaignID: "c1", SourceEntityID: "e1"})
		}, false},
		{"ordinary category", func(b ws.EventBus) {
			(&entityEventPublisherAdapter{bus: b}).PublishEntityTypeEvent("updated", "c1", &entities.EntityType{ID: 1})
		}, true},
		{"DM-only category", func(b ws.EventBus) {
			(&entityEventPublisherAdapter{bus: b}).PublishEntityTypeEvent("updated", "c1", &entities.EntityType{ID: 9, PresetCategory: &adv})
		}, false},
		{"party note", func(b ws.EventBus) {
			(&noteEventPublisherAdapter{bus: b}).PublishNoteEvent(notes.NoteEvent{Type: "updated", CampaignID: "c1", NoteID: "n1", Audience: notes.Audience{Everyone: true}})
		}, true},
		{"quest", func(b ws.EventBus) { (&questAnnouncerAdapter{bus: b}).QuestChanged("c1", "e1", 1, false) }, true},
		{"hidden quest", func(b ws.EventBus) { (&questAnnouncerAdapter{bus: b}).QuestChanged("c1", "e1", 1, true) }, false},
		{"boards", func(b ws.EventBus) { (&questAnnouncerAdapter{bus: b}).BoardsChanged("c1", quests.TypeHome(3), false) }, true},
		{"layer", func(b ws.EventBus) { newMaps(b).PublishLayerEvent("created", "c1", &maps.Layer{ID: "l1"}) }, true},
		{"hex", func(b ws.EventBus) { newMaps(b).PublishHexChanged("c1", "m1", 2, nil) }, true},
		{"pin", func(b ws.EventBus) {
			newMaps(b).PublishMarkerEvent("created", "c1", &maps.Marker{ID: "p1", MapID: "m1", Visibility: "everyone"})
		}, true},
		{"DM-only pin", func(b ws.EventBus) {
			newMaps(b).PublishMarkerEvent("created", "c1", &maps.Marker{ID: "p1", MapID: "m1", Visibility: "dm_only"})
		}, false},
		{"token", func(b ws.EventBus) { newMaps(b).PublishTokenEvent("updated", "c1", &maps.Token{ID: "t1", MapID: "m1"}) }, true},
		{"hidden token", func(b ws.EventBus) {
			newMaps(b).PublishTokenEvent("updated", "c1", &maps.Token{ID: "t1", MapID: "m1", IsHidden: true})
		}, false},
		{"token move", func(b ws.EventBus) { newMaps(b).PublishTokenPositionEvent("c1", "m1", "t1", 1, 1, false) }, true},
	}
	player := &ws.Client{UserID: "u-player", Role: permissions.RolePlayer, Source: "browser"}
	owner := &ws.Client{UserID: "u-owner", Role: permissions.RoleOwner, Source: "browser"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &noteCaptureBus{}
			tt.send(bus)
			if len(bus.msgs) == 0 {
				t.Fatal("nothing published")
			}
			// Map writes send an items-changed notice before the item event;
			// the item event is the last one.
			msg := bus.msgs[len(bus.msgs)-1]
			if got := msg.DeliversTo(player); got != tt.want {
				t.Errorf("player receives = %v, want %v", got, tt.want)
			}
			if !msg.DeliversTo(owner) {
				t.Error("the owner must receive every live update")
			}
		})
	}
}
