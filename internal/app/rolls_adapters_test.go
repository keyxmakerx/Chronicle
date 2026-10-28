package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolls"
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
	foundry := &ws.Client{UserID: "u-dm", Role: permissions.RoleOwner, Source: "foundry-module"}

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
