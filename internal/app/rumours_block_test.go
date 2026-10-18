package app

// rumours_block_test.go pins the Rumours block's seam to the page: the viewer
// it asks the service about, what a hidden or failed read draws, and the
// page-access answers the plugin relies on.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/rumours"
)

// fakeRumoursService records the viewer it was asked about and answers with a
// canned view or error; the other methods are never reached by a page render.
type fakeRumoursService struct {
	rumours.Service
	view  *rumours.View
	err   error
	calls int
	got   rumours.Viewer
	gotC  string
	gotE  string
}

func (f *fakeRumoursService) View(_ context.Context, campaignID, entityID string, v rumours.Viewer) (*rumours.View, error) {
	f.calls++
	f.got, f.gotC, f.gotE = v, campaignID, entityID
	return f.view, f.err
}

func rumoursRC(role campaigns.Role, granted bool) entities.BlockRenderContext {
	return entities.BlockRenderContext{
		Block:  entities.TemplateBlock{Type: rumours.BlockType},
		CC:     &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: granted},
		Entity: &entities.Entity{ID: "page-1", CampaignID: "camp-1"},
		UserID: "user-1",
	}
}

func TestRumoursBlock_Registration(t *testing.T) {
	reg := entities.NewBlockRegistry()
	registerRumoursBlock(reg, &fakeRumoursService{})
	if !reg.IsValid(rumours.BlockType) {
		t.Fatal("the rumours block should be registered")
	}
	if !reg.IsSingleton(rumours.BlockType) {
		t.Error("rumours is one list per page")
	}
}

func TestRumoursBlock_Render(t *testing.T) {
	shown := &rumours.View{Total: 1, Rumours: []rumours.RumourView{{ID: "r1", Quote: "She sank her own ship.", Who: "Bram", Sure: 2}}}
	cases := []struct {
		name     string
		rc       entities.BlockRenderContext
		svc      *fakeRumoursService
		wantRole int
		drawn    bool
	}{
		{"player is asked as a player", rumoursRC(campaigns.RolePlayer, false), &fakeRumoursService{view: shown}, int(campaigns.RolePlayer), true},
		{"scribe is asked as a scribe", rumoursRC(campaigns.RoleScribe, false), &fakeRumoursService{view: shown}, int(campaigns.RoleScribe), true},
		{"owner is asked as the owner", rumoursRC(campaigns.RoleOwner, false), &fakeRumoursService{view: shown}, int(campaigns.RoleOwner), true},
		{"a granted co-director is asked as the owner", rumoursRC(campaigns.RolePlayer, true), &fakeRumoursService{view: shown}, int(campaigns.RoleOwner), true},
		{"a hidden view draws nothing", rumoursRC(campaigns.RolePlayer, false), &fakeRumoursService{view: &rumours.View{Hidden: true}}, int(campaigns.RolePlayer), false},
		{"a failed read draws nothing", rumoursRC(campaigns.RolePlayer, false), &fakeRumoursService{err: errors.New("db down")}, int(campaigns.RolePlayer), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := entities.NewBlockRegistry()
			registerRumoursBlock(reg, tc.svc)
			got := renderToString(t, reg.Render(context.Background(), tc.rc))
			if tc.svc.got.Role != tc.wantRole || tc.svc.got.UserID != "user-1" {
				t.Errorf("viewer = role %d user %q, want role %d user-1", tc.svc.got.Role, tc.svc.got.UserID, tc.wantRole)
			}
			if tc.svc.gotC != "camp-1" || tc.svc.gotE != "page-1" {
				t.Errorf("asked about %q/%q", tc.svc.gotC, tc.svc.gotE)
			}
			if drawn := strings.Contains(got, `data-widget="rumours"`); drawn != tc.drawn {
				t.Errorf("drawn = %v, want %v: %q", drawn, tc.drawn, got)
			}
			if !tc.drawn && strings.TrimSpace(got) != "" {
				t.Errorf("a block that draws nothing must leave no markup: %q", got)
			}
		})
	}
}

func TestRumoursBlock_NoContextCallsNothing(t *testing.T) {
	svc := &fakeRumoursService{view: &rumours.View{}}
	reg := entities.NewBlockRegistry()
	registerRumoursBlock(reg, svc)
	rc := rumoursRC(campaigns.RoleOwner, false)
	rc.Entity = nil
	if got := renderToString(t, reg.Render(context.Background(), rc)); strings.TrimSpace(got) != "" || svc.calls != 0 {
		t.Errorf("no page: drew %q after %d calls", got, svc.calls)
	}
}

// fakePageService is the two entity methods the adapter uses.
type fakePageService struct {
	entities.EntityService
	entity *entities.Entity
	access *entities.EffectivePermission
	err    error
}

func (f fakePageService) GetByID(context.Context, string) (*entities.Entity, error) {
	return f.entity, f.err
}

func (f fakePageService) CheckEntityAccess(context.Context, string, int, string) (*entities.EffectivePermission, error) {
	return f.access, f.err
}

func TestRumoursPages_Open(t *testing.T) {
	page := &entities.Entity{ID: "page-1", CampaignID: "camp-1"}
	cases := []struct {
		name string
		svc  fakePageService
		want int
	}{
		{"viewable page opens", fakePageService{entity: page, access: &entities.EffectivePermission{CanView: true}}, 0},
		{"missing page is not found", fakePageService{}, http.StatusNotFound},
		{"another campaign's page is not found", fakePageService{entity: &entities.Entity{ID: "page-1", CampaignID: "camp-2"}, access: &entities.EffectivePermission{CanView: true}}, http.StatusNotFound},
		{"a page the viewer may not see is not found", fakePageService{entity: page, access: &entities.EffectivePermission{}}, http.StatusNotFound},
		{"a failed lookup is not found", fakePageService{err: errors.New("db down")}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (&rumoursPagesAdapter{svc: tc.svc}).Open(context.Background(), "camp-1", "page-1", 1, "user-1")
			if tc.want == 0 {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if got := apperror.SafeCode(err); got != tc.want {
				t.Fatalf("code = %d, want %d", got, tc.want)
			}
		})
	}
}
