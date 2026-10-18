package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// roleAwareStanding stands in for the standing service, which owns what a
// viewer may know: below DM visibility there is no DM-only reason and, with the
// switch off, no block.
type roleAwareStanding struct {
	entities.StandingService
	switchOff bool
	fail      bool
	role      int
}

func (f *roleAwareStanding) View(_ context.Context, _, _ string, role int, _ string) (*entities.StandingView, error) {
	f.role = role
	if f.fail {
		return nil, errors.New("db down")
	}
	dm := permissions.CanSeeDmOnly(role)
	if !dm && f.switchOff {
		return &entities.StandingView{Hidden: true}, nil
	}
	reasons := []entities.StandingReasonView{{ID: "r1", Direction: "up", Text: "Saved the granary", When: "session 12"}}
	if dm {
		reasons = append([]entities.StandingReasonView{{ID: "r2", Direction: "down", Text: "Secretly paid off by the Duke", When: "session 11", DMOnly: true}}, reasons...)
	}
	off := f.switchOff
	v := &entities.StandingView{CanEdit: dm, Subjects: []entities.StandingSubjectView{{Key: "party", Name: "The party", Set: true, Level: 1, Word: "Wary", Total: len(reasons), Reasons: reasons}}}
	if dm {
		on := !off
		v.PlayersSee = &on
	}
	return v, nil
}

func TestStandingBlockVisibility(t *testing.T) {
	camp := &campaigns.Campaign{ID: "c1"}
	tests := []struct {
		name     string
		cc       *campaigns.CampaignContext
		off      bool
		fail     bool
		wantRole int
		contains []string
		absent   []string
		nothing  bool
	}{
		{name: "player, switch on", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true},
			wantRole: int(campaigns.RolePlayer), contains: []string{"Wary", "Saved the granary"},
			absent: []string{"Secretly paid off", "Add a reason", "data-dm-only"}},
		{name: "player, switch off: no block at all", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true},
			off: true, wantRole: int(campaigns.RolePlayer), nothing: true},
		{name: "owner, switch off: still there, with the amber edge", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true},
			off: true, wantRole: int(campaigns.RoleOwner), contains: []string{"Wary", "Secretly paid off", "Add a reason", "sg-dm", "data-sg-dmtag"}},
		{name: "co-Director sees what the owner sees", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true},
			wantRole: int(campaigns.RoleOwner), contains: []string{"Secretly paid off", "Add a reason"}},
		{name: "a failed read draws nothing", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true},
			fail: true, wantRole: int(campaigns.RoleOwner), nothing: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &roleAwareStanding{switchOff: tt.off, fail: tt.fail}
			reg := entities.NewBlockRegistry()
			registerStandingBlock(reg, fake)
			c := reg.Render(context.Background(), entities.BlockRenderContext{
				Block: entities.TemplateBlock{ID: "b1", Type: entities.StandingBlockType},
				CC:    tt.cc, Entity: &entities.Entity{ID: "e1", CampaignID: "c1", Name: "The Lantern Guild"}, UserID: "u1",
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
				t.Errorf("standing read for role %d, want %d", fake.role, tt.wantRole)
			}
			if tt.nothing {
				if strings.TrimSpace(out) != "" {
					t.Errorf("want no block, got %q", out)
				}
				return
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

func TestStandingBlockIsOnePerPage(t *testing.T) {
	reg := entities.NewBlockRegistry()
	registerStandingBlock(reg, &roleAwareStanding{})
	if !reg.IsSingleton(entities.StandingBlockType) {
		t.Error("Standing must be one per page: its reasons are a single sheet")
	}
}
