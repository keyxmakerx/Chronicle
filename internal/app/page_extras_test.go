package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeExtrasSettings struct{ vals map[string]string }

func (f *fakeExtrasSettings) Get(_ context.Context, k string) (string, error) {
	if v, ok := f.vals[k]; ok {
		return v, nil
	}
	return "", apperror.NewNotFound("setting not found")
}

func (f *fakeExtrasSettings) Set(_ context.Context, k, v string) error {
	f.vals[k] = v
	return nil
}

type fakeExtrasCampaigns struct{ ids []string }

func (f fakeExtrasCampaigns) ListAll(_ context.Context, opts campaigns.ListOptions) ([]campaigns.Campaign, int, error) {
	var out []campaigns.Campaign
	start := (opts.Page - 1) * opts.PerPage
	for i := start; i < len(f.ids) && i < start+opts.PerPage; i++ {
		out = append(out, campaigns.Campaign{ID: f.ids[i]})
	}
	return out, len(f.ids), nil
}

type fakeExtrasAddons struct{ armory map[string]bool }

func (f fakeExtrasAddons) IsEnabledForCampaign(_ context.Context, id, slug string) (bool, error) {
	return slug == "armory" && f.armory[id], nil
}

type fakeExtrasEntities struct {
	types map[string][]entities.EntityType
	plans map[string]map[int]entities.PageExtrasPlan
}

func (f *fakeExtrasEntities) GetEntityTypes(_ context.Context, id string) ([]entities.EntityType, error) {
	return f.types[id], nil
}

func (f *fakeExtrasEntities) PlacePageExtras(_ context.Context, id string, plans map[int]entities.PageExtrasPlan) (int, error) {
	f.plans[id] = plans
	return 1, nil
}

func TestPlacePageExtrasOnce(t *testing.T) {
	parent := 1
	types := []entities.EntityType{
		{ID: 1, Slug: "character", Name: "Character", Enabled: true},
		{ID: 2, Slug: "npc", Name: "NPC", ParentTypeID: &parent, Enabled: true},
		{ID: 3, Slug: "location", Name: "Location", Enabled: true},
	}
	st := &fakeExtrasSettings{vals: map[string]string{}}
	ents := &fakeExtrasEntities{
		types: map[string][]entities.EntityType{"c1": types, "c2": types},
		plans: map[string]map[int]entities.PageExtrasPlan{},
	}
	addons := fakeExtrasAddons{armory: map[string]bool{"c1": true}}

	n, err := placePageExtrasOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1", "c2"}}, addons, ents)
	if err != nil || n != 2 {
		t.Fatalf("first run: n=%d err=%v, want 2 layouts and no error", n, err)
	}
	if st.vals[pageExtrasPlacedKey] != "1" {
		t.Fatalf("the run was not recorded")
	}
	if p := ents.plans["c1"][2]; !p.SystemPanels || !p.CharacterItems {
		t.Errorf("NPC type with the armory on: plan %+v, want system panels and items", p)
	}
	if p := ents.plans["c2"][2]; !p.SystemPanels || p.CharacterItems {
		t.Errorf("NPC type with the armory off: plan %+v, want system panels only", p)
	}
	if p := ents.plans["c1"][3]; p.SystemPanels || p.CharacterItems {
		t.Errorf("location type: plan %+v, want nothing beyond the page pieces", p)
	}

	// A second boot changes nothing, so a block the owner removed stays removed.
	ents.plans = map[string]map[int]entities.PageExtrasPlan{}
	if n, err := placePageExtrasOnce(context.Background(), st, fakeExtrasCampaigns{ids: []string{"c1"}}, addons, ents); err != nil || n != 0 || len(ents.plans) != 0 {
		t.Errorf("second run: n=%d err=%v plans=%v, want a no-op", n, err, ents.plans)
	}
}
