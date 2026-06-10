package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Anonymous-access guard for cordinator#30. This bug class keeps regressing —
// a view route lands in the authenticated route group instead of the
// public-capable one, so logged-out visitors to a PUBLIC campaign get bounced
// to /login (the V1→V2 calendar cutover left the V2 view routes in the
// authenticated group). The test wires the REAL campaigns + calendar routes
// and asserts the auth boundary anonymously, public vs private, so a future
// mis-grouping fails here instead of in production.
//
// The mocks embed each service interface so only the methods the anonymous
// request path actually invokes need a body. View handlers are made to fail
// fast (the calendar stub returns NotFound from the first method they call)
// so the test observes the *auth* outcome — redirect-to-login vs
// reached-the-handler — without standing up real calendar data.

type guardAuthSvc struct{ auth.AuthService } // ValidateSession is never called: anonymous requests carry no token.

type guardAddonSvc struct{ addons.AddonService }

func (guardAddonSvc) IsEnabledForCampaign(_ context.Context, _ string, _ string) (bool, error) {
	return true, nil
}

type guardCampaignSvc struct {
	campaigns.CampaignService
	public bool
}

func (m guardCampaignSvc) GetByID(_ context.Context, id string) (*campaigns.Campaign, error) {
	return &campaigns.Campaign{ID: id, IsPublic: m.public}, nil
}

func (guardCampaignSvc) GetPendingTransfer(_ context.Context, _ string) (*campaigns.OwnershipTransfer, error) {
	return nil, nil
}

type guardCalSvc struct {
	CalendarService
}

func (guardCalSvc) ListVisibleCalendars(_ context.Context, _ string, _ int, _ string) ([]Calendar, error) {
	return nil, apperror.NewNotFound("stub")
}

func (guardCalSvc) GetActiveCalendar(_ context.Context, _ string, _ string) (*Calendar, error) {
	return nil, apperror.NewNotFound("stub")
}

// newGuardRouter builds an Echo with the real campaigns + calendar route
// registration against the mocks. public controls whether the resolved
// campaign reports IsPublic.
func newGuardRouter(public bool) *echo.Echo {
	e := echo.New()
	e.Use(emw.Recover()) // a handler panic on the reached-handler path becomes 500, not a test crash.

	campaignSvc := guardCampaignSvc{public: public}
	authSvc := guardAuthSvc{}
	addonSvc := guardAddonSvc{}

	campaigns.RegisterRoutes(e, campaigns.NewHandler(campaignSvc), campaignSvc, authSvc)
	RegisterRoutes(e, NewHandler(guardCalSvc{}), campaignSvc, authSvc, addonSvc)
	return e
}

// isLoginRedirect reports whether the response is a redirect to the login page —
// the exact signature of the bug (private/unauthenticated → /login).
func isLoginRedirect(rec *httptest.ResponseRecorder) bool {
	switch rec.Code {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return rec.Header().Get("Location") == "/login"
	}
	return false
}

func TestAnonymousAccess_PublicVsPrivate(t *testing.T) {
	const cid = "camp-1"
	tests := []struct {
		name      string
		method    string
		path      string
		public    bool
		wantLogin bool // anonymous request should be redirected to /login
	}{
		// Campaign root — public-capable view route.
		{"campaign root, public", http.MethodGet, "/campaigns/" + cid, true, false},
		{"campaign root, private", http.MethodGet, "/campaigns/" + cid, false, true},

		// Calendar V2 shell — the surface that regressed (read view).
		{"calendar v2, public", http.MethodGet, "/campaigns/" + cid + "/calendar/v2", true, false},
		{"calendar v2, private", http.MethodGet, "/campaigns/" + cid + "/calendar/v2", false, true},
		{"calendar v2 explicit cal, public", http.MethodGet, "/campaigns/" + cid + "/calendar/v2/cal-1", true, false},
		{"calendar v2 explicit cal, private", http.MethodGet, "/campaigns/" + cid + "/calendar/v2/cal-1", false, true},

		// World-state seed GET — lazy-loaded read on public calendar/embed surfaces.
		{"world-state GET, public", http.MethodGet, "/campaigns/" + cid + "/calendar/world-state", true, false},
		{"world-state GET, private", http.MethodGet, "/campaigns/" + cid + "/calendar/world-state", false, true},

		// Mutating / per-user route stays authenticated even on a public campaign.
		{"calendar v2 switch POST, public", http.MethodPost, "/campaigns/" + cid + "/calendar/v2/switch", true, true},
		{"calendar v2 switch POST, private", http.MethodPost, "/campaigns/" + cid + "/calendar/v2/switch", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newGuardRouter(tt.public)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if got := isLoginRedirect(rec); got != tt.wantLogin {
				t.Fatalf("anonymous %s %s (public=%v): login-redirect=%v (code=%d location=%q), want login-redirect=%v",
					tt.method, tt.path, tt.public, got, rec.Code, rec.Header().Get("Location"), tt.wantLogin)
			}
		})
	}
}
