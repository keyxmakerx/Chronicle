// entity_vis_parity_test.go pins that entities' anon-reachable data
// endpoints (GetEntry, GetFieldsAPI, PreviewAPI, GetAliasesAPI) gate on
// CheckEntityAccess, not the legacy `IsPrivate && role < RoleScribe` check —
// which ignores visibility='custom' and would wrongly serve an entity
// flipped to custom with restrictive grants.
//
// Drives real anonymous HTTP requests through the public-campaign
// middleware chain, asserting: a custom-restricted entity is never served
// to anon, a foreign-campaign entity ID is rejected (cross-campaign IDOR),
// and a viewable entity is unchanged (never a 200 for restricted/foreign).
package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type visFakeAuthSvc struct{ auth.AuthService } // anon: ValidateSession never called.

type visFakeCampaignSvc struct {
	campaigns.CampaignService
	public bool
}

func (m visFakeCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: m.public}, nil
}

// visFakeEntitySvc models entities as visibility='custom' with
// is_private=false, the state where the legacy `IsPrivate && role<Scribe`
// gate would wrongly pass and only CheckEntityAccess correctly denies.
// campaignOf drives the IDOR check; canView drives CheckEntityAccess.
type visFakeEntitySvc struct {
	EntityService
	campaignOf map[string]string
	canView    map[string]bool
}

func (s visFakeEntitySvc) GetByID(_ context.Context, id string) (*Entity, error) {
	camp, ok := s.campaignOf[id]
	if !ok {
		return nil, apperror.NewNotFound("entity not found")
	}
	return &Entity{ID: id, CampaignID: camp, Visibility: VisibilityCustom, IsPrivate: false}, nil
}

// A page the viewer sees is editable from Scribe up, as in default visibility.
func (s visFakeEntitySvc) CheckEntityAccess(_ context.Context, entityID string, role int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: s.canView[entityID], CanEdit: s.canView[entityID] && role >= int(campaigns.RoleScribe)}, nil
}

func (visFakeEntitySvc) GetEntityTypeByID(_ context.Context, _ int) (*EntityType, error) {
	return &EntityType{}, nil
}

func (visFakeEntitySvc) GetAliases(_ context.Context, _ string) ([]EntityAlias, error) {
	return []EntityAlias{}, nil
}

func newVisParityRouter(svc EntityService) *echo.Echo {
	e := echo.New()
	e.Use(emw.Recover())
	RegisterRoutes(e, NewHandler(svc), visFakeCampaignSvc{public: true}, visFakeAuthSvc{})
	return e
}

// TestEntityDataEndpoints_AnonCustomVisibilityGate drives every anon-reachable
// entity-data endpoint against a custom-restricted entity, a viewable entity,
// and a foreign-campaign entity, asserting the canonical gate at each.
func TestEntityDataEndpoints_AnonCustomVisibilityGate(t *testing.T) {
	svc := visFakeEntitySvc{
		campaignOf: map[string]string{
			"pub-ent":     "camp-1", // gate admits this viewer
			"priv-ent":    "camp-1", // custom-restricted: gate denies (the leak fix)
			"foreign-ent": "camp-2", // another campaign (IDOR probe)
		},
		canView: map[string]bool{"pub-ent": true, "priv-ent": false, "foreign-ent": true},
	}
	router := newVisParityRouter(svc)

	// The four anon-reachable, entity-scoped data endpoints, by URL suffix.
	suffixes := []string{"/entry", "/fields", "/preview", "/aliases"}

	get := func(eid, suffix string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/entities/"+eid+suffix, nil)
		router.ServeHTTP(rec, req)
		return rec
	}

	for _, suffix := range suffixes {
		t.Run("viewable entity"+suffix+" → served", func(t *testing.T) {
			if rec := get("pub-ent", suffix); rec.Code != http.StatusOK {
				t.Fatalf("gate-admitted viewer got %d, want 200 (over-shoot?): %s", rec.Code, rec.Body.String())
			}
		})
		t.Run("custom-restricted entity"+suffix+" → never served to anon", func(t *testing.T) {
			if rec := get("priv-ent", suffix); rec.Code == http.StatusOK {
				t.Errorf("custom-restricted entity leaked to anon (got 200): %q", rec.Body.String())
			}
		})
		t.Run("foreign-campaign entity"+suffix+" → 404 (kills IDOR)", func(t *testing.T) {
			if rec := get("foreign-ent", suffix); rec.Code == http.StatusOK {
				t.Errorf("cross-campaign IDOR: foreign entity served (got 200): %q", rec.Body.String())
			}
		})
	}
}
