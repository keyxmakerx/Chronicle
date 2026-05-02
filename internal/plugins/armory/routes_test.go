// routes_test.go pins the role-gate fix for /armory/purchase. The
// regression we're guarding against: someone changes the route gate from
// RequireRole(RolePlayer) back to RequireRole(RoleScribe), silently
// blocking ordinary players from buying anything in the gallery.
package armory

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestPurchaseRoleGate_AdmitsPlayer pins that a Player passes the role
// middleware bound to /armory/purchase by routes.go. We forge a
// CampaignContext with RolePlayer (as RequireCampaignAccess would set)
// and verify the inner handler runs — i.e. RequireRole(RolePlayer)
// admits Players.
func TestPurchaseRoleGate_AdmitsPlayer(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RolePlayer,
	})

	called := false
	next := func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	}

	if err := campaigns.RequireRole(campaigns.RolePlayer)(next)(c); err != nil {
		t.Fatalf("RequireRole(RolePlayer) returned error for Player: %v", err)
	}
	if !called {
		t.Error("inner handler was not invoked — Player was blocked by the role gate this PR's fix relies on")
	}
}

// TestPurchaseRoleGate_RejectsBelowPlayer is the negative side of the same
// contract: a context with RoleNone (e.g. a non-member) must still be
// rejected. Pins that flipping to RolePlayer didn't accidentally open the
// route to outsiders.
func TestPurchaseRoleGate_RejectsBelowPlayer(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleNone,
	})

	next := func(c echo.Context) error {
		t.Fatal("inner handler should not run for RoleNone")
		return nil
	}

	err := campaigns.RequireRole(campaigns.RolePlayer)(next)(c)
	if err == nil {
		t.Fatal("expected forbidden error for RoleNone")
	}
}
