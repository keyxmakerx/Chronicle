// dm_grant_visibility_test.go pins that EntityHistory's IDOR/visibility
// guard passes the promoted cc.VisibilityRole(), not the raw
// int(cc.MemberRole), to ResolveEntityView — otherwise a Co-DM (Player +
// DM grant) allowed onto a dm_only entity's page gets 404'd loading that
// same entity's History panel.
//
// auditDmGrantGuard mirrors the real adapter's behavior (entities'
// CheckEntityAccess via ResolveEntityView): a dm_only (private) entity
// requires role>=RoleScribe (2) — the threshold VisibilityRole()'s Owner
// promotion (3) clears and int(cc.MemberRole) for a Player (1) does not.
package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type auditDmGrantGuard struct {
	campaignOf map[string]string
	private    map[string]bool
}

func (g auditDmGrantGuard) ResolveEntityView(_ context.Context, entityID string, role int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	if g.private[entityID] && role < int(campaigns.RoleScribe) {
		return camp, false, nil
	}
	return camp, true, nil
}

type auditDmGrantService struct {
	AuditService
	entries []AuditEntry
}

func (s auditDmGrantService) GetEntityHistory(_ context.Context, entityID, _ string) ([]AuditEntry, error) {
	return s.entries, nil
}

func auditCoDmContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
		IsMember:    true,
	}
}

// TestEntityHistory_CoDmReachesDmOnlyEntity pins the defect: a Co-DM
// requesting the history of a dm_only entity must get 200 with its history,
// not a 404 from the visibility guard.
func TestEntityHistory_CoDmReachesDmOnlyEntity(t *testing.T) {
	guard := auditDmGrantGuard{
		campaignOf: map[string]string{"dm-only-ent": "camp-1"},
		private:    map[string]bool{"dm-only-ent": true},
	}
	svc := auditDmGrantService{entries: []AuditEntry{{ID: 1, EntityID: "dm-only-ent", EntityName: "The Hidden Bunker"}}}
	h := NewHandler(svc)
	h.SetEntityViewGuard(guard)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/dm-only-ent/history", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "eid", Value: "dm-only-ent"}})
	c.Set("campaign_context", auditCoDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	err := h.EntityHistory(c)
	if err != nil {
		t.Fatalf("Co-DM fetching history of a dm_only entity: got %v, want nil (200)", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Co-DM fetching history of a dm_only entity: got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "The Hidden Bunker") {
		t.Errorf("Co-DM must see the dm_only entity's history; body=%s", rec.Body)
	}
}
