package app

import (
	"context"
	"errors"
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

// TestPageChangedNudge pins who is told that an open page changed: players
// only for a page everyone may read, and the owner always gets the full event.
func TestPageChangedNudge(t *testing.T) {
	tests := []struct {
		name string
		e    *entities.Entity
		want bool
	}{
		{"shared page", &entities.Entity{ID: "e1"}, true},
		{"private page", &entities.Entity{ID: "e1", IsPrivate: true}, false},
		{"custom-access page", &entities.Entity{ID: "e1", Visibility: entities.VisibilityCustom}, false},
		{"DM-only category", &entities.Entity{ID: "e1", TypeDMOnly: true}, false},
	}
	player := &ws.Client{UserID: "u-player", Role: permissions.RolePlayer, Source: "browser"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &noteCaptureBus{}
			(&entityEventPublisherAdapter{bus: bus}).PublishEntityEvent("updated", "c1", "e1", tt.e)
			var nudge *ws.Message
			for _, m := range bus.msgs {
				if m.Type == ws.MsgPageChanged {
					nudge = m
				}
			}
			if (nudge != nil) != tt.want {
				t.Fatalf("nudge sent = %v, want %v", nudge != nil, tt.want)
			}
			if nudge != nil {
				if !nudge.DeliversTo(player) {
					t.Error("a player must receive the nudge for a shared page")
				}
				if len(nudge.Payload) != 0 {
					t.Error("the nudge must carry no content")
				}
			}
			if last := bus.msgs[len(bus.msgs)-1]; last.Type != ws.MsgEntityUpdated || last.DeliversTo(player) {
				t.Error("the full entity event stays last and DM-only")
			}
		})
	}
}

// fixedPages answers a shop page lookup with one page, or none.
type fixedPages struct{ e *entities.Entity }

func (f fixedPages) GetByID(_ context.Context, _ string) (*entities.Entity, error) {
	if f.e == nil {
		return nil, errors.New("not found")
	}
	return f.e, nil
}

// TestShopChangedNudge pins who is told that an open shop's wares changed: a
// write to a shop's listing nudges players only for a shop page everyone may
// read and a listing that isn't GM-only, the owner always, and any other
// relation nobody.
func TestShopChangedNudge(t *testing.T) {
	shared := &entities.Entity{ID: "s1", CampaignID: "c1"}
	tests := []struct {
		name   string
		page   *entities.Entity
		rel    relations.Relation
		sent   bool
		player bool
	}{
		{"shared shop", shared, relations.Relation{CampaignID: "c1", SourceEntityID: "s1", RelationType: "sells"}, true, true},
		{"GM-only listing", shared, relations.Relation{CampaignID: "c1", SourceEntityID: "s1", RelationType: "sells", DmOnly: true}, true, false},
		{"private shop", &entities.Entity{ID: "s1", CampaignID: "c1", IsPrivate: true}, relations.Relation{CampaignID: "c1", SourceEntityID: "s1", RelationType: "sells"}, true, false},
		{"another campaign's page", &entities.Entity{ID: "s1", CampaignID: "c2"}, relations.Relation{CampaignID: "c1", SourceEntityID: "s1", RelationType: "sells"}, false, false},
		{"missing page", nil, relations.Relation{CampaignID: "c1", SourceEntityID: "s1", RelationType: "sells"}, false, false},
		{"not a listing", shared, relations.Relation{CampaignID: "c1", SourceEntityID: "s1", RelationType: "Has Item"}, false, false},
	}
	player := &ws.Client{UserID: "u-player", Role: permissions.RolePlayer, Source: "browser"}
	owner := &ws.Client{UserID: "u-owner", Role: permissions.RoleOwner, Source: "browser"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &noteCaptureBus{}
			rel := tt.rel
			(&relationEventPublisherAdapter{bus: bus, pages: fixedPages{tt.page}}).PublishRelationEvent(relations.RelationEventMetadataUpdated, &rel)
			var nudge *ws.Message
			for _, m := range bus.msgs {
				if m.Type == ws.MsgShopChanged {
					nudge = m
				}
			}
			if (nudge != nil) != tt.sent {
				t.Fatalf("nudge sent = %v, want %v", nudge != nil, tt.sent)
			}
			if last := bus.msgs[len(bus.msgs)-1]; last.Type != ws.MsgRelationMetadataUpdated {
				t.Errorf("the relation event must stay the last one published; got %s", last.Type)
			}
			if nudge == nil {
				return
			}
			if nudge.ResourceID != "s1" || nudge.CampaignID != "c1" || len(nudge.Payload) != 0 {
				t.Errorf("nudge = %s %s %s", nudge.CampaignID, nudge.ResourceID, nudge.Payload)
			}
			if got := nudge.DeliversTo(player); got != tt.player {
				t.Errorf("player receives = %v, want %v", got, tt.player)
			}
			if !nudge.DeliversTo(owner) {
				t.Error("the owner must be nudged")
			}
		})
	}
}
