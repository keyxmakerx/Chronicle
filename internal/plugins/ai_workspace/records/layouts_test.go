package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeLayouts is one Characters category with an NPC main layout and a Hero
// layout claimed characters start on.
type fakeLayouts struct{ chosen map[string]string }

func (f fakeLayouts) ForCampaign(context.Context, string) map[int]entities.PageLayouts {
	return map[int]entities.PageLayouts{1: {
		MainName: "NPC",
		Claimed:  "hero",
		Extra:    []entities.PageLayout{{Key: "hero", Name: "Hero"}},
	}}
}
func (f fakeLayouts) PageKeys(context.Context, string) map[string]string { return f.chosen }

func layoutWorld() *Lookups {
	owner := "u1"
	mk := func(id, name string, claimed bool) entities.Entity {
		e := page(id, name, "Character", false)
		e.EntityTypeID = 1
		if claimed {
			e.OwnerUserID = &owner
		}
		return e
	}
	return &Lookups{
		Pages: &fakePages{
			all:   []entities.Entity{mk("aric", "Aric", true), mk("bram", "Bram", false), mk("cass", "Cass", false), page("keep", "The Keep", "Location", false)},
			types: []entities.EntityType{{ID: 1, Name: "Character", NamePlural: "Characters"}},
		},
		Layouts: fakeLayouts{chosen: map[string]string{"cass": "hero"}},
	}
}

// The AI can tell a hero from an NPC in one Characters list: the outline
// groups the category's pages by the layout each wears, and one page names
// its layout. Players are told neither, since layout names are the
// Director's own words.
func TestLookups_LayoutsSplitOneCategory(t *testing.T) {
	tests := []struct {
		name      string
		a         Actor
		wantIndex []string
		wantNot   []string
	}{
		{"a Director", owner,
			[]string{"**Characters** (3), by layout:", "Hero layout (2): Aric, Cass;", "NPC layout (1): Bram;", "**Locations** (1): The Keep"}, nil},
		{"a player", player,
			[]string{"**Characters** (3): Aric, Bram, Cass"}, []string{"layout"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := layoutWorld().PageIndex(context.Background(), camp, tt.a)
			for _, w := range tt.wantIndex {
				if !strings.Contains(idx, w) {
					t.Errorf("outline missing %q:\n%s", w, idx)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(idx, w) {
					t.Errorf("outline should not mention %q:\n%s", w, idx)
				}
			}
		})
	}

	ans := layoutWorld().Answer(context.Background(), camp, owner, []Record{lookup("page", map[string]any{"name": "Aric"})})
	if ans[0].Error != "" || !strings.Contains(ans[0].Text, "Hero layout") {
		t.Errorf("read_page does not name the layout: %+v", ans[0])
	}
	ans = layoutWorld().Answer(context.Background(), camp, player, []Record{lookup("page", map[string]any{"name": "Aric"})})
	if strings.Contains(ans[0].Text, "layout") {
		t.Errorf("a player's read names the layout: %s", ans[0].Text)
	}
}
