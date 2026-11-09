// dm_grant_dashboard_test.go pins ADR-057: the dashboard's "recently updated
// entities" list must be gated on cc.VisibilityRole(), not the raw
// cc.MemberRole, so a Co-DM (Player + DM grant) sees dm_only entities there
// the same as in the entities plugin's own list and category views.
//
// This drives the real Show handler up to the ListRecentForDashboard call
// and captures the role integer actually passed (recovering from any
// unrelated template panic so the capture still lands), rather than
// rendering the full dashboard page or stubbing VisibilityRole() itself.
package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// dashboardStubCampaignService only needs GetPendingTransfer to not panic;
// Show's other CampaignService use (none, before the recentLister call) is
// never reached.
type dashboardStubCampaignService struct {
	CampaignService
}

func (dashboardStubCampaignService) GetPendingTransfer(_ context.Context, _ string) (*OwnershipTransfer, error) {
	return nil, nil
}

// roleCaptureRecentLister records the role argument ListRecentForDashboard
// was called with, so the test can assert on the actual value threaded
// through the handler rather than on behavior of a stub that encodes the
// promotion rule itself.
type roleCaptureRecentLister struct {
	called  bool
	gotRole int
}

func (r *roleCaptureRecentLister) ListRecentForDashboard(_ context.Context, _ string, role int, _ string, _ int) ([]RecentEntity, error) {
	r.called = true
	r.gotRole = role
	return nil, nil
}

func dashboardCoDmContext() *CampaignContext {
	return &CampaignContext{
		Campaign:    &Campaign{ID: "camp-1"},
		MemberRole:  RolePlayer,
		IsDmGranted: true,
		IsMember:    true,
	}
}

// callShowCapturingRole drives h.Show(c), recovering from any panic in the
// (unrelated) template render that follows the recentLister call, so the
// capture made before that render still stands.
func callShowCapturingRole(t *testing.T, h *Handler, c *echo.Context) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Show panicked during unrelated template rendering (expected in this minimal fixture): %v", r)
		}
	}()
	_ = h.Show(c)
}

// TestShow_RecentEntities_UsesPromotedVisibilityRole pins that the
// "recently updated" entity list is gated on cc.VisibilityRole()
// (Owner=3 for a Co-DM), not the raw cc.MemberRole (Player=1).
func TestShow_RecentEntities_UsesPromotedVisibilityRole(t *testing.T) {
	h := NewHandler(dashboardStubCampaignService{})
	capture := &roleCaptureRecentLister{}
	h.SetRecentEntityLister(capture)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}})
	c.Set(contextKeyCampaign, dashboardCoDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	callShowCapturingRole(t, h, c)

	if !capture.called {
		t.Fatal("Show never called ListRecentForDashboard — cannot assert on the role it would have passed")
	}
	if capture.gotRole != int(RoleOwner) {
		t.Errorf("Show must call ListRecentForDashboard with the Co-DM's promoted VisibilityRole (Owner=%d), got role=%d (raw MemberRole=%d) — "+
			"a Co-DM would see dm_only entities vanish from the dashboard's recently-updated list even though they can open them directly",
			int(RoleOwner), capture.gotRole, int(RolePlayer))
	}
}
