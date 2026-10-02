// codm_visibility_test.go pins that a co-DM (MemberRole=Player,
// IsDmGranted=true) reaches the NPC service as cc.VisibilityRole() (promoted
// to Owner), not the raw int(cc.MemberRole) — from both NPCSection (the
// Characters-page NPC/Monsters block) and CountAPI (the sidebar badge),
// which must stay in agreement.
//
// TEST HONESTY: a handler unit test with a fake NPCService proves which role
// integer the handler forwards, not that the real SQL/entity-visibility
// predicate honours it — that is visibleNPCIDs and the entities plugin's own
// predicate.
package npcs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeRoleCapturingNPCService is a minimal NPCService that records every role
// it was called with.
type fakeRoleCapturingNPCService struct {
	listRoles     []int
	lastCountRole int
	countCalled   bool
}

func (f *fakeRoleCapturingNPCService) ListNPCs(ctx context.Context, campaignID string, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error) {
	f.listRoles = append(f.listRoles, role)
	return nil, 0, nil
}

func (f *fakeRoleCapturingNPCService) CountNPCs(ctx context.Context, campaignID string, role int, userID string) (int, error) {
	f.lastCountRole = role
	f.countCalled = true
	return 0, nil
}

func (f *fakeRoleCapturingNPCService) ListTags(ctx context.Context, campaignID string, role int, userID string, includeDmOnly bool) ([]NPCTagInfo, error) {
	return nil, nil
}

func (f *fakeRoleCapturingNPCService) SetTagLister(TagLister) {}

func coDMContext()*campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
	}
}

func plainPlayerContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RolePlayer,
	}
}

func newNPCTestContext(method, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// TestNPCSection_CoDMIsPromotedToOwnerForVisibility covers handler.go's
// NPCSection (the entities.NPCSectionProvider implementation): a co-DM must
// see dm_only / custom-restricted NPCs in the Characters page section, same
// as an Owner would.
func TestNPCSection_CoDMIsPromotedToOwnerForVisibility(t *testing.T) {
	svc := &fakeRoleCapturingNPCService{}
	h := NewHandler(svc)

	// featureTag "" skips the "featured" ListNPCs call, leaving exactly one
	// call (the "all" list) to inspect.
	_ = h.NPCSection(context.Background(), coDMContext(), "user-1", "csrf-token", "")

	if len(svc.listRoles) != 1 {
		t.Fatalf("ListNPCs called %d times, want 1", len(svc.listRoles))
	}
	if got := svc.listRoles[0]; got != int(permissions.RoleOwner) {
		t.Errorf("co-DM's role reaching NPCService.ListNPCs (NPCSection) = %d, want %d (permissions.RoleOwner via cc.VisibilityRole())",
			got, int(permissions.RoleOwner))
	}
}

// TestCountNPCs_CoDMIsPromotedToOwnerForVisibility covers handler.go's
// CountAPI (the sidebar badge).
func TestCountNPCs_CoDMIsPromotedToOwnerForVisibility(t *testing.T) {
	svc := &fakeRoleCapturingNPCService{}
	h := NewHandler(svc)

	c, rec := newNPCTestContext(http.MethodGet, "/campaigns/camp-1/npcs/count")
	c.Set("campaign_context", coDMContext())

	if err := h.CountAPI(c); err != nil {
		t.Fatalf("CountAPI returned unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("CountAPI status = %d, want 200", rec.Code)
	}
	if !svc.countCalled {
		t.Fatal("CountNPCs was never called")
	}
	if svc.lastCountRole != int(permissions.RoleOwner) {
		t.Errorf("co-DM's role reaching NPCService.CountNPCs = %d, want %d (permissions.RoleOwner via cc.VisibilityRole())",
			svc.lastCountRole, int(permissions.RoleOwner))
	}
}

// TestNPCSectionAndCountAPI_AgreeForCoDM checks that NPCSection and CountAPI
// land on the same promoted role for the same co-DM viewer.
func TestNPCSectionAndCountAPI_AgreeForCoDM(t *testing.T) {
	cc := coDMContext()

	sectionSvc := &fakeRoleCapturingNPCService{}
	sectionHandler := NewHandler(sectionSvc)
	_ = sectionHandler.NPCSection(context.Background(), cc, "user-1", "csrf-token", "")

	countSvc := &fakeRoleCapturingNPCService{}
	countHandler := NewHandler(countSvc)
	c, _ := newNPCTestContext(http.MethodGet, "/campaigns/camp-1/npcs/count")
	c.Set("campaign_context", cc)
	if err := countHandler.CountAPI(c); err != nil {
		t.Fatalf("CountAPI returned unexpected error: %v", err)
	}

	if len(sectionSvc.listRoles) != 1 {
		t.Fatalf("NPCSection's ListNPCs called %d times, want 1", len(sectionSvc.listRoles))
	}
	if sectionSvc.listRoles[0] != countSvc.lastCountRole {
		t.Errorf("NPCSection role (%d) and CountAPI role (%d) disagree for the same co-DM viewer — "+
			"handler.go:60 says these are meant to match",
			sectionSvc.listRoles[0], countSvc.lastCountRole)
	}
}

// TestNPCSection_PlainPlayerIsNotPromoted and its CountAPI twin are the
// negative controls: a Player with no DM grant must still reach the service
// at RolePlayer.
func TestNPCSection_PlainPlayerIsNotPromoted(t *testing.T) {
	svc := &fakeRoleCapturingNPCService{}
	h := NewHandler(svc)

	_ = h.NPCSection(context.Background(), plainPlayerContext(), "user-1", "csrf-token", "")

	if len(svc.listRoles) != 1 {
		t.Fatalf("ListNPCs called %d times, want 1", len(svc.listRoles))
	}
	if got := svc.listRoles[0]; got != int(campaigns.RolePlayer) {
		t.Errorf("plain Player's role reaching NPCService.ListNPCs (NPCSection) = %d, want %d (campaigns.RolePlayer, unpromoted)",
			got, int(campaigns.RolePlayer))
	}
}

func TestCountNPCs_PlainPlayerIsNotPromoted(t *testing.T) {
	svc := &fakeRoleCapturingNPCService{}
	h := NewHandler(svc)

	c, _ := newNPCTestContext(http.MethodGet, "/campaigns/camp-1/npcs/count")
	c.Set("campaign_context", plainPlayerContext())

	if err := h.CountAPI(c); err != nil {
		t.Fatalf("CountAPI returned unexpected error: %v", err)
	}
	if !svc.countCalled {
		t.Fatal("CountNPCs was never called")
	}
	if svc.lastCountRole != int(campaigns.RolePlayer) {
		t.Errorf("plain Player's role reaching NPCService.CountNPCs = %d, want %d (campaigns.RolePlayer, unpromoted)",
			svc.lastCountRole, int(campaigns.RolePlayer))
	}
}
