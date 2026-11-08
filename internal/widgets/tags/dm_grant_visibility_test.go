// dm_grant_visibility_test.go pins that a Co-DM (Player + DM grant) can see
// dm_only tags: GetEntityTags's entity-privacy gate must use the promoted
// cc.VisibilityRole(), not the raw int(cc.MemberRole), or it 404s the Co-DM
// before canSeeDmOnly's own IsDmGranted check ever runs (ADR-057).
//
// tagsDmGrantGate mirrors the real adapter (entities' CheckEntityAccess via
// ResolveViewableEntity): a dm_only (private) entity requires role >=
// RoleScribe (2), which VisibilityRole()'s Owner promotion (3) clears but a
// raw Player role (1) does not.
package tags

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

type tagsDmGrantGate struct {
	campaignOf map[string]string
	private    map[string]bool
}

func (g tagsDmGrantGate) ResolveViewableEntity(_ context.Context, entityID string, role int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	if g.private[entityID] && role < int(campaigns.RoleScribe) {
		return camp, false, nil
	}
	return camp, true, nil
}

type tagsDmGrantService struct {
	TagService
	tags []Tag
}

func (s tagsDmGrantService) GetEntityTags(_ context.Context, _ string, _ bool) ([]Tag, error) {
	return s.tags, nil
}

func tagsCoDmContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
		IsMember:    true,
	}
}

// TestGetEntityTags_CoDmReachesDmOnlyEntity pins the defect: a Co-DM
// requesting the tags of a dm_only entity must get 200 with its tags, not a
// 404 from the entity-privacy gate.
func TestGetEntityTags_CoDmReachesDmOnlyEntity(t *testing.T) {
	gate := tagsDmGrantGate{
		campaignOf: map[string]string{"dm-only-ent": "camp-1"},
		private:    map[string]bool{"dm-only-ent": true},
	}
	tags := []Tag{{ID: 1, Name: "Secret Faction", DmOnly: true}}
	h := NewHandler(tagsDmGrantService{tags: tags})
	h.SetEntityGate(gate)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/dm-only-ent/tags", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "eid", Value: "dm-only-ent"}})
	c.Set("campaign_context", tagsCoDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	err := h.GetEntityTags(c)
	if err != nil {
		t.Fatalf("Co-DM fetching tags of a dm_only entity: got %v, want nil (200)", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Co-DM fetching tags of a dm_only entity: got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Secret Faction") {
		t.Errorf("Co-DM must see the dm_only tag content; body=%s", rec.Body)
	}
}
