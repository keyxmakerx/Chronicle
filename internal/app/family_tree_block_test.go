package app

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// roleAwareFamily stands in for the family service, which owns what a viewer
// may know: it answers with the Director's half only to a DM-visibility role.
type roleAwareFamily struct {
	role int
}

func (f *roleAwareFamily) Tree(_ context.Context, _, entityID string, role int, _ string) (*entities.FamilyTree, error) {
	f.role = role
	t := &entities.FamilyTree{PageID: entityID, People: []entities.FamilyPerson{
		{ID: entityID, Name: "Maud", Read: true},
		{ID: "p", Name: "Ione", Read: true},
	}}
	t.People[0].Parents = []entities.FamilyParent{{ID: "p", Kind: entities.FamilyKindBirth}}
	if role >= int(permissions.RoleOwner) {
		t.People = append(t.People, entities.FamilyPerson{ID: "h", Name: "Hidden Half", DMOnly: true})
		t.People[0].Parents = append(t.People[0].Parents, entities.FamilyParent{ID: "h", Kind: entities.FamilyKindBirth})
	}
	return t, nil
}

// The block asks the service for the viewer's own role (a co-Director counts
// as a Director), and what is drawn follows the answer.
func TestFamilyTreeBlockFollowsTheViewersRole(t *testing.T) {
	camp := &campaigns.Campaign{ID: "c1"}
	tests := []struct {
		name     string
		cc       *campaigns.CampaignContext
		wantRole int
		contains []string
		absent   []string
	}{
		{"player", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true}, int(campaigns.RolePlayer), []string{"Ione", `data-dm="0"`}, []string{"Hidden Half", "data-dm-only", "data-fm-edit", "family/candidates"}},
		{"owner", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true}, int(campaigns.RoleOwner), []string{"Hidden Half", `data-dm="1"`, "data-fm-edit"}, nil},
		{"co-Director", &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true}, int(campaigns.RoleOwner), []string{"Hidden Half", `data-dm="1"`, "data-fm-edit"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &roleAwareFamily{}
			reg := entities.NewBlockRegistry()
			registerFamilyTreeBlock(reg, fake)
			c := reg.Render(context.Background(), entities.BlockRenderContext{
				Block: entities.TemplateBlock{ID: "b1", Type: entities.FamilyTreeBlockType},
				CC:    tt.cc, Entity: &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Maud"}, UserID: "u1",
			})
			if c == nil {
				t.Fatal("the block type is not registered")
			}
			var sb strings.Builder
			if err := c.Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if fake.role != tt.wantRole {
				t.Errorf("tree read for role %d, want %d", fake.role, tt.wantRole)
			}
			for _, w := range tt.contains {
				if !strings.Contains(out, w) {
					t.Errorf("block lacks %q", w)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(out, a) {
					t.Errorf("block contains %q", a)
				}
			}
		})
	}
}

func TestFamilyTreeBlockIsOnePerPage(t *testing.T) {
	reg := entities.NewBlockRegistry()
	registerFamilyTreeBlock(reg, &roleAwareFamily{})
	if !reg.IsSingleton(entities.FamilyTreeBlockType) {
		t.Error("the Family tree must be one per page: its tree is a single sheet")
	}
}
