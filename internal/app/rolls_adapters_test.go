package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
	"github.com/keyxmakerx/chronicle/internal/systems"
	ws "github.com/keyxmakerx/chronicle/internal/websocket"
)

type rollsBus struct{ msgs []*ws.Message }

func (b *rollsBus) Publish(m *ws.Message) { b.msgs = append(b.msgs, m) }

// TestRollsPublisherAudience runs each audience through the hub's own
// per-recipient gate, so a roll reaches exactly the sockets its mode allows.
func TestRollsPublisherAudience(t *testing.T) {
	owner := &ws.Client{UserID: "u-dm", Role: permissions.RoleOwner, Source: "browser"}
	coDM := &ws.Client{UserID: "u-co", Role: permissions.RolePlayer, IsDmGranted: true, Source: "browser"}
	roller := &ws.Client{UserID: "u-roller", Role: permissions.RolePlayer, Source: "browser"}
	other := &ws.Client{UserID: "u-other", Role: permissions.RolePlayer, Source: "browser"}
	foundry := &ws.Client{UserID: "u-dm", Role: permissions.RoleOwner, Source: foundry_vtt.ModuleSource}

	tests := []struct {
		name string
		a    rolls.Audience
		want map[*ws.Client]bool
		sent bool
	}{
		{"everyone", rolls.Audience{Everyone: true}, map[*ws.Client]bool{owner: true, coDM: true, roller: true, other: true, foundry: true}, true},
		{"directors only", rolls.Audience{DirectorsOnly: true}, map[*ws.Client]bool{owner: true, coDM: true, foundry: true}, true},
		{"directors and the roller", rolls.Audience{Users: []string{"u-roller"}}, map[*ws.Client]bool{owner: true, coDM: true, roller: true, foundry: true}, true},
		{"the roller alone", rolls.Audience{Users: []string{"u-roller"}, Strict: true}, map[*ws.Client]bool{roller: true}, true},
		{"nobody", rolls.Audience{}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &rollsBus{}
			(&rollsPublisher{bus: bus}).RollMade("c1", rolls.View{ID: "r1"}, tt.a)
			if !tt.sent {
				if len(bus.msgs) != 0 {
					t.Fatalf("sent %d messages to nobody", len(bus.msgs))
				}
				return
			}
			if len(bus.msgs) != 1 || bus.msgs[0].Type != ws.MsgRollMade || bus.msgs[0].ResourceID != "r1" {
				t.Fatalf("msgs = %+v", bus.msgs)
			}
			for _, c := range []*ws.Client{owner, coDM, roller, other, foundry} {
				if got := bus.msgs[0].DeliversTo(c); got != tt.want[c] {
					t.Errorf("%s (%s) receives = %v, want %v", c.UserID, c.Source, got, tt.want[c])
				}
			}
		})
	}
}

type powerSystems struct{ sys systems.System }

func (p powerSystems) EnabledSystem(context.Context, string) systems.System { return p.sys }

type powerHeroes struct {
	entities.EntityService
	mine []entities.Entity
}

func (p powerHeroes) ListByOwner(_ context.Context, _, _ string) ([]entities.Entity, error) {
	return p.mine, nil
}

// The Table's power roll comes from the game system's declaration and the
// viewer's own claimed hero; a campaign with none shows no line.
func TestRollsPowerAdapter(t *testing.T) {
	ctx := context.Background()
	decl := fakeSystem{manifest: &systems.SystemManifest{Crafting: &systems.CraftingRules{
		Dice: "2d10", Edge: 2, Bane: -2, Tiers: []int{11, 16},
		Characteristics: []systems.CraftingStat{{Key: "might", Label: "Might"}, {Key: "agility", Label: "Agility"}},
	}}}
	hero := entities.Entity{Name: "Sivin", FieldsData: map[string]any{"might": float64(1), "agility": "+2"}}

	t.Run("no system", func(t *testing.T) {
		got, err := (&rollsPowerAdapter{sys: powerSystems{}}).PowerRoll(ctx, "c1", "u1")
		if err != nil || got.Declared {
			t.Fatalf("got %+v, %v", got, err)
		}
	})
	t.Run("a system with no declaration", func(t *testing.T) {
		got, _ := (&rollsPowerAdapter{sys: powerSystems{sys: fakeSystem{manifest: &systems.SystemManifest{}}}}).PowerRoll(ctx, "c1", "u1")
		if got.Declared {
			t.Fatalf("declared without a block: %+v", got)
		}
	})
	t.Run("a hero's numbers", func(t *testing.T) {
		a := &rollsPowerAdapter{sys: powerSystems{sys: decl}, entities: powerHeroes{mine: []entities.Entity{hero}}}
		got, err := a.PowerRoll(ctx, "c1", "u1")
		if err != nil || !got.Declared || got.Hero != "Sivin" || len(got.Characteristics) != 2 {
			t.Fatalf("got %+v, %v", got, err)
		}
		if v := got.Characteristics[0].Value; v == nil || *v != 1 {
			t.Errorf("might = %v", v)
		}
		if v := got.Characteristics[1].Value; v == nil || *v != 2 {
			t.Errorf("agility = %v", v)
		}
	})
	t.Run("no hero keeps the line without numbers", func(t *testing.T) {
		got, _ := (&rollsPowerAdapter{sys: powerSystems{sys: decl}, entities: powerHeroes{}}).PowerRoll(ctx, "c1", "u1")
		if !got.Declared || got.Hero != "" || got.Characteristics[0].Value != nil {
			t.Fatalf("got %+v", got)
		}
	})
}
