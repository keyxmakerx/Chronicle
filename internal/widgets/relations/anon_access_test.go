// anon_access_test.go — C-PUBLIC-VIEW-FIX-R2 content-level coverage.
//
// Drives real anonymous HTTP requests through the public-campaign middleware
// chain into ListRelations, asserting: (1) the source-entity gate — a private
// source entity's relations are never served to anon, and a foreign-campaign
// entity ID is rejected (IDOR); (2) target filtering — a relation whose TARGET
// the viewer cannot see is dropped (its name/slug are the leak) while visible
// targets remain. GraphAPI node filtering is covered at the service level in
// service_test.go (TestGetFilteredGraphData_HidesPrivateNodes).
package relations

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

type relFakeAuthSvc struct{ auth.AuthService }

type relFakeCampaignSvc struct {
	campaigns.CampaignService
	public bool
}

func (m relFakeCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: m.public}, nil
}

// relFakeEntityGate resolves the source entity + filters target IDs by
// visibility. `viewable` is the set of entity IDs the viewer may see.
type relFakeEntityGate struct {
	campaignOf map[string]string
	canView    map[string]bool
	viewable   map[string]bool
}

func (g relFakeEntityGate) ResolveViewableEntity(_ context.Context, entityID string, _ int, _ string) (string, bool, error) {
	camp, ok := g.campaignOf[entityID]
	if !ok {
		return "", false, apperror.NewNotFound("entity not found")
	}
	return camp, g.canView[entityID], nil
}

func (g relFakeEntityGate) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, _ int, _ string) (map[string]bool, error) {
	out := make(map[string]bool)
	for _, id := range ids {
		if g.viewable[id] {
			out[id] = true
		}
	}
	return out, nil
}

type fakeRelService struct {
	RelationService
	rels []Relation
}

func (s fakeRelService) ListByEntity(_ context.Context, _, _ string) ([]Relation, error) {
	return s.rels, nil
}

func newRelationsRouter(gate EntityGate, rels []Relation) *echo.Echo {
	e := echo.New()
	e.Use(emw.Recover())
	h := NewHandler(fakeRelService{rels: rels})
	h.SetEntityGate(gate)
	RegisterRoutes(e, h, relFakeCampaignSvc{public: true}, relFakeAuthSvc{})
	return e
}

func TestListRelations_AnonSourceGate(t *testing.T) {
	gate := relFakeEntityGate{
		campaignOf: map[string]string{"pub-ent": "camp-1", "priv-ent": "camp-1", "foreign-ent": "camp-2"},
		canView:    map[string]bool{"pub-ent": true, "priv-ent": false, "foreign-ent": true},
		viewable:   map[string]bool{}, // no targets needed for the source-gate cases
	}
	router := newRelationsRouter(gate, nil)

	get := func(eid string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/"+eid+"/relations", nil))
		return rec
	}

	if rec := get("pub-ent"); rec.Code != http.StatusOK {
		t.Errorf("public source entity: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if rec := get("priv-ent"); rec.Code == http.StatusOK {
		t.Errorf("private source entity relations leaked to anon (got 200): %q", rec.Body.String())
	}
	if rec := get("foreign-ent"); rec.Code == http.StatusOK {
		t.Errorf("cross-campaign IDOR: foreign source relations served (got 200): %q", rec.Body.String())
	}
}

func TestListRelations_AnonTargetFiltering(t *testing.T) {
	rels := []Relation{
		{ID: 1, SourceEntityID: "pub-ent", TargetEntityID: "vis-target", TargetEntityName: "Visible Ally", DmOnly: false},
		{ID: 2, SourceEntityID: "pub-ent", TargetEntityID: "priv-target", TargetEntityName: "Secret Villain", DmOnly: false},
		{ID: 3, SourceEntityID: "pub-ent", TargetEntityID: "vis-target", TargetEntityName: "Visible Ally", DmOnly: true}, // dm_only → also dropped for anon
	}
	gate := relFakeEntityGate{
		campaignOf: map[string]string{"pub-ent": "camp-1"},
		canView:    map[string]bool{"pub-ent": true},
		viewable:   map[string]bool{"vis-target": true}, // priv-target NOT viewable
	}
	rec := httptest.NewRecorder()
	newRelationsRouter(gate, rels).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/pub-ent/relations", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("anon list on public source: got %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Visible Ally") {
		t.Errorf("visible-target relation missing from anon payload: %q", body)
	}
	if strings.Contains(body, "Secret Villain") {
		t.Errorf("private-target relation name leaked to anon: %q", body)
	}
}
