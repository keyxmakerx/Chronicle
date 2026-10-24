package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeCreaturePanelEntities struct {
	types  map[string][]entities.EntityType
	placed map[string][]int
}

func (f *fakeCreaturePanelEntities) GetEntityTypes(_ context.Context, id string) ([]entities.EntityType, error) {
	return f.types[id], nil
}

func (f *fakeCreaturePanelEntities) PlaceSystemPanels(_ context.Context, id string, ids []int) (int, error) {
	f.placed[id] = ids
	return len(ids), nil
}

func TestPlaceCreaturePanelsOnce(t *testing.T) {
	cat := func(s string) *string { return &s }
	parent := 2
	withCreatures := []entities.EntityType{
		{ID: 1, Slug: "character", Enabled: true},
		{ID: 2, Slug: "drawsteel-creature", PresetCategory: cat("creature"), Enabled: true},
		{ID: 3, Slug: "dragon", ParentTypeID: &parent, Enabled: true},
		{ID: 4, Slug: "dnd5e-monster", Enabled: false},
	}
	plain := []entities.EntityType{{ID: 9, Slug: "location", Enabled: true}}
	st := &fakeExtrasSettings{vals: map[string]string{}}
	ents := &fakeCreaturePanelEntities{
		types:  map[string][]entities.EntityType{"c1": withCreatures, "c2": plain},
		placed: map[string][]int{},
	}

	n, err := placeCreaturePanelsOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1", "c2"}}, ents)
	if err != nil {
		t.Fatalf("placeCreaturePanelsOnce: %v", err)
	}
	if n != 2 || !slices.Equal(ents.placed["c1"], []int{2, 3}) {
		t.Fatalf("placed %d, c1 = %v; want the creature type and its sub-type only", n, ents.placed["c1"])
	}
	if _, ok := ents.placed["c2"]; ok {
		t.Error("a campaign with no creature types was touched")
	}
	if st.vals[creaturePanelsPlacedKey] != "1" {
		t.Error("run was not recorded")
	}

	ents.placed = map[string][]int{}
	if n, err := placeCreaturePanelsOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1"}}, ents); err != nil || n != 0 || len(ents.placed) != 0 {
		t.Fatalf("second run = %d, %v, %v; want nothing", n, err, ents.placed)
	}
}

type datedCampaigns struct{ list []campaigns.Campaign }

func (f datedCampaigns) ListAll(_ context.Context, _ campaigns.ListOptions) ([]campaigns.Campaign, int, error) {
	return f.list, len(f.list), nil
}

func TestPlaceCreaturePanelsOnNewCampaigns(t *testing.T) {
	creature := []entities.EntityType{{ID: 5, Slug: "creature", Enabled: true}}
	st := &fakeExtrasSettings{vals: map[string]string{creaturePanelsPlacedKey: "1"}}
	ents := &fakeCreaturePanelEntities{
		types:  map[string][]entities.EntityType{"old": creature, "new": creature},
		placed: map[string][]int{},
	}
	camps := datedCampaigns{list: []campaigns.Campaign{
		{ID: "old", CreatedAt: creaturePanelsSince.Add(-time.Hour)},
		{ID: "new", CreatedAt: creaturePanelsSince.Add(72 * time.Hour)},
	}}

	n, err := placeCreaturePanelsOnNewCampaigns(context.Background(), st, camps, ents)
	if err != nil {
		t.Fatalf("placeCreaturePanelsOnNewCampaigns: %v", err)
	}
	if n != 1 || !slices.Equal(ents.placed["new"], []int{5}) {
		t.Fatalf("placed %d, new = %v; want the newer campaign's creature type", n, ents.placed["new"])
	}
	if _, ok := ents.placed["old"]; ok {
		t.Error("a campaign the first sweep covered was touched again")
	}
	if st.vals[creaturePanelsNewCampaignsKey] != "1" {
		t.Error("run was not recorded")
	}
	ents.placed = map[string][]int{}
	if n, _ := placeCreaturePanelsOnNewCampaigns(context.Background(), st, camps, ents); n != 0 || len(ents.placed) != 0 {
		t.Fatalf("second run placed %d; want nothing", n)
	}
}
