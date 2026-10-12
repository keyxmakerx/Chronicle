package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type advTypeLister struct {
	entities.EntityService
}

func (advTypeLister) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	adv := entities.PresetCategoryAdventures
	return []entities.EntityType{
		{ID: 1, Slug: "npc", Name: "NPC", NamePlural: "NPCs", Enabled: true},
		{ID: 9, Slug: "adventures", Name: "Adventure", NamePlural: "Adventures", Enabled: true, PresetCategory: &adv},
	}, nil
}

// The relations graph's category filter is built from the viewer's categories,
// so a player must not be offered Adventures there.
func TestGraphTypeLister_HidesAdventuresFromPlayers(t *testing.T) {
	tests := []struct {
		name string
		role int
		want int
	}{
		{"anonymous", int(campaigns.RoleNone), 1},
		{"player", int(campaigns.RolePlayer), 1},
		{"scribe", int(campaigns.RoleScribe), 2},
		{"owner", int(campaigns.RoleOwner), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (&entityTypeListerForGraphAdapter{svc: advTypeLister{}}).ListEntityTypesForGraph(context.Background(), "camp-1", tt.role)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Fatalf("got %d categories, want %d: %+v", len(got), tt.want, got)
			}
			for _, s := range got {
				if s.Slug == "adventures" && tt.want == 1 {
					t.Error("Adventures offered to a viewer below the DM team")
				}
			}
		})
	}
}

type noAddons struct{}

func (noAddons) ListForCampaign(context.Context, string) ([]addons.CampaignAddon, error) {
	return nil, nil
}

// A member's own sidebar pins are chosen from the categories they can see, so
// Adventures must not be a row a player could pin.
func TestNavSectionsSource_HidesAdventuresFromPlayers(t *testing.T) {
	src := &navSectionsSource{entities: advTypeLister{}, addons: noAddons{}}
	tests := []struct {
		name    string
		role    campaigns.Role
		granted bool
		want    bool
	}{
		{"player", campaigns.RolePlayer, false, false},
		{"scribe", campaigns.RoleScribe, false, true},
		{"player with DM access", campaigns.RolePlayer, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tt.role, IsDmGranted: tt.granted, IsMember: true,
			}
			secs, err := src.NavSectionsFor(context.Background(), cc)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, s := range secs {
				for _, r := range s.Rows {
					if r.Label == "Adventures" || r.Key == "category:9" || r.Key == "cat-9" {
						found = true
					}
				}
			}
			if found != tt.want {
				t.Errorf("Adventures row present = %v, want %v", found, tt.want)
			}
		})
	}
}
