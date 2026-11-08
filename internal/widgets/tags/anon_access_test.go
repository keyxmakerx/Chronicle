// anon_access_test.go pins the anonymous-access gate on the per-entity tag
// read (GET /campaigns/:id/entities/:eid/tags): a private entity's tags must
// not be served, a foreign-campaign entity must be rejected (IDOR), and a
// public entity's tags must be served unchanged.
package tags

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	emw "github.com/labstack/echo/v5/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type tagFakeAuthSvc struct{ auth.AuthService }

type tagFakeCampaignSvc struct {
	campaigns.CampaignService
	public bool
}

func (m tagFakeCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: m.public}, nil
}

type tagFakeEntityGate struct {
	campaignOf map[string]string
	canView    map[string]bool
}

func (g tagFakeEntityGate) ResolveViewableEntity(_ context.Context, entityID string, _ int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	return camp, g.canView[entityID], nil
}

type fakeTagService struct {
	TagService
	tags []Tag
}

func (s fakeTagService) GetEntityTags(_ context.Context, _ string, _ bool) ([]Tag, error) {
	return s.tags, nil
}

func newTagsRouter(gate EntityGate) *echo.Echo {
	e := echo.New()
	e.Use(emw.Recover())
	h := NewHandler(fakeTagService{tags: []Tag{{ID: 1, Name: "Secret BBEG"}}})
	h.SetEntityGate(gate)
	RegisterRoutes(e, h, tagFakeCampaignSvc{public: true}, tagFakeAuthSvc{})
	return e
}

func TestGetEntityTags_AnonEntityPrivacyGate(t *testing.T) {
	gate := tagFakeEntityGate{
		campaignOf: map[string]string{"pub-ent": "camp-1", "priv-ent": "camp-1", "foreign-ent": "camp-2"},
		canView:    map[string]bool{"pub-ent": true, "priv-ent": false, "foreign-ent": true},
	}
	router := newTagsRouter(gate)

	get := func(eid string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/"+eid+"/tags", nil))
		return rec
	}

	t.Run("public entity → tags served", func(t *testing.T) {
		rec := get("pub-ent")
		if rec.Code != http.StatusOK {
			t.Fatalf("public entity tags: got %d, want 200 (%s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Secret BBEG") {
			t.Errorf("public entity tags payload missing: %q", rec.Body.String())
		}
	})

	t.Run("private entity → tag names never served to anon", func(t *testing.T) {
		if rec := get("priv-ent"); rec.Code == http.StatusOK {
			t.Errorf("private-entity tag names leaked to anon (got 200): %q", rec.Body.String())
		}
	})

	t.Run("foreign-campaign entity → 404 (kills IDOR)", func(t *testing.T) {
		if rec := get("foreign-ent"); rec.Code == http.StatusOK {
			t.Errorf("cross-campaign IDOR: foreign entity tags served (got 200): %q", rec.Body.String())
		}
	})
}

// TestGetEntityTags_NilGateFailsClosed pins fail-closed behavior: a handler
// wired without its EntityGate must return 5xx, never serve tags.
func TestGetEntityTags_NilGateFailsClosed(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/any-ent/tags", nil)
	newTagsRouter(nil).ServeHTTP(rec, req)
	if rec.Code < 500 {
		t.Errorf("tags handler with no EntityGate wired must fail closed (5xx), got %d: %q", rec.Code, rec.Body.String())
	}
}
