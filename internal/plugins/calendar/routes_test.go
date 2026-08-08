package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

		// THE BENCH — public-capable since C-CALV4-V2SUNSET R2-4 ([VS-4] SIGNED).
		//
		// EXTENDED, NOT REPLACED. Every V2 row above stays: those routes still
		// exist, the claims are still true, and deleting them would remove the
		// only assertion that a public-capable calendar exists at all ([VS-9]).
		//
		// This is the slice's headline row. `/apps/calendar` is the ONLY calendar
		// surface the product still links to, so an anonymous visitor to a PUBLIC
		// campaign who follows any swept door must land on it rather than on
		// /login. The move is a group change (cg → pub) plus a guard swap
		// (RequireRole(RolePlayer) → RequireViewAccess()), and routes_snapshot.txt
		// records METHOD/PATH/file and NOTHING about middleware — so this table is
		// the oracle for it, and the wire snapshot is not.
		{"apps calendar Bench, public", http.MethodGet, "/campaigns/" + cid + "/apps/calendar", true, false},
		{"apps calendar Bench, private", http.MethodGet, "/campaigns/" + cid + "/apps/calendar", false, true},

		// Mutating / per-user route stays authenticated even on a public campaign.
		{"calendar v2 switch POST, public", http.MethodPost, "/campaigns/" + cid + "/calendar/v2/switch", true, true},
		{"calendar v2 switch POST, private", http.MethodPost, "/campaigns/" + cid + "/calendar/v2/switch", false, true},

		// [VS-4]'s scope, asserted as a NEGATIVE: no second route moved. Each of
		// these was named OUT by the block, and each must still bounce an
		// anonymous visitor to /login on a PUBLIC campaign. "Moving a second
		// route is a STOP-AND-FLAG" is only enforceable if something checks.
		// The three WRITE routes bounce to a bare /login, which is what
		// isLoginRedirect above recognises. The two authenticated GETs bounce to
		// /login?redirect=… instead, so they are asserted by
		// TestBenchRoute_NoSecondRouteMoved below rather than by widening this
		// predicate — a guard that accepted both forms here would also accept
		// them for every V2 row above, which is a weaker test than the one that
		// shipped.
		{"calendar prefs POST stays authenticated, public", http.MethodPost, "/campaigns/" + cid + "/calendar/prefs", true, true},
		{"calendar sidebar-pin POST stays authenticated, public", http.MethodPost, "/campaigns/" + cid + "/calendar/v2/sidebar-pin", true, true},
		{"world-state PUT stays authenticated, public", http.MethodPut, "/campaigns/" + cid + "/calendar/world-state", true, true},
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

// --- C-CALV4-V2SUNSET R2-4, [VS-5] items 1-3 and 7, [VS-17] SIGNED ----------
//
// THE WIRE SNAPSHOT CANNOT SEE THIS SLICE'S AUTHORISATION CHANGE. It records
// METHOD, PATH and defining file; `GET /campaigns/:id/apps/calendar` keeps all
// three and moves from the authenticated group to the public-capable one. So
// the oracle stays byte-identical at 727 lines while the gate changes, which is
// exactly the shape a reviewer skims past ([VS-3]'s RULING). These tests are
// the oracle instead.

// benchRouteLine is the registration [VS-4] SIGNED, asserted against the SOURCE
// rather than against behaviour.
//
// WHY SOURCE. Echo exposes Method, Path and handler name through e.Routes() and
// exposes NOTHING about a route's middleware chain, so "its chain is
// byte-identical to GET /calendar/v2's" — item 1 — is not answerable from the
// route table. The two halves of the pairing ARE answerable from the line that
// registers it: the group (`pub`, which carries OptionalAuth →
// AllowPublicCampaignAccess → RequireAddon) and the guard
// (`RequireViewAccess()`). A test that only checked behaviour would go green
// again if someone re-registered the route on `cg` with a hand-rolled public
// bypass, which is the failure this pins.
const benchRouteLine = `pub.GET("/apps/calendar", h.AppDashboard, campaigns.RequireViewAccess())`

func TestBenchRoute_IsRegisteredOnThePublicGroupWithViewAccess(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatalf("reading routes.go: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, benchRouteLine) {
		t.Errorf("[VS-4]: the Bench is no longer registered as\n\t%s\n"+
			"The group and the guard move TOGETHER: anonymous and non-member visitors are "+
			"RoleNone, and RequireRole(RolePlayer) refuses RoleNone (`0 < 1`), so `pub` with "+
			"RequireRole would 403 exactly the population this route move exists to serve — "+
			"the regression PR #478 already shipped and fixed once.", benchRouteLine)
	}
	// Item 2: the path is a LITERAL string. wire_contract_test.go skips and logs
	// a non-literal path, which would put an auth surface outside the oracle.
	if strings.Contains(body, `pub.GET(appsCalendarPath`) || strings.Contains(body, "pub.GET(fmt.Sprintf") {
		t.Error("[VS-5] item 2: the Bench's path stopped being a literal string — the wire " +
			"contract test skips non-literal paths, which would put an auth surface outside the oracle")
	}
	// The pairing this copies must still be there to have been copied FROM.
	if !strings.Contains(body, `pub.GET("/calendar/v2", h.ShowV2, campaigns.RequireViewAccess())`) {
		t.Error("[VS-4]: the shipped pairing the Bench's registration copies has moved — " +
			"re-read the block before trusting this test's twin")
	}
}

