package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// shopRoomsFake and shopFrontsFake store what the armory's own normalizers
// accept, so these tests prove the blocks' JSON is what the shop room and
// the Shops street keep.
type shopRoomsFake struct{ rooms map[string]json.RawMessage }

func (f *shopRoomsFake) GetRoom(_ context.Context, _, id string, _ int, _ string) (json.RawMessage, error) {
	return f.rooms[id], nil
}
func (f *shopRoomsFake) SaveRoom(_ context.Context, _, id, _ string, raw []byte) (json.RawMessage, error) {
	l, err := armory.NormalizeShopRoomLayout(raw)
	if err == nil {
		f.rooms[id] = l
	}
	return l, err
}

type shopFrontsFake struct {
	armory.ShopStreetService
	fronts map[string]json.RawMessage
}

func (f *shopFrontsFake) Front(_ context.Context, _, id string) (json.RawMessage, bool, error) {
	if fr, ok := f.fronts[id]; ok {
		return fr, true, nil
	}
	def, _ := json.Marshal(armory.DefaultShopFront())
	return def, false, nil
}
func (f *shopFrontsFake) SaveFront(_ context.Context, _, id, _ string, raw []byte) (json.RawMessage, error) {
	fr, err := armory.NormalizeShopFront(raw)
	if err == nil {
		f.fronts[id] = fr
	}
	return fr, err
}

type shopPage struct{}

func (shopPage) GetBySlug(_ context.Context, campaignID, slug string) (*entities.Entity, error) {
	return &entities.Entity{ID: "s1", CampaignID: campaignID, Name: "The Rusty Anchor", Slug: slug, TypeSlug: "shop"}, nil
}

type ownsAll struct{}

func (ownsAll) IsOwner(context.Context, string, string) (bool, error) { return true, nil }

func TestAIShopLooksPassTheArmorysChecks(t *testing.T) {
	ctx := context.Background()
	rooms, fronts := &shopRoomsFake{rooms: map[string]json.RawMessage{}}, &shopFrontsFake{fronts: map[string]json.RawMessage{}}
	a := aiShopLooksAdapter{rooms: rooms, fronts: fronts}
	room := records.NewShopRoomKind(a, shopPage{}, ownsAll{})
	front := records.NewStorefrontKind(a, shopPage{}, ownsAll{})
	owner := records.Actor{UserID: "o", Role: 3}
	rec := func(kind string, f map[string]any) records.Record {
		f["shop"] = "The Rusty Anchor"
		return records.Record{Kind: kind, Action: records.ActionUpdate, Fields: f}
	}

	// A room nobody arranged, set up whole, then changed again.
	if err := room.Apply(ctx, "c", owner, rec("shop-room", map[string]any{"type": "apothecary", "setting": "tree", "size": "large", "mood": "fey", "manner": "nervous",
		"says": map[string]any{"hello": []any{"Oh! A customer."}, "refuse": "Not for your sort."}, "chatter": []any{"Mind the jars."}})); err != nil {
		t.Fatal(err)
	}
	var got armory.ShopRoomLayout
	_ = json.Unmarshal(rooms.rooms["s1"], &got)
	if got.RoomType != "apothecary" || got.Size != "l" || got.Furnish != "room" || got.Moments["refuse"][0] != "Not for your sort." || got.Lines[0] != "Mind the jars." {
		t.Fatalf("first room %+v", got)
	}
	if err := room.Apply(ctx, "c", owner, rec("shop-room", map[string]any{"says": map[string]any{"hello": "none"}})); err != nil {
		t.Fatal(err)
	}
	got = armory.ShopRoomLayout{}
	_ = json.Unmarshal(rooms.rooms["s1"], &got)
	if got.Moments["hello"] != nil || got.Furnish != "room" {
		t.Fatalf("second room %+v", got)
	}

	// Six lines for one moment is one more than the room keeps.
	six := []any{"a", "b", "c", "d", "e", "f"}
	if p := room.Plan(ctx, "c", owner, rec("shop-room", map[string]any{"says": map[string]any{"bye": six}})); p.Error == "" {
		t.Fatal("the room's limit on a moment's lines was not checked")
	}

	// A storefront from the street's default, every key written.
	if err := front.Apply(ctx, "c", owner, rec("storefront", map[string]any{"style": "painted", "colour": "#aa3344", "trim": "none", "awning": "plain", "awning_colour": "#112233",
		"sign": "banner", "sign_words": "Rope & Rum", "sign_picture": "bottle", "wares": []any{"bottle"}, "lamp": false, "closed": "curtains", "curtain_colour": "#445566"})); err != nil {
		t.Fatal(err)
	}
	var fr armory.ShopFront
	_ = json.Unmarshal(fronts.fronts["s1"], &fr)
	if fr.Style != "painted" || fr.Trim != "" || fr.Words != "Rope & Rum" || fr.Lamp || fr.CurtainColour != "#445566" || len(fr.Wares) != 1 {
		t.Fatalf("storefront %+v", fr)
	}
	if p := front.Plan(ctx, "c", owner, rec("storefront", map[string]any{"sign_words": "A name far too long for any sign board"})); p.Error == "" {
		t.Fatal("the sign's length was not checked")
	}
}
