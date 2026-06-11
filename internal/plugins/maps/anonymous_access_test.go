// anonymous_access_test.go — public-campaign read access for the embeddable
// map widget (C-SWEEP-FIXES-R1 / cordinator#39 finding 4). Mirrors calendar's
// TestAnonymousAccess_PublicVsPrivate: the read-only map data routes are
// reachable anonymously on a PUBLIC campaign; on a PRIVATE campaign they bounce
// to /login; and fog / layers / writes bounce even on a public campaign.
package maps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type guardAuthSvc struct{ auth.AuthService } // ValidateSession never called: anonymous carries no token.

type guardAddonSvc struct{ addons.AddonService }

func (guardAddonSvc) IsEnabledForCampaign(_ context.Context, _, _ string) (bool, error) { return true, nil }

type guardCampaignSvc struct {
	campaigns.CampaignService
	public bool
}

func (m guardCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: m.public}, nil
}

// guardMapSvc / guardDrawingSvc embed the service interfaces and implement only
// the reads the anonymous request path reaches. GetMap returns a map in the
// path's campaign so the IDOR check passes; the list reads return empty (the
// handler reaches a 200, proving it got past auth).
type guardMapSvc struct {
	MapService
}

func (guardMapSvc) GetMap(_ context.Context, id string) (*Map, error) {
	return &Map{ID: id, CampaignID: "camp-1"}, nil
}
func (guardMapSvc) ListMarkers(_ context.Context, _ string, _ int, _ string) ([]Marker, error) {
	return []Marker{}, nil
}

type guardDrawingSvc struct {
	DrawingService
}

func (guardDrawingSvc) ListDrawings(_ context.Context, _ string, _ int) ([]Drawing, error) {
	return []Drawing{}, nil
}
func (guardDrawingSvc) ListTokens(_ context.Context, _ string, _ int) ([]Token, error) {
	return []Token{}, nil
}

func newGuardRouter(public bool) *echo.Echo {
	e := echo.New()
	e.Use(emw.Recover())
	campaignSvc := guardCampaignSvc{public: public}
	authSvc := guardAuthSvc{}
	addonSvc := guardAddonSvc{}
	mapSvc := guardMapSvc{}
	RegisterRoutes(e, NewHandler(mapSvc), campaignSvc, authSvc, addonSvc)
	RegisterDrawingRoutes(e, NewDrawingHandler(mapSvc, guardDrawingSvc{}), campaignSvc, authSvc, addonSvc)
	return e
}

func isLoginRedirect(rec *httptest.ResponseRecorder) bool {
	switch rec.Code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return rec.Header().Get("Location") == "/login"
	}
	return false
}

func TestMapsAnonymousAccess_PublicVsPrivate(t *testing.T) {
	const base = "/campaigns/camp-1/maps/m1"
	tests := []struct {
		name      string
		method    string
		path      string
		public    bool
		wantLogin bool
	}{
		// Read-only map data: reachable anonymously on a public campaign.
		{"meta public", http.MethodGet, base + "/meta", true, false},
		{"meta private", http.MethodGet, base + "/meta", false, true},
		{"markers public", http.MethodGet, base + "/markers", true, false},
		{"markers private", http.MethodGet, base + "/markers", false, true},
		{"drawings public", http.MethodGet, base + "/drawings", true, false},
		{"drawings private", http.MethodGet, base + "/drawings", false, true},
		{"tokens public", http.MethodGet, base + "/tokens", true, false},
		{"tokens private", http.MethodGet, base + "/tokens", false, true},

		// GM tools + writes bounce anonymous even on a public campaign.
		{"fog public (GM-only)", http.MethodGet, base + "/fog", true, true},
		{"layers public (GM-only)", http.MethodGet, base + "/layers", true, true},
		{"create marker public (write)", http.MethodPost, base + "/markers", true, true},
		{"create map public (write)", http.MethodPost, "/campaigns/camp-1/maps", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newGuardRouter(tt.public)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if got := isLoginRedirect(rec); got != tt.wantLogin {
				t.Errorf("anonymous %s %s (public=%v): login-redirect=%v (code=%d loc=%q), want %v",
					tt.method, tt.path, tt.public, got, rec.Code, rec.Header().Get("Location"), tt.wantLogin)
			}
		})
	}
}
