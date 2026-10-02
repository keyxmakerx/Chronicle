// codm_visibility_test.go pins that a co-DM (MemberRole=Player,
// IsDmGranted=true) must be handed to the Armory service as a
// promoted viewer via cc.VisibilityRole() — the same promotion
// campaigns.CampaignContext applies on entity paths — not the raw
// int(cc.MemberRole).
//
// This is a handler unit test with a fake ArmoryService: it proves
// which role integer Handler.Index / Handler.CountAPI hand to the
// service, not that the real SQL/entity visibility predicate (armory/
// service.go's visibleItemIDs) honours a co-DM.
package armory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeRoleCapturingService is a minimal ArmoryService that records the role
// argument it was called with and nothing else. ListItems returns an error
// so Index returns immediately after the call under test, before it would
// otherwise reach templ rendering (which needs a fully-populated
// CampaignContext this test doesn't build).
type fakeRoleCapturingService struct {
	lastListRole  int
	lastCountRole int
	listCalled    bool
	countCalled   bool
}

func (f *fakeRoleCapturingService) ListItems(ctx context.Context, campaignID string, role int, userID string, opts ItemListOptions) ([]ItemCard, int, error) {
	f.lastListRole = role
	f.listCalled = true
	return nil, 0, errStopAfterCapture
}

func (f *fakeRoleCapturingService) SetTagLister(TagLister) {}

func (f *fakeRoleCapturingService) ListTagOptions(context.Context, string, int, string) ([]ItemTagInfo, error) {
	return nil, nil
}

func (f *fakeRoleCapturingService) CountItems(ctx context.Context, campaignID string, role int, userID string) (int, error) {
	f.lastCountRole = role
	f.countCalled = true
	return 0, nil
}

func (f *fakeRoleCapturingService) GetItemTypes(ctx context.Context, campaignID string) ([]ItemTypeInfo, error) {
	return nil, nil
}

// errStopAfterCapture is a sentinel error used only to short-circuit Index
// right after it calls ListItems, before rendering.
type stopAfterCaptureErr struct{}

func (stopAfterCaptureErr) Error() string { return "stop after capture (test sentinel)" }

var errStopAfterCapture = stopAfterCaptureErr{}

// coDMContext builds the CampaignContext a co-DM (a DM-granted Player) would
// have: MemberRole stays Player (the system-assigned title is unchanged),
// but IsDmGranted is true.
func coDMContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
	}
}

// plainPlayerContext builds the CampaignContext of an ordinary Player with
// no DM grant.
func plainPlayerContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RolePlayer,
	}
}

func newArmoryTestContext(cc *campaigns.CampaignContext, method, path string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", cc)
	return c, rec
}

// TestListItems_CoDMIsPromotedToOwnerForVisibility: a co-DM must reach
// the service as permissions.RoleOwner (via cc.VisibilityRole()),
// matching what visibleItemIDs treats as unrestricted — otherwise a
// co-DM is narrowed like a plain Player and misses dm_only / custom-
// restricted items.
func TestListItems_CoDMIsPromotedToOwnerForVisibility(t *testing.T) {
	svc := &fakeRoleCapturingService{}
	h := NewHandler(svc)

	c, _ := newArmoryTestContext(coDMContext(), http.MethodGet, "/campaigns/camp-1/armory")
	_ = h.Index(c) // error is expected (errStopAfterCapture); we only care about the captured role

	if !svc.listCalled {
		t.Fatal("ListItems was never called")
	}
	if svc.lastListRole != int(permissions.RoleOwner) {
		t.Errorf("co-DM's role reaching ArmoryService.ListItems = %d, want %d (permissions.RoleOwner via cc.VisibilityRole()) — "+
			"a co-DM must see dm_only/custom-restricted armory items like an Owner does",
			svc.lastListRole, int(permissions.RoleOwner))
	}
}

// TestCountItems_CoDMIsPromotedToOwnerForVisibility is the same finding
// against Handler.CountAPI / ArmoryService.CountItems, so the badge count a
// co-DM sees agrees with the list they can page through.
func TestCountItems_CoDMIsPromotedToOwnerForVisibility(t *testing.T) {
	svc := &fakeRoleCapturingService{}
	h := NewHandler(svc)

	c, rec := newArmoryTestContext(coDMContext(), http.MethodGet, "/campaigns/camp-1/armory/count")
	if err := h.CountAPI(c); err != nil {
		t.Fatalf("CountAPI returned unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("CountAPI status = %d, want 200", rec.Code)
	}
	if !svc.countCalled {
		t.Fatal("CountItems was never called")
	}
	if svc.lastCountRole != int(permissions.RoleOwner) {
		t.Errorf("co-DM's role reaching ArmoryService.CountItems = %d, want %d (permissions.RoleOwner via cc.VisibilityRole())",
			svc.lastCountRole, int(permissions.RoleOwner))
	}
}

// TestListItems_PlainPlayerIsNotPromoted is the negative control: a Player
// with no DM grant must still reach the service at RolePlayer, never Owner.
// This must pass BOTH before and after the fix — it guards against the fix
// overshooting into "everyone is promoted".
func TestListItems_PlainPlayerIsNotPromoted(t *testing.T) {
	svc := &fakeRoleCapturingService{}
	h := NewHandler(svc)

	c, _ := newArmoryTestContext(plainPlayerContext(), http.MethodGet, "/campaigns/camp-1/armory")
	_ = h.Index(c)

	if !svc.listCalled {
		t.Fatal("ListItems was never called")
	}
	if svc.lastListRole != int(campaigns.RolePlayer) {
		t.Errorf("plain Player's role reaching ArmoryService.ListItems = %d, want %d (campaigns.RolePlayer, unpromoted)",
			svc.lastListRole, int(campaigns.RolePlayer))
	}
}

func TestCountItems_PlainPlayerIsNotPromoted(t *testing.T) {
	svc := &fakeRoleCapturingService{}
	h := NewHandler(svc)

	c, _ := newArmoryTestContext(plainPlayerContext(), http.MethodGet, "/campaigns/camp-1/armory/count")
	if err := h.CountAPI(c); err != nil {
		t.Fatalf("CountAPI returned unexpected error: %v", err)
	}
	if !svc.countCalled {
		t.Fatal("CountItems was never called")
	}
	if svc.lastCountRole != int(campaigns.RolePlayer) {
		t.Errorf("plain Player's role reaching ArmoryService.CountItems = %d, want %d (campaigns.RolePlayer, unpromoted)",
			svc.lastCountRole, int(campaigns.RolePlayer))
	}
}
