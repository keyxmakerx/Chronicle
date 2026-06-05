// entity_ties_handler_test.go — C-CAL-WORLDSTATE-PRODUCTION-PORT 2b.
//
// The production attach-entity picker's HTTP surface: list / link / unlink
// ties on a real event, IDOR-closed via requireEventInCampaign. Role gating
// (Player read / Scribe write) is middleware-enforced at the route layer
// (campaigns.RequireRole in routes.go) and exercised by the campaigns
// package; these tests cover the handler logic + the cross-campaign IDOR.
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// tiesTestHandler wires a Handler whose service reads an event + its calendar
// from the mock repo so requireEventInCampaign resolves. The event belongs to
// calendar "cal-1" in campaign "camp-1".
func tiesTestHandler(repo *mockCalendarRepo) *Handler {
	if repo.getEventFn == nil {
		repo.getEventFn = func(_ context.Context, id string) (*Event, error) {
			return &Event{ID: id, CalendarID: "cal-1", Name: "Siege", Visibility: "everyone"}, nil
		}
	}
	if repo.getByIDFn == nil {
		repo.getByIDFn = func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Name: "Harptos"}, nil
		}
	}
	return NewHandler(NewCalendarService(repo))
}

func tiesCtx(e *echo.Echo, method, body string, role campaigns.Role) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, "/api", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role,
	})
	c.Set("auth_user_id", "user-1")
	return c, rec
}

func TestListEventEntitiesAPI_ReturnsTies(t *testing.T) {
	role := "involved"
	repo := &mockCalendarRepo{
		entitiesForEventFn: func(_ context.Context, eventID string) ([]EntityTieRef, error) {
			return []EntityTieRef{{EntityID: "npc-1", EntityName: "Marisha", EntityType: "npc", ParticipationRole: &role}}, nil
		},
	}
	h := tiesTestHandler(repo)
	e := echo.New()
	c, rec := tiesCtx(e, http.MethodGet, "", campaigns.RolePlayer)
	c.SetParamNames("id", "calId", "eid")
	c.SetParamValues("camp-1", "cal-1", "evt-1")

	if err := h.ListEventEntitiesAPI(c); err != nil {
		t.Fatalf("ListEventEntitiesAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Marisha") || !strings.Contains(body, `"participation_role":"involved"`) {
		t.Errorf("response missing tie data: %s", body)
	}
}

func TestLinkEventEntityAPI_LinksWithRole(t *testing.T) {
	var gotEntity, gotEvent, gotRole string
	repo := &mockCalendarRepo{
		linkEntityEventFn: func(_ context.Context, entityID, eventID, role string) error {
			gotEntity, gotEvent, gotRole = entityID, eventID, role
			return nil
		},
	}
	h := tiesTestHandler(repo)
	e := echo.New()
	c, rec := tiesCtx(e, http.MethodPut, `{"role":"affected"}`, campaigns.RoleScribe)
	c.SetParamNames("id", "calId", "eid", "entityId")
	c.SetParamValues("camp-1", "cal-1", "evt-1", "npc-1")

	if err := h.LinkEventEntityAPI(c); err != nil {
		t.Fatalf("LinkEventEntityAPI: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d want 200", rec.Code)
	}
	if gotEntity != "npc-1" || gotEvent != "evt-1" || gotRole != "affected" {
		t.Errorf("link args = (%q,%q,%q); want (npc-1,evt-1,affected)", gotEntity, gotEvent, gotRole)
	}
}

func TestLinkEventEntityAPI_InvalidRoleRejected(t *testing.T) {
	called := false
	repo := &mockCalendarRepo{
		linkEntityEventFn: func(_ context.Context, _, _, _ string) error { called = true; return nil },
	}
	h := tiesTestHandler(repo)
	e := echo.New()
	c, _ := tiesCtx(e, http.MethodPut, `{"role":"bogus"}`, campaigns.RoleScribe)
	c.SetParamNames("id", "calId", "eid", "entityId")
	c.SetParamValues("camp-1", "cal-1", "evt-1", "npc-1")

	if err := h.LinkEventEntityAPI(c); err == nil {
		t.Errorf("invalid role should return an error")
	}
	if called {
		t.Errorf("invalid role must not reach the repo")
	}
}

func TestUnlinkEventEntityAPI_Detaches(t *testing.T) {
	var gotEntity, gotEvent string
	repo := &mockCalendarRepo{
		unlinkEntityEventFn: func(_ context.Context, entityID, eventID string) error {
			gotEntity, gotEvent = entityID, eventID
			return nil
		},
	}
	h := tiesTestHandler(repo)
	e := echo.New()
	c, rec := tiesCtx(e, http.MethodDelete, "", campaigns.RoleScribe)
	c.SetParamNames("id", "calId", "eid", "entityId")
	c.SetParamValues("camp-1", "cal-1", "evt-1", "npc-1")

	if err := h.UnlinkEventEntityAPI(c); err != nil {
		t.Fatalf("UnlinkEventEntityAPI: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status=%d want 204", rec.Code)
	}
	if gotEntity != "npc-1" || gotEvent != "evt-1" {
		t.Errorf("unlink args = (%q,%q); want (npc-1,evt-1)", gotEntity, gotEvent)
	}
}

func TestLinkEventEntityAPI_CrossCampaignIDOR(t *testing.T) {
	// The event's calendar belongs to a DIFFERENT campaign → requireEventInCampaign
	// must 404 before any link happens.
	called := false
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "OTHER-camp"}, nil
		},
		linkEntityEventFn: func(_ context.Context, _, _, _ string) error { called = true; return nil },
	}
	h := tiesTestHandler(repo)
	e := echo.New()
	c, _ := tiesCtx(e, http.MethodPut, `{"role":"involved"}`, campaigns.RoleScribe)
	c.SetParamNames("id", "calId", "eid", "entityId")
	c.SetParamValues("camp-1", "cal-1", "evt-1", "npc-1")

	err := h.LinkEventEntityAPI(c)
	if err == nil {
		t.Errorf("cross-campaign link should be rejected")
	}
	if !isAppErrorType(err, "not_found") {
		t.Errorf("expected not_found IDOR error, got %v", err)
	}
	if called {
		t.Errorf("cross-campaign link must not reach the repo")
	}
}
