// backlinks_scope_test.go — C-SWEEP-R3.
//
// GET /campaigns/:id/entities/:eid/backlinks is a PUBLIC-CAPABLE read route
// (routes.go: AllowPublicCampaignAccess + RequireViewAccess). Those two
// middlewares only resolve/authorize the CAMPAIGN — neither looks at :eid — so
// the handler owns the entity-side check, exactly like Show / PreviewAPI /
// GetAliasesAPI. Before this fix it had neither the campaign-match 404 nor
// CheckEntityAccess, and the repository query had no campaign predicate, so an
// anonymous visitor to ANY public campaign could name a private campaign's
// entity id and receive that campaign's referencing entities in full
// (including their entry_html). These pin both halves.
package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// backlinkGuardSvc is a stub EntityService whose entity lives in
// entityCampaign; it records the campaign id the handler scopes the read to.
type backlinkGuardSvc struct {
	EntityService
	entityCampaign string
	canView        bool
	gotCampaignID  string
}

func (m *backlinkGuardSvc) GetByID(_ context.Context, id string) (*Entity, error) {
	return &Entity{ID: id, CampaignID: m.entityCampaign}, nil
}

func (m *backlinkGuardSvc) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: m.canView}, nil
}

func (m *backlinkGuardSvc) GetBacklinksWithSnippets(_ context.Context, campaignID, _ string, _ int, _ string) ([]BacklinkEntry, error) {
	m.gotCampaignID = campaignID
	snippet := "The baron hid the key beneath the chapel floor."
	return []BacklinkEntry{{Entity: Entity{ID: "mentioner-1", CampaignID: m.entityCampaign, Name: "Baron Vex's Secret Ledger"}, Snippet: snippet}}, nil
}

// TestBacklinksAnonymousCrossCampaign pins that the backlinks read refuses an
// entity that does not belong to the campaign in the URL, and that a viewer who
// may not view the entity gets nothing either. The handler returns
// apperror.NewNotFound; the bare Echo error handler used here surfaces that as
// a non-200, so the contract asserted is simply: never a 200 backlinks payload.
func TestBacklinksAnonymousCrossCampaign(t *testing.T) {
	newRouter := func(svc *backlinkGuardSvc) *echo.Echo {
		e := echo.New()
		e.Use(emw.Recover())
		RegisterRoutes(e, NewHandler(svc), guardCampaignSvc{public: true}, guardAuthSvc{})
		return e
	}

	t.Run("entity from another campaign is not served", func(t *testing.T) {
		svc := &backlinkGuardSvc{entityCampaign: "camp-victim", canView: true}
		rec := httptest.NewRecorder()
		newRouter(svc).ServeHTTP(rec,
			httptest.NewRequest(http.MethodGet, "/campaigns/camp-public/entities/e-victim/backlinks", nil))
		if rec.Code == http.StatusOK {
			t.Errorf("cross-campaign backlinks must not be returned (got 200: %q)", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "Baron Vex") {
			t.Errorf("other campaign's entity leaked in body: %q", rec.Body.String())
		}
	})

	t.Run("entity the viewer may not view is not served", func(t *testing.T) {
		svc := &backlinkGuardSvc{entityCampaign: "camp-public", canView: false}
		rec := httptest.NewRecorder()
		newRouter(svc).ServeHTTP(rec,
			httptest.NewRequest(http.MethodGet, "/campaigns/camp-public/entities/e1/backlinks", nil))
		if rec.Code == http.StatusOK {
			t.Errorf("backlinks of a non-viewable entity must not be returned (got 200: %q)", rec.Body.String())
		}
	})

	t.Run("in-campaign entity is served, scoped to the URL campaign", func(t *testing.T) {
		svc := &backlinkGuardSvc{entityCampaign: "camp-public", canView: true}
		rec := httptest.NewRecorder()
		newRouter(svc).ServeHTTP(rec,
			httptest.NewRequest(http.MethodGet, "/campaigns/camp-public/entities/e1/backlinks", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("in-campaign backlinks must be served; got %d (%q)", rec.Code, rec.Body.String())
		}
		if svc.gotCampaignID != "camp-public" {
			t.Errorf("backlinks read scoped to campaign %q, want the URL campaign %q", svc.gotCampaignID, "camp-public")
		}
	})
}

// TestBacklinksWhereIsCampaignScoped pins the SQL half without a database: the
// backlinks WHERE clause must carry `e.campaign_id = ?` with the campaign id as
// its first bound arg. Mention ids are campaign-agnostic UUIDs, so a query
// without this term matches entry_html in EVERY campaign — and for an Owner
// viewer visibilityFilter contributes an EMPTY fragment, so nothing else
// constrains the rows.
func TestBacklinksWhereIsCampaignScoped(t *testing.T) {
	for _, role := range []int{permissions.RoleNone, permissions.RolePlayer, permissions.RoleOwner} {
		where, args := backlinksWhere("camp-1", "target-1", role, "user-9")
		if !strings.Contains(where, "e.campaign_id = ?") {
			t.Errorf("role %d: backlinks WHERE is missing the campaign scope:\n%s", role, where)
		}
		if len(args) == 0 || args[0] != "camp-1" {
			t.Errorf("role %d: first bound arg = %v, want the campaign id", role, args)
		}
		// The mention-pattern + self-exclusion terms must survive alongside it.
		for _, want := range []string{`e.entry_html LIKE ?`, `e.id != ?`} {
			if !strings.Contains(where, want) {
				t.Errorf("role %d: backlinks WHERE lost %q:\n%s", role, want, where)
			}
		}
	}
}
