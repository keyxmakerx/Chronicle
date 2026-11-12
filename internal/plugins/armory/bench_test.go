package armory

import (
	"context"
	"strings"
	"testing"
)

type craftGateStub struct {
	allow map[string]bool // by "user/character"
}

func (g craftGateStub) CanCraft(_ context.Context, _ string, characterID string, a Actor) bool {
	return g.allow[a.UserID+"/"+characterID]
}

// The bench is in the pack only for a viewer the crafting check lets through,
// and only on a page that asked for it; the first box is the bench, and the
// panel's own reload keeps asking.
func TestCharacterPanel_Bench(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		gate      CraftGate
		who       Actor
		bench     bool
		wantBench bool
	}{
		{"claiming player, asked", craftGateStub{map[string]bool{robin.UserID + "/c2": true}}, robin, true, true},
		{"claiming player, layout page did not ask", craftGateStub{map[string]bool{robin.UserID + "/c2": true}}, robin, false, false},
		{"viewer the crafting check refuses", craftGateStub{map[string]bool{robin.UserID + "/c2": true}}, gm, true, false},
		{"crafting off or degraded", nil, robin, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, pl := placesFx()
			svc := NewStashService(StashDeps{
				Repo: f.repo, Directory: f.dir, Visibility: f.vis, Actor: fakeActor{f.dir.fakeDir},
				Fields: f.fields, Relations: f.rels, Events: f.events, Handouts: f.hand, Notifier: f.notify,
				UserNames: fakeNames{"gm": "Greta", "u1": "Tess", "u2": "Robin", "u3": "Lee"},
				Shares:    f.shares, Auditor: f.audit, Places: pl,
			})
			if tc.gate != nil {
				svc.SetCraftGate(tc.gate)
			}
			f.svc = svc
			v, err := f.svc.CharacterPanel(ctx, "camp", tc.who, "c2")
			if err != nil || v == nil {
				t.Fatalf("%v %v", v, err)
			}
			v.Bench = v.CanCraft && tc.bench
			var sb strings.Builder
			if err := CharacterPanel(v, "tok").Render(ctx, &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			has := strings.Contains(out, `data-widget="hero-bench"`)
			if has != tc.wantBench {
				t.Fatalf("bench drawn = %v, want %v", has, tc.wantBench)
			}
			if !tc.wantBench {
				if strings.Contains(out, "data-hb-drape") || strings.Contains(out, "bench=1") {
					t.Error("no bench: no drape and no bench reload")
				}
				return
			}
			if strings.Index(out, `data-widget="hero-bench"`) > strings.Index(out, `data-hp-box="hand"`) {
				t.Error("On the bench comes first in the row")
			}
			for _, want := range []string{`data-hb-drape`, `data-entity-id="c2"`, `aria-controls="hb-drape-c2"`, `/armory/characters/c2/panel?bench=1`, "On the bench"} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %s", want)
				}
			}
		})
	}
}

// A hero carrying nothing still gets the bench, so a first project can start.
func TestCharacterPanel_BenchWithNoItems(t *testing.T) {
	v := &CharacterPanelView{CampaignID: "camp", Character: EntityRef{ID: "c9", Name: "Nell"}, CanCraft: true, Bench: true}
	var sb strings.Builder
	if err := CharacterPanel(v, "tok").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if !strings.Contains(out, `data-widget="hero-bench"`) || !strings.Contains(out, "No items carried.") {
		t.Error("empty pack still has the bench")
	}
	if strings.Contains(out, `data-hp-box=`) {
		t.Error("no item containers to draw")
	}
}
