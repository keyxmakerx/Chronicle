// dm_grant_visibility_test.go pins that ListPosts's entity-privacy gate
// (h.entityGate.ResolveViewableEntity) uses cc.VisibilityRole(), not the raw
// int(cc.MemberRole) — otherwise a Co-DM (Player + DM grant) gets 404'd
// before the content branch's cc.IsDmGranted check can admit them (ADR-057).
//
// postsDmGrantGate mirrors the real adapter's default-mode visibility rule: a
// dm_only (private) entity requires role>=RoleScribe (2), a threshold
// VisibilityRole()'s Owner promotion (3) clears and a raw Player role (1)
// does not.
package posts

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

type postsDmGrantGate struct {
	campaignOf map[string]string
	private    map[string]bool
}

func (g postsDmGrantGate) ResolveViewableEntity(_ context.Context, entityID string, role int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	if g.private[entityID] && role < int(campaigns.RoleScribe) {
		return camp, false, nil
	}
	return camp, true, nil
}

type postsDmGrantService struct {
	PostService
	posts []Post
}

func (s postsDmGrantService) ListByEntity(_ context.Context, _, _ string, _ bool) ([]Post, error) {
	return s.posts, nil
}

func postsCoDmContext() *campaigns.CampaignContext {
	return &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
		IsMember:    true,
	}
}

// TestListPosts_CoDmReachesDmOnlyEntity pins the defect: a Co-DM requesting
// the posts of a dm_only entity must get 200 with its dm_only posts, not a
// 404 from the entity-privacy gate.
func TestListPosts_CoDmReachesDmOnlyEntity(t *testing.T) {
	gate := postsDmGrantGate{
		campaignOf: map[string]string{"dm-only-ent": "camp-1"},
		private:    map[string]bool{"dm-only-ent": true},
	}
	posts := []Post{{ID: "p1", Name: "The Baron's Secret Ledger", IsPrivate: true}}
	h := NewHandler(postsDmGrantService{posts: posts})
	h.SetEntityGate(gate)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/dm-only-ent/posts", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "eid", Value: "dm-only-ent"}})
	c.Set("campaign_context", postsCoDmContext())
	auth.SetSession(c, &auth.Session{UserID: "codm-1"})

	err := h.ListPosts(c)
	if err != nil {
		t.Fatalf("Co-DM fetching posts of a dm_only entity: got %v, want nil (200)", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("Co-DM fetching posts of a dm_only entity: got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "The Baron's Secret Ledger") {
		t.Errorf("Co-DM must see the dm_only post content; body=%s", rec.Body)
	}
}
