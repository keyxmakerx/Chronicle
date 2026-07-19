// entity_history_idor_test.go — SEC-IDOR-2 cross-campaign + visibility guard for
// the per-entity audit history endpoint. EntityHistory returned an entity's full
// change log (campaign id, entity name, editor display names, timestamps) for
// any entity id, with no ownership or visibility check. It now (a) resolves the
// entity through the injected guard and 404s a foreign-campaign or non-viewable
// id, and (b) always scopes the history query to the caller's campaign as an
// unconditional backstop. These tests pin both layers.
package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeHistorySvc records the campaign scope EntityHistory threads into the
// service and returns a sentinel row so a served (leaked) response is detectable.
type fakeHistorySvc struct {
	AuditService
	gotEntityID   string
	gotCampaignID string
}

func (f *fakeHistorySvc) GetEntityHistory(_ context.Context, entityID, campaignID string) ([]AuditEntry, error) {
	f.gotEntityID = entityID
	f.gotCampaignID = campaignID
	return []AuditEntry{{ID: 1, EntityID: entityID, EntityName: "leaked-secret-name"}}, nil
}

// fakeViewGuard models entities.ResolveEntityView: campaignOf gives the entity's
// home campaign, canView its visibility for the caller.
type fakeViewGuard struct {
	campaignOf map[string]string
	canView    map[string]bool
}

func (g fakeViewGuard) ResolveEntityView(_ context.Context, entityID string, _ int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	return camp, g.canView[entityID], nil
}

func TestEntityHistory_CampaignAndVisibilityGate(t *testing.T) {
	guard := fakeViewGuard{
		campaignOf: map[string]string{"viewable": "c1", "restricted": "c1", "foreign": "c2"},
		canView:    map[string]bool{"viewable": true, "restricted": false, "foreign": true},
	}

	cases := []struct {
		name   string
		eid    string
		wantOK bool
	}{
		{"viewable same-campaign entity → served", "viewable", true},
		{"custom-restricted entity → 404 (visibility gate)", "restricted", false},
		{"foreign-campaign entity → 404 (kills IDOR)", "foreign", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeHistorySvc{}
			h := NewHandler(svc)
			h.SetEntityViewGuard(guard)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/"+tc.eid+"/history", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", tc.eid)
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:   &campaigns.Campaign{ID: "c1"},
				MemberRole: campaigns.RolePlayer,
			})

			err := h.EntityHistory(c)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("permitted viewer got error: %v", err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("permitted viewer got %d, want 200", rec.Code)
				}
				if svc.gotCampaignID != "c1" {
					t.Errorf("history query must be scoped to the caller's campaign; got %q", svc.gotCampaignID)
				}
				return
			}
			// Denied: 404 apperror, and the history query must never run.
			assertAppError(t, err, http.StatusNotFound)
			if svc.gotEntityID != "" {
				t.Errorf("blocked request must not reach the history query (entity=%q)", svc.gotEntityID)
			}
			if strings.Contains(rec.Body.String(), "leaked-secret-name") {
				t.Errorf("blocked request leaked history; body=%s", rec.Body)
			}
		})
	}
}

// TestEntityHistory_ScopesQueryEvenWithoutGuard pins the unconditional DB-level
// backstop: even if no view guard is wired, the handler still scopes the history
// query to the caller's campaign, so the cross-campaign IDOR stays closed.
func TestEntityHistory_ScopesQueryEvenWithoutGuard(t *testing.T) {
	svc := &fakeHistorySvc{}
	h := NewHandler(svc) // deliberately no SetEntityViewGuard

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/ent-9/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "ent-9")
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "c1"},
		MemberRole: campaigns.RolePlayer,
	})

	if err := h.EntityHistory(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.gotCampaignID != "c1" {
		t.Errorf("history query must always carry the caller's campaign scope; got %q", svc.gotCampaignID)
	}
}
