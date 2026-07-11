package npcs

// public_route_test.go — C-PUBLIC-VIEW-FIX real-route wiring guard.
//
// The composed-chain unit tests in campaigns/public_view_access_test.go pin that
// AllowPublicCampaignAccess + RequireViewAccess admits public visitors. THIS test
// pins that the actual REGISTERED pub routes wire that gate — it registers npcs'
// real routes and drives GET /campaigns/:id/npcs* as an anonymous visitor. If a
// future edit re-wires a pub route back to RequireRole(RolePlayer) (the original
// regression), this test flips to 403 and fails. npcs is the representative pub
// plugin (its two pub routes share the identical group construction as every
// other pub plugin: OptionalAuth -> AllowPublicCampaignAccess -> RequireAddon ->
// RequireViewAccess).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- minimal stubs: embed each interface, override only what the pub chain calls ---

type stubNPCSvc struct{}

func (stubNPCSvc) ListNPCs(context.Context, string, int, string, NPCListOptions) ([]NPCCard, int, error) {
	return nil, 0, nil
}
func (stubNPCSvc) CountNPCs(context.Context, string, int, string) (int, error) { return 0, nil }

type stubCampaignSvc struct {
	campaigns.CampaignService
	campaign *campaigns.Campaign
}

func (s stubCampaignSvc) GetByID(context.Context, string) (*campaigns.Campaign, error) {
	return s.campaign, nil
}

// GetMember is not reached for an anonymous visitor (AllowPublicCampaignAccess
// takes the unauthenticated branch), but stub it defensively.
func (s stubCampaignSvc) GetMember(context.Context, string, string) (*campaigns.CampaignMember, error) {
	return nil, apperror.NewNotFound("not a member")
}

type stubAuthSvc struct{ auth.AuthService } // no token for anon => no method is called

type stubAddonSvc struct {
	addons.AddonService
	enabled bool
}

func (s stubAddonSvc) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return s.enabled, nil
}

func serveNPCRoute(t *testing.T, path string, campaign *campaigns.Campaign) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}

	RegisterRoutes(e, NewHandler(stubNPCSvc{}),
		stubCampaignSvc{campaign: campaign},
		stubAuthSvc{},
		stubAddonSvc{enabled: true},
	)

	req := httptest.NewRequest(http.MethodGet, path, nil) // no session cookie => anonymous
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// TestNPCPubRoutes_AnonymousAdmittedOnPublic pins that the registered npcs pub
// routes admit an anonymous visitor to a PUBLIC campaign — i.e. the gate is
// RequireViewAccess, not RequireRole(RolePlayer) (the production regression).
//
//   - /npcs/count runs its handler and returns 200 (unambiguous: gate passed).
//   - /npcs (h.Index) is an unconditional deprecation redirect to /characters
//     (the gallery folded into the Characters page). A 302 to /characters — as
//     opposed to a 403 or a bounce to /login — proves the gate ADMITTED the anon
//     visitor and the handler ran. (The /characters target is itself on the
//     auth-required group, out of this fix's pub-swap scope; public character
//     CONTENT is served via the entities pub routes.)
func TestNPCPubRoutes_AnonymousAdmittedOnPublic(t *testing.T) {
	pub := &campaigns.Campaign{ID: "camp-1", IsPublic: true}

	t.Run("/npcs/count → 200", func(t *testing.T) {
		rec := serveNPCRoute(t, "/campaigns/camp-1/npcs/count", pub)
		if rec.Code != http.StatusOK {
			t.Fatalf("anon GET /npcs/count on a PUBLIC campaign = %d, want 200 (route must use RequireViewAccess, not RequireRole)", rec.Code)
		}
	})

	t.Run("/npcs → 302 to /characters (gate passed, not /login)", func(t *testing.T) {
		rec := serveNPCRoute(t, "/campaigns/camp-1/npcs", pub)
		if rec.Code != http.StatusFound {
			t.Fatalf("anon GET /npcs on a PUBLIC campaign = %d, want 302 (deprecation redirect); a 403 would mean the gate rejected anon", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/campaigns/camp-1/characters" {
			t.Errorf("redirect Location = %q, want /campaigns/camp-1/characters — proves the gate admitted the anon visitor into the handler", loc)
		}
	})
}

// TestNPCPubRoutes_AnonymousRedirectedOnPrivate confirms the gate still protects
// PRIVATE campaigns: an anonymous visitor is redirected to /login (302), never
// admitted. Proves the fix widened access exactly to public campaigns, no more.
func TestNPCPubRoutes_AnonymousRedirectedOnPrivate(t *testing.T) {
	priv := &campaigns.Campaign{ID: "camp-1", IsPublic: false}
	rec := serveNPCRoute(t, "/campaigns/camp-1/npcs/count", priv)
	if rec.Code != http.StatusFound {
		t.Fatalf("anon GET on a PRIVATE campaign = %d, want 302 redirect", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("redirect Location = %q, want /login", loc)
	}
}
