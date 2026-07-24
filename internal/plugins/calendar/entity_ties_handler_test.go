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

	"github.com/keyxmakerx/chronicle/internal/permissions"
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
	var gotRole int
	var gotUser string
	repo := &mockCalendarRepo{
		entitiesForEventFn: func(_ context.Context, eventID string, viewerRole int, userID string) ([]EntityTieRef, error) {
			gotRole, gotUser = viewerRole, userID
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
	// C-CAL-ENTITY-TIES-LEAK-FIX: the handler must forward the real viewer
	// role + userID to the repo — before the fix, EntitiesForEvent took no
	// viewer context at all, so no filtering was structurally possible.
	if gotRole != int(campaigns.RolePlayer) || gotUser != "user-1" {
		t.Errorf("viewer context not forwarded to repo: role=%d user=%q, want role=%d user=user-1", gotRole, gotUser, campaigns.RolePlayer)
	}
}

// TestListEventEntitiesAPI_RespectsEntityVisibility pins C-CAL-ENTITY-TIES-LEAK-FIX
// (a cordinator#32 gap #1 follow-up the initial audit missed): before the fix,
// ListEventEntitiesAPI called svc.EntitiesForEvent(ctx, eventID) with NO viewer
// context, so any Player could read a dm_only entity's NAME via any event's tie
// list. The mock repo below models the same role>=RoleOwner threshold
// entityVisibilityFilter enforces in the real WHERE clause (see
// TestEntityVisibilityFilter in entity_ties_test.go for the actual SQL-fragment
// coverage — there is no live MariaDB in this sandbox for a literal query-level
// repro; see the PR for that deviation note) to prove the handler now forwards
// the viewer identity that filtering depends on, for Player, Owner, and co-DM
// (IsDmGranted) requesters.
func TestListEventEntitiesAPI_RespectsEntityVisibility(t *testing.T) {
	visible := func() EntityTieRef {
		r := "involved"
		return EntityTieRef{EntityID: "public-1", EntityName: "Public NPC", EntityType: "npc", ParticipationRole: &r}
	}
	dmOnly := func() EntityTieRef {
		r := "involved"
		return EntityTieRef{EntityID: "secret-1", EntityName: "Secret Villain", EntityType: "npc", ParticipationRole: &r}
	}

	tests := []struct {
		name        string
		memberRole  campaigns.Role
		isDmGranted bool
		wantSecret  bool
	}{
		{"player cannot see dm_only tie", campaigns.RolePlayer, false, false},
		{"owner can see dm_only tie", campaigns.RoleOwner, false, true},
		{"co-DM (DM-granted player) can see dm_only tie", campaigns.RolePlayer, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotRole int
			var gotUser string
			repo := &mockCalendarRepo{
				entitiesForEventFn: func(_ context.Context, eventID string, role int, userID string) ([]EntityTieRef, error) {
					gotRole, gotUser = role, userID
					out := []EntityTieRef{visible()}
					if role >= permissions.RoleOwner {
						out = append(out, dmOnly())
					}
					return out, nil
				},
			}
			h := tiesTestHandler(repo)
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api", strings.NewReader(""))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: tt.memberRole, IsDmGranted: tt.isDmGranted,
			})
			c.Set("auth_user_id", "user-9")
			c.SetParamNames("id", "calId", "eid")
			c.SetParamValues("camp-1", "cal-1", "evt-1")

			if err := h.ListEventEntitiesAPI(c); err != nil {
				t.Fatalf("ListEventEntitiesAPI: %v", err)
			}
			if gotUser != "user-9" {
				t.Errorf("viewer userID not forwarded: got %q", gotUser)
			}
			body := rec.Body.String()
			hasSecret := strings.Contains(body, "Secret Villain")
			if hasSecret != tt.wantSecret {
				t.Errorf("dm_only entity visibility = %v, want %v (forwarded role=%d, body=%s)", hasSecret, tt.wantSecret, gotRole, body)
			}
			if !strings.Contains(body, "Public NPC") {
				t.Errorf("public entity should always be visible: %s", body)
			}
		})
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
