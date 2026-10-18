package app

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

// roleAwareSightings stands in for the sightings service, which owns what a
// viewer may know: a viewer without DM visibility gets only the nights seen.
type roleAwareSightings struct {
	sessions.SightingService
	seen    []sessions.Sighting
	suggest []sessions.Sighting
	viewer  permissions.Viewer
}

func (f *roleAwareSightings) SightingsFor(_ context.Context, _, _ string, v permissions.Viewer) (*sessions.Sightings, error) {
	f.viewer = v
	out := &sessions.Sightings{Seen: append([]sessions.Sighting{}, f.seen...)}
	if v.Role() >= permissions.RoleOwner {
		out.Suggestions = f.suggest
	}
	return out, nil
}

// The block is drawn from what the sightings service says for the viewer:
// a player whose page was never seen gets nothing at all, the Director keeps
// the line, and nobody short of DM visibility sees the Director's controls.
func TestLastSeenBlockVisibility(t *testing.T) {
	seen := []sessions.Sighting{{SessionID: "s1", SessionName: "The Drowned Bell", Date: "2026-09-12", Source: sessions.SightingSourceLink}}
	suggest := []sessions.Sighting{{SessionID: "s2", SessionName: "Smugglers' Tide", Date: "2026-09-19", Source: sessions.SightingSourceLink}}
	camp := &campaigns.Campaign{ID: "c1"}
	tests := []struct {
		name     string
		cc       *campaigns.CampaignContext
		seen     []sessions.Sighting
		wantRole int
		contains []string
		absent   []string
		nothing  bool
	}{
		{
			name: "player, page seen", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true},
			seen: seen, wantRole: int(campaigns.RolePlayer),
			contains: []string{"The Drowned Bell", "1 night"},
			absent:   []string{"Add a sighting", "data-dm-only", "to review", "Smugglers"},
		},
		{
			name: "player, never seen: no block", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsMember: true},
			wantRole: int(campaigns.RolePlayer), nothing: true,
		},
		{
			name: "owner, page seen", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true},
			seen: seen, wantRole: int(campaigns.RoleOwner),
			contains: []string{"The Drowned Bell", "Add a sighting", "1 to review"},
		},
		{
			name: "owner, never seen: the line stays", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RoleOwner, IsMember: true},
			wantRole: int(campaigns.RoleOwner), contains: []string{"Not seen yet", "Add a sighting", "1 to review"},
		},
		{
			name: "co-Director sees what the owner sees", cc: &campaigns.CampaignContext{Campaign: camp, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true},
			seen: seen, wantRole: int(campaigns.RoleOwner),
			contains: []string{"Add a sighting", "1 to review"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &roleAwareSightings{seen: tt.seen, suggest: suggest}
			reg := entities.NewBlockRegistry()
			registerLastSeenBlock(reg, fake)
			c := reg.Render(context.Background(), entities.BlockRenderContext{
				Block: entities.TemplateBlock{ID: "b1", Type: sessions.LastSeenBlockType},
				CC:    tt.cc, Entity: &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Brother Ansel"}, UserID: "u1",
			})
			if c == nil {
				t.Fatal("the block type is not registered")
			}
			var sb strings.Builder
			if err := c.Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if fake.viewer.Role() != tt.wantRole {
				t.Errorf("sightings read for role %d, want %d", fake.viewer.Role(), tt.wantRole)
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

func TestLastSeenBlockIsOnePerPageAndGatedByGameNights(t *testing.T) {
	reg := entities.NewBlockRegistry()
	registerLastSeenBlock(reg, &roleAwareSightings{})
	if !reg.IsSingleton(sessions.LastSeenBlockType) {
		t.Error("Last seen must be one per page: its list is a single sheet")
	}
	var meta *entities.BlockMeta
	for _, m := range reg.Types() {
		if m.Type == sessions.LastSeenBlockType {
			m := m
			meta = &m
		}
	}
	if meta == nil || meta.Addon != sessions.SessionsAddonSlug {
		t.Errorf("block meta = %+v, want gated by the game nights addon", meta)
	}
}