// TestBenchRoute_SnapshotIsUnmoved is [VS-5] item 3, asserted as a NEGATIVE.
// R2-4 removes and adds nothing, so the count is stated rather than derived: a
// diff in either direction is a STOP-AND-FLAG, INCLUDING a move toward the 722
// that C-CALV4-SHELL-REMOVAL will eventually reach.
func TestBenchRoute_SnapshotIsUnmoved(t *testing.T) {
	const wantLines = 727
	raw, err := os.ReadFile(filepath.Join("..", "..", "wire", "routes_snapshot.txt"))
	if err != nil {
		t.Fatalf("reading the wire snapshot: %v", err)
	}
	if got := strings.Count(string(raw), "\n"); got != wantLines {
		t.Errorf("routes_snapshot.txt is %d lines, want %d — R2-4 adds and removes NO route, "+
			"and the 727 → 722 diff belongs to C-CALV4-SHELL-REMOVAL", got, wantLines)
	}
	// The snapshot records the path WITHOUT the group prefix: "GET\t/apps/calendar".
	if !strings.Contains(string(raw), "GET\t/apps/calendar\t") {
		t.Error("the Bench's path left the snapshot — the route move must not touch METHOD or PATH")
	}
}

// addonOffAddonSvc reports the calendar addon DISABLED for every campaign.
type addonOffAddonSvc struct{ addons.AddonService }

func (addonOffAddonSvc) IsEnabledForCampaign(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}

// TestAnonymousAccess_AddonOffAnswersIdenticallyToTheV2Shell is [VS-17] SIGNED.
//
// §5.2 item 7 said only "the addon middleware answers, for every identity",
// which passes for a 404, a 403 AND a /login redirect — three different
// disclosures about whether a public campaign has the calendar addon enabled.
// The RULING is that the answer for /apps/calendar must be IDENTICAL to the
// answer for /calendar/v2 for the same identity, because both then live on
// `pub` behind the same RequireAddon; divergence means the gate is not actually
// shared and the move is not the copy of a shipped pairing [VS-4] signed it as.
//
// THE MEASURED ANSWER IS WRITTEN INTO THE TEST rather than asserted as
// `!= 200`. Measured on this branch, for an anonymous visitor to a PUBLIC
// campaign with the calendar addon OFF, on BOTH routes: **303 See Other to
// /campaigns/:id** — the campaign dashboard, not a 404, not a 403 and not
// /login. addons/middleware.go:38-44 is where that is decided: a full-page
// request redirects to the campaign, and only an HTMX request gets the 404.
// The status is stated in the slice's report because it IS a disclosure
// decision — a public campaign with the addon off is indistinguishable from one
// whose calendar you simply navigated away from, which is the more discreet of
// the three possible answers. Changing it is out of scope ([VS-17]).
func TestAnonymousAccess_AddonOffAnswersIdenticallyToTheV2Shell(t *testing.T) {
	const cid = "camp-1"
	const wantStatus = http.StatusSeeOther
	const wantLocation = "/campaigns/" + cid

	newRouter := func() *echo.Echo {
		e := echo.New()
		e.Use(emw.Recover())
		campaignSvc := guardCampaignSvc{public: true}
		authSvc := guardAuthSvc{}
		campaigns.RegisterRoutes(e, campaigns.NewHandler(campaignSvc), campaignSvc, authSvc)
		RegisterRoutes(e, NewHandler(guardCalSvc{}), campaignSvc, authSvc, addonOffAddonSvc{})
		return e
	}
	get := func(path string) (int, string) {
		e := newRouter()
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Header().Get("Location")
	}

	benchCode, benchLoc := get("/campaigns/" + cid + "/apps/calendar")
	shellCode, shellLoc := get("/campaigns/" + cid + "/calendar/v2")

	if benchCode != shellCode || benchLoc != shellLoc {
		t.Errorf("[VS-17] STOP-AND-FLAG: addon-off answers DIVERGE for an anonymous visitor — "+
			"/apps/calendar=%d%q vs /calendar/v2=%d%q. Both routes are on `pub` behind the same "+
			"RequireAddon, so a divergence is a bug in the move, not a product choice",
			benchCode, benchLoc, shellCode, shellLoc)
	}
	if benchCode != wantStatus || benchLoc != wantLocation {
		t.Errorf("[VS-17]: addon-off answer is %d %q, want the measured %d %q — the test asserts a "+
			"SPECIFIC status, never `!= 200`. If this changed, re-measure and re-state it in the "+
			"report; changing it to something 'better' is out of scope",
			benchCode, benchLoc, wantStatus, wantLocation)
	}
}

// TestBenchRoute_NoSecondRouteMoved is the other half of [VS-4]'s scope: ONE
// route changes group and guard, and "moving a second route is a STOP-AND-FLAG"
// is only enforceable if something checks.
//
// These two are authenticated GETs, so their bounce carries a ?redirect= the
// bare-/login predicate above deliberately does not accept. They get their own
// predicate here rather than a widened one there.
func TestBenchRoute_NoSecondRouteMoved(t *testing.T) {
	const cid = "camp-1"
	for _, path := range []string{
		"/campaigns/" + cid + "/schedule",
		"/campaigns/" + cid + "/calendar/v2/cal-1/settings/months",
	} {
		e := newGuardRouter(true) // PUBLIC campaign — the population the move serves
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		loc := rec.Header().Get("Location")
		if !strings.HasPrefix(loc, "/login") {
			t.Errorf("[VS-4] STOP-AND-FLAG: anonymous GET %s on a PUBLIC campaign was not bounced "+
				"to /login (code=%d location=%q) — a second route moved to the public group, and "+
				"[WG-7]'s scope is the whole scope", path, rec.Code, loc)
		}
	}
}
