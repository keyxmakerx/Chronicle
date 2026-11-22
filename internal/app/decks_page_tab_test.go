package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// heroHandSvc answers HeroHand by who asks, as the real service does: the
// hero's player and a Director read a hand, anyone else only what a deck lets
// them see. It records the viewer so a test can prove the tab asks as the
// viewer and not as someone else.
type heroHandSvc struct {
	decks.Service
	own    string
	err    error
	empty  bool
	viewer decks.Viewer
	called bool
}

func (s *heroHandSvc) HeroHand(_ context.Context, _, heroID string, v decks.Viewer) (*decks.HeroHandView, error) {
	s.called, s.viewer = true, v
	if s.err != nil {
		return nil, s.err
	}
	card := decks.HandCard{DeckName: "Omens"}
	out := &decks.HeroHandView{HeroID: heroID}
	switch {
	case s.empty:
	case v.UserID == s.own || v.Director:
		out.Own = &decks.HandSpot{HeroID: heroID, Hand: []decks.HandCard{card, card}, Play: []decks.HandCard{card}}
	default:
		out.Others = []decks.HandOther{{HeroID: heroID, DeckName: "Omens", Count: 2, Faces: []decks.Face{{}}}}
	}
	return out, nil
}

func TestDecksPageTabState(t *testing.T) {
	on := fakeAddonSwitch{on: map[string]bool{decks.AddonSlug: true}}
	player := func() *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}
	}
	director := func() *campaigns.CampaignContext {
		return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	}
	tests := []struct {
		name       string
		addons     decksAddonSwitch
		svc        *heroHandSvc
		cc         *campaigns.CampaignContext
		user       string
		noEntity   bool
		wantRows   int
		wantErr    bool
		wantAsked  bool
		wantViewer decks.Viewer
	}{
		{"add-on off", fakeAddonSwitch{on: map[string]bool{}}, &heroHandSvc{own: "u-tam"}, player(), "u-tam", false, 0, false, false, decks.Viewer{}},
		{"add-on lookup fails", fakeAddonSwitch{err: errors.New("db down")}, &heroHandSvc{own: "u-tam"}, player(), "u-tam", false, 0, true, false, decks.Viewer{}},
		{"no add-on service", nil, &heroHandSvc{own: "u-tam"}, player(), "u-tam", false, 0, false, false, decks.Viewer{}},
		{"incomplete context", on, &heroHandSvc{own: "u-tam"}, &campaigns.CampaignContext{}, "u-tam", false, 0, false, false, decks.Viewer{}},
		{"no page", on, &heroHandSvc{own: "u-tam"}, player(), "u-tam", true, 0, false, false, decks.Viewer{}},
		{"not a hero or not viewable", on, &heroHandSvc{err: errors.New("not found")}, player(), "u-tam", false, 0, true, true, decks.Viewer{UserID: "u-tam", MemberRole: int(campaigns.RolePlayer), VisibilityRole: int(campaigns.RolePlayer)}},
		{"the hero's player: hand and play", on, &heroHandSvc{own: "u-tam"}, player(), "u-tam", false, 3, false, true, decks.Viewer{UserID: "u-tam", MemberRole: int(campaigns.RolePlayer), VisibilityRole: int(campaigns.RolePlayer)}},
		{"a Director", on, &heroHandSvc{own: "u-tam"}, director(), "u-dm", false, 3, false, true, decks.Viewer{UserID: "u-dm", MemberRole: int(campaigns.RoleOwner), VisibilityRole: int(campaigns.RoleOwner), Director: true}},
		{"another player: counts only", on, &heroHandSvc{own: "u-tam"}, player(), "u-ilse", false, 2, false, true, decks.Viewer{UserID: "u-ilse", MemberRole: int(campaigns.RolePlayer), VisibilityRole: int(campaigns.RolePlayer)}},
		{"a hero with no cards", on, &heroHandSvc{own: "u-tam", empty: true}, player(), "u-tam", false, 0, false, true, decks.Viewer{UserID: "u-tam", MemberRole: int(campaigns.RolePlayer), VisibilityRole: int(campaigns.RolePlayer)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tab := decksPageTab(tc.svc, tc.addons)
			rc := entities.BlockRenderContext{CC: tc.cc, UserID: tc.user, Entity: &entities.Entity{ID: "h-tam"}}
			if tc.noEntity {
				rc.Entity = nil
			}
			rows, canAdd, err := tab.State(context.Background(), rc)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if rows != tc.wantRows || canAdd {
				t.Errorf("rows, canAdd = %d, %v; want %d, false", rows, canAdd, tc.wantRows)
			}
			if tc.svc.called != tc.wantAsked {
				t.Errorf("service asked = %v, want %v", tc.svc.called, tc.wantAsked)
			}
			if tc.wantAsked && tc.svc.viewer != tc.wantViewer {
				t.Errorf("viewer = %+v, want %+v", tc.svc.viewer, tc.wantViewer)
			}
		})
	}
}

// With no service the tab is never offered, and the shape the page panel
// reads (label, selector, no action button) is fixed.
func TestDecksPageTabShape(t *testing.T) {
	tab := decksPageTab(nil, fakeAddonSwitch{on: map[string]bool{decks.AddonSlug: true}})
	if tab.Label != "Cards" || tab.Act != "" || tab.Count != ".dkh-strip .dkh-item" {
		t.Errorf("tab = %+v", tab)
	}
	rows, canAdd, err := tab.State(context.Background(), entities.BlockRenderContext{
		CC:     &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}},
		Entity: &entities.Entity{ID: "h"},
	})
	if rows != 0 || canAdd || err != nil {
		t.Errorf("nil service: %d %v %v", rows, canAdd, err)
	}
}
