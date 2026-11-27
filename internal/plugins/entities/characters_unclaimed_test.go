package entities

// characters_unclaimed_test.go pins where a hero nobody has claimed yet shows
// on the Characters page: with The Party, its card saying "Not claimed yet",
// whether its page type is on the Party's list or the NPC list.

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// unclaimedSvc serves a fixed roster and records the page types asked for.
type unclaimedSvc struct {
	EntityService
	roster []Entity
	types  []int
	role   int
}

func (s *unclaimedSvc) ListRoster(_ context.Context, _ string, typeIDs []int, role int, _ string) ([]Entity, error) {
	s.types, s.role = typeIDs, role
	return s.roster, nil
}

func TestUnclaimedHeroes(t *testing.T) {
	u := "u1"
	svc := &unclaimedSvc{roster: []Entity{
		{ID: "free", Name: "Brannoc", Playable: true},
		{ID: "taken", Name: "Wren", Playable: true, OwnerUserID: &u},
		{ID: "owned", Name: "Sola", OwnerUserID: &u},
	}}
	h := NewHandler(svc)
	h.SetCharacterLists(&fakeCastLists{chars: []int{7}, npcs: []int{7, 9}})
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
	got := h.unclaimedHeroes(context.Background(), cc, "me", []int{7})
	if len(got) != 1 || got[0].Entity.ID != "free" || !got[0].Unclaimed || got[0].OwnerName != "" {
		t.Fatalf("unclaimed = %+v, want only Brannoc, marked unclaimed", got)
	}
	slices.Sort(svc.types)
	if !slices.Equal(svc.types, []int{7, 9}) {
		t.Errorf("types asked = %v, want the Party's and the NPC list's, once each", svc.types)
	}
	if svc.role != int(cc.VisibilityRole()) {
		t.Errorf("role = %d, want the viewer's visibility role", svc.role)
	}

	// No list at all asks for nothing.
	h2 := NewHandler(&unclaimedSvc{roster: svc.roster})
	if got := h2.unclaimedHeroes(context.Background(), cc, "me", nil); got != nil {
		t.Errorf("with no lists got %+v, want nothing", got)
	}
}

func TestCharactersContent_UnclaimedJoinsTheParty(t *testing.T) {
	party := []CastMember{{Entity: Entity{ID: "a", Name: "Wren"}, OwnerName: "Sam"}}
	unclaimed := []CastMember{{Entity: Entity{ID: "b", Name: "Brannoc Saltwake"}, Unclaimed: true}}
	tests := []struct {
		name string
		view CastView
		want map[string]bool
	}{
		{"after the claimed ones, saying so",
			CastView{ShowPlayers: true, PartyListed: true, Party: party, Unclaimed: unclaimed},
			map[string]bool{"Wren": true, "Sam": true, "Brannoc Saltwake": true, "Not claimed yet": true}},
		{"on its own, instead of the empty party",
			CastView{ShowPlayers: true, PartyListed: true, IsMember: true, ClaimTypeID: 3, Unclaimed: unclaimed},
			map[string]bool{"Brannoc Saltwake": true, "Not claimed yet": true, "No player characters yet": false}},
		{"a claimed card never says it",
			CastView{ShowPlayers: true, PartyListed: true, Party: party},
			map[string]bool{"Not claimed yet": false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderCastContent(t, tt.view)
			for needle, want := range tt.want {
				if got := strings.Contains(out, needle); got != want {
					t.Errorf("contains %q = %v, want %v", needle, got, want)
				}
			}
			if strings.Contains(out, "Wren") && strings.Contains(out, "Brannoc") && strings.Index(out, "Wren") > strings.Index(out, "Brannoc") {
				t.Error("an unclaimed hero comes after the claimed ones")
			}
		})
	}
}
