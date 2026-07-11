// anon_access_test.go — C-PUBLIC-VIEW-FIX-R2 content-level coverage.
//
// Drives real anonymous HTTP requests through the public-campaign middleware
// chain (auth.OptionalAuth + campaigns.AllowPublicCampaignAccess +
// campaigns.RequireViewAccess) into ListPosts, asserting the entity-privacy gate:
// a private entity's posts are never served to an anonymous visitor, and an
// entity ID belonging to another campaign is rejected (cross-campaign IDOR). A
// public entity is unchanged. Uses the default Echo error handler, so a denied
// request surfaces as a non-200 (the app's real handler maps NotFound → 404); the
// contract asserted here is "never a 200 posts payload".
package posts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type fakeAuthSvc struct{ auth.AuthService } // anon: ValidateSession never called.

type fakeCampaignSvc struct {
	campaigns.CampaignService
	public bool
}

func (m fakeCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: m.public}, nil
}

// fakeEntityGate maps entity IDs to their owning campaign + view decision.
type fakeEntityGate struct {
	campaignOf map[string]string
	canView    map[string]bool
}

func (g fakeEntityGate) ResolveViewableEntity(_ context.Context, entityID string, _ int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	return camp, g.canView[entityID], nil
}

type fakePostService struct {
	PostService
	posts []Post
}

func (s fakePostService) ListByEntity(_ context.Context, _, _ string, _ bool) ([]Post, error) {
	return s.posts, nil
}

func newPostsRouter(gate EntityGate) *echo.Echo {
	e := echo.New()
	e.Use(emw.Recover())
	h := NewHandler(fakePostService{posts: []Post{{ID: "p1", Name: "Spoiler Post"}}})
	h.SetEntityGate(gate)
	RegisterRoutes(e, h, fakeCampaignSvc{public: true}, fakeAuthSvc{})
	return e
}

func TestListPosts_AnonEntityPrivacyGate(t *testing.T) {
	gate := fakeEntityGate{
		campaignOf: map[string]string{
			"pub-ent":     "camp-1", // public entity in the URL campaign
			"priv-ent":    "camp-1", // private entity in the URL campaign
			"foreign-ent": "camp-2", // entity in ANOTHER campaign (IDOR probe)
		},
		canView: map[string]bool{"pub-ent": true, "priv-ent": false, "foreign-ent": true},
	}
	router := newPostsRouter(gate)

	get := func(eid string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/"+eid+"/posts", nil)
		router.ServeHTTP(rec, req)
		return rec
	}

	t.Run("public entity → posts served", func(t *testing.T) {
		rec := get("pub-ent")
		if rec.Code != http.StatusOK {
			t.Fatalf("public entity posts: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Spoiler Post") {
			t.Errorf("public entity posts payload missing: %q", rec.Body.String())
		}
	})

	t.Run("private entity → never served to anon", func(t *testing.T) {
		rec := get("priv-ent")
		if rec.Code == http.StatusOK {
			t.Errorf("private-entity posts leaked to anon (got 200): %q", rec.Body.String())
		}
	})

	t.Run("foreign-campaign entity → 404 (kills IDOR)", func(t *testing.T) {
		rec := get("foreign-ent")
		if rec.Code == http.StatusOK {
			t.Errorf("cross-campaign IDOR: foreign entity posts served (got 200): %q", rec.Body.String())
		}
	})
}

// TestListPosts_ViewerCanSeeIsServed pins that the gate does not over-shoot: a
// viewer the gate reports CAN view a (private) entity is served its posts — the
// handler honors the gate's decision exactly, like the entity Show page.
func TestListPosts_ViewerCanSeeIsServed(t *testing.T) {
	gate := fakeEntityGate{
		campaignOf: map[string]string{"priv-ent": "camp-1"},
		canView:    map[string]bool{"priv-ent": true}, // a DM/member can view it
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/priv-ent/posts", nil)
	newPostsRouter(gate).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer with view access got %d, want 200 (over-shoot?): %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Spoiler Post") {
		t.Errorf("expected posts payload for a permitted viewer, got %q", rec.Body.String())
	}
}
