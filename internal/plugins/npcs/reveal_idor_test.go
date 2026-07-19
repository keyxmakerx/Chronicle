// reveal_idor_test.go — SEC-IDOR-1 cross-campaign guard for the NPC reveal
// toggle. ToggleReveal took the entity id straight from the URL and toggled
// is_private with no campaign check, so a Scribe in one campaign could flip
// visibility on another campaign's entity by UUID. The handler now forwards the
// caller's campaign to the (campaign-scoped) toggler and surfaces its NotFound
// verbatim. These tests pin both halves: the scope is forwarded, and a mismatch
// is a 404 (not a masked 500).
package npcs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeToggler records the campaign it was called with and models the real
// entities.TogglePrivateInCampaign guard: it flips only when campaignID matches
// the entity's home campaign, returning NotFound otherwise.
type fakeToggler struct {
	homeCampaign  string
	gotCampaignID string
	gotEntityID   string
	called        bool
}

func (f *fakeToggler) TogglePrivate(_ context.Context, entityID, campaignID string) (bool, error) {
	f.called = true
	f.gotEntityID = entityID
	f.gotCampaignID = campaignID
	if campaignID != f.homeCampaign {
		return false, apperror.NewNotFound("entity not found")
	}
	return true, nil
}

func serveToggleReveal(t *testing.T, tog VisibilityToggler, campaignID, eid string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	h := NewHandler(stubNPCSvc{})
	h.SetVisibilityToggler(tog)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/campaigns/"+campaignID+"/npcs/"+eid+"/reveal", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues(campaignID, eid)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: campaignID},
		MemberRole: campaigns.RoleScribe,
	})
	return rec, h.ToggleReveal(c)
}

// TestToggleReveal_ForwardsCampaignScope: the handler passes cc.Campaign.ID to
// the toggler, and a same-campaign reveal succeeds (200). Regression guard so
// the scope check doesn't block legitimate reveals.
func TestToggleReveal_ForwardsCampaignScope(t *testing.T) {
	tog := &fakeToggler{homeCampaign: "camp-1"}
	rec, err := serveToggleReveal(t, tog, "camp-1", "ent-1")
	if err != nil {
		t.Fatalf("same-campaign reveal should succeed, got: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !tog.called {
		t.Fatal("toggler was never called")
	}
	if tog.gotCampaignID != "camp-1" {
		t.Errorf("handler must forward cc.Campaign.ID to the toggler; got %q, want %q", tog.gotCampaignID, "camp-1")
	}
}

// TestToggleReveal_CrossCampaignRejected: the entity lives in another campaign,
// so the campaign-scoped toggler returns NotFound; the handler must surface that
// as a 404, not mask it as a 500.
func TestToggleReveal_CrossCampaignRejected(t *testing.T) {
	tog := &fakeToggler{homeCampaign: "camp-owner"} // entity belongs elsewhere
	_, err := serveToggleReveal(t, tog, "camp-attacker", "ent-1")
	if err == nil {
		t.Fatal("cross-campaign reveal must be rejected")
	}
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 AppError, got %v", err)
	}
	if tog.gotCampaignID != "camp-attacker" {
		t.Errorf("handler must forward the caller's campaign; got %q", tog.gotCampaignID)
	}
}
