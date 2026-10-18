package app

// pagetests_block_test.go pins the Tests block's seam to the page: the viewer
// it asks the service about, and when it draws nothing at all.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/pagetests"
)

// fakePageTestsService answers ListForPage with a canned list or error; the
// other methods are never reached by a page render.
type fakePageTestsService struct {
	pagetests.Service
	list  *pagetests.PageList
	err   error
	calls int
	got   pagetests.Viewer
}

func (f *fakePageTestsService) ListForPage(_ context.Context, _, _ string, v pagetests.Viewer) (*pagetests.PageList, error) {
	f.calls++
	f.got = v
	return f.list, f.err
}

func pageTestsRC(role campaigns.Role, granted bool) entities.BlockRenderContext {
	return entities.BlockRenderContext{
		Block:  entities.TemplateBlock{Type: pagetests.BlockType},
		CC:     &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role, IsDmGranted: granted},
		Entity: &entities.Entity{ID: "page-1", CampaignID: "camp-1"},
		UserID: "user-1",
	}
}

func TestPageTestsBlock_Render(t *testing.T) {
	one := &pagetests.PageList{Tests: []pagetests.Test{{ID: "t1", Name: "Hunt"}}}
	cases := []struct {
		name     string
		rc       entities.BlockRenderContext
		svc      *fakePageTestsService
		wantRole int
		drawn    bool
	}{
		{"a player with a shown test sees the block", pageTestsRC(campaigns.RolePlayer, false), &fakePageTestsService{list: one}, int(campaigns.RolePlayer), true},
		{"a player with no shown test sees nothing", pageTestsRC(campaigns.RolePlayer, false), &fakePageTestsService{list: &pagetests.PageList{}}, int(campaigns.RolePlayer), false},
		{"the DM sees an empty block to add to", pageTestsRC(campaigns.RoleOwner, false), &fakePageTestsService{list: &pagetests.PageList{CanEdit: true}}, int(campaigns.RoleOwner), true},
		{"a granted co-DM is asked as the owner", pageTestsRC(campaigns.RolePlayer, true), &fakePageTestsService{list: one}, int(campaigns.RoleOwner), true},
		{"a failed read draws nothing", pageTestsRC(campaigns.RoleOwner, false), &fakePageTestsService{err: errors.New("db down")}, int(campaigns.RoleOwner), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := entities.NewBlockRegistry()
			registerPageTestsBlock(reg, tc.svc)
			if !reg.IsSingleton(pagetests.BlockType) {
				t.Fatal("tests are one block per page")
			}
			got := renderToString(t, reg.Render(context.Background(), tc.rc))
			if tc.svc.got.VisibilityRole != tc.wantRole || tc.svc.got.UserID != "user-1" {
				t.Errorf("viewer = %+v, want role %d user-1", tc.svc.got, tc.wantRole)
			}
			if drawn := strings.Contains(got, `data-widget="page_tests"`); drawn != tc.drawn {
				t.Errorf("drawn = %v, want %v: %q", drawn, tc.drawn, got)
			}
			if !tc.drawn && strings.TrimSpace(got) != "" {
				t.Errorf("a block that draws nothing must leave no markup: %q", got)
			}
		})
	}
}
