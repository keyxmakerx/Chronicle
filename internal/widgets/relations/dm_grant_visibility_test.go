// dm_grant_visibility_test.go pins that ListRelations's source-entity gate
// (h.entityGate.ResolveViewableEntity) uses cc.VisibilityRole(), not the raw
// int(cc.MemberRole) — otherwise a Co-DM (Player + DM grant) gets 404'd
// before the "privileged" content branch's cc.IsDmGranted check can admit
// them to the full, unfiltered relation list for a dm_only entity (ADR-057).
//
// relDmGrantGate mirrors the real adapter's default-mode visibility rule: a
// dm_only (private) source entity requires role>=RoleScribe (2), the actual
// threshold VisibilityRole()'s Owner promotion (3) clears and a raw Player
// role (1) does not.
package relations

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

// relDmGrantGate resolves the source entity and filters target visibility
// using the same role>=RoleScribe threshold entities.visibilityFilter applies
// to a dm_only entity.
type relDmGrantGate struct {
	campaignOf map[string]string
	private    map[string]bool
}

func (g relDmGrantGate) ResolveViewableEntity(_ context.Context, entityID string, role int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	if g.private[entityID] && role < int(campaigns.RoleScribe) {
		return camp, false, nil
	}
	return camp, true, nil
}

func (g relDmGrantGate) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, role int, _ string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !g.private[id] || role >= int(campaigns.RoleScribe) {
			out[id] = true
		}
	}
	return out, nil
}

type relDmGrantService struct {
	RelationService
	rels []Relation
}

func (s relDmGrantService) ListByEntity(_ context.Context, _, _ string) ([]Relation, error) {
	return s.rels, nil
}

// relCoDmContext is a Player MemberRole with an Owner-granted dm_only
// visibility grant — the Co-DM shape the defect hits.
func relCoDmContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
		IsMember:    true,
	}
}

// TestListRelations_CoDmReachesDmOnlySourceEntity pins the defect: a Co-DM
// requesting the relations of a dm_only entity must get 200 with the full
// (unfiltered — the privileged branch already honors IsDmGranted) relation
// list, not a 404 from the source gate.
func TestListRelations_CoDmReachesDmOnlySourceEntity(t *testing.T) {
	gate := relDmGrantGate{
		campaignOf: map[string]string{"dm-only-ent": "camp-1"},
		private:    map[string]bool{"dm-only-ent": true},
	}
	rels := []Relation{
		{ID: 1, SourceEntityID: "dm-only-ent", TargetEntityID: "t1", TargetEntityName: "The Secret Cabal", DmOnly: true},
	}
	h := NewHandler(relDmGrantService{rels: rels})
	h.SetEntityGate(gate)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/dm-only-ent/relations", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "eid", Value: "dm-only-ent"}})
	c.Set("campaign_context", relCoDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	err := h.ListRelations(c)
	if err != nil {
		t.Fatalf("Co-DM fetching relations of a dm_only entity: got %v, want nil (200)", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Co-DM fetching relations of a dm_only entity: got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "The Secret Cabal") {
		t.Errorf("Co-DM must see the dm_only relation content; body=%s", rec.Body)
	}
}
