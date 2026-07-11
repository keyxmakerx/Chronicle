package campaigns

// public_view_access_test.go — C-PUBLIC-VIEW-FIX.
//
// WHY THIS FILE EXISTS: the public-view 403 regression shipped because the only
// test (middleware_anon_test.go) pinned AllowPublicCampaignAccess *in isolation*
// — it asserted the resolved CampaignContext but never ran the role GATE that
// every pub route applies AFTER it. The bug lived entirely in the COMPOSITION:
// AllowPublicCampaignAccess resolves anon -> RoleNone (correct, #478), then the
// route's RequireRole(RolePlayer) rejects RoleNone -> 403. These tests drive the
// composed chain through a real Echo router so that class of gap can't recur.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// serveViewChain builds the exact middleware composition every public-capable
// route uses — OptionalAuth (substituted by a faithful session-setter, since the
// auth plumbing is orthogonal to the gate under test) -> AllowPublicCampaignAccess
// -> the role gate -> handler — registers it on a real Echo router, and serves one
// GET /campaigns/camp-1. Returns the recorder and the CampaignContext that reached
// the handler (nil if the chain short-circuited before it).
func serveViewChain(svc CampaignService, session *auth.Session, gate echo.MiddlewareFunc) (*httptest.ResponseRecorder, *CampaignContext) {
	e := echo.New()
	// Mirror production: map AppError.Code -> HTTP status. Redirects are written
	// by the middleware directly (response already committed), so skip those.
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

	setSession := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if session != nil {
				auth.SetSession(c, session)
			}
			return next(c)
		}
	}

	var reached *CampaignContext
	pub := e.Group("/campaigns/:id", setSession, AllowPublicCampaignAccess(svc))
	pub.GET("", func(c echo.Context) error {
		reached = GetCampaignContext(c)
		return c.NoContent(http.StatusOK)
	}, gate)

	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec, reached
}

func publicCampaign() *Campaign  { return &Campaign{ID: "camp-1", IsPublic: true} }
func privateCampaign() *Campaign { return &Campaign{ID: "camp-1", IsPublic: false} }

// TestPublicViewChain_AccessMatrix drives the FULL composed chain (with the
// production gate, RequireViewAccess) across the viewer × visibility matrix.
func TestPublicViewChain_AccessMatrix(t *testing.T) {
	notMember := apperror.NewNotFound("not a member")

	cases := []struct {
		name        string
		svc         *stubPublicSvc
		session     *auth.Session
		wantStatus  int
		wantReached bool // did the request reach the handler past the gate?
		wantRole    Role // role the handler saw (only checked when reached)
	}{
		{
			name:        "anonymous → public campaign → 200 (the regression: was 403)",
			svc:         &stubPublicSvc{campaign: publicCampaign(), memberErr: notMember},
			session:     nil,
			wantStatus:  http.StatusOK,
			wantReached: true,
			wantRole:    RoleNone,
		},
		{
			name:        "anonymous → private campaign → 302 /login",
			svc:         &stubPublicSvc{campaign: privateCampaign()},
			session:     nil,
			wantStatus:  http.StatusFound,
			wantReached: false,
		},
		{
			name:        "authenticated non-member → public campaign → 200",
			svc:         &stubPublicSvc{campaign: publicCampaign(), memberErr: notMember},
			session:     &auth.Session{UserID: "stranger"},
			wantStatus:  http.StatusOK,
			wantReached: true,
			wantRole:    RoleNone,
		},
		{
			name:        "authenticated non-member → private campaign → 403",
			svc:         &stubPublicSvc{campaign: privateCampaign(), memberErr: notMember},
			session:     &auth.Session{UserID: "stranger"},
			wantStatus:  http.StatusForbidden,
			wantReached: false,
		},
		{
			name:        "real Player member → public campaign → 200 (keeps role)",
			svc:         &stubPublicSvc{campaign: publicCampaign(), member: &CampaignMember{UserID: "u1", Role: RolePlayer}},
			session:     &auth.Session{UserID: "u1"},
			wantStatus:  http.StatusOK,
			wantReached: true,
			wantRole:    RolePlayer,
		},
		{
			name:        "real Player member → private campaign → 200 (membership, not IsPublic, admits)",
			svc:         &stubPublicSvc{campaign: privateCampaign(), member: &CampaignMember{UserID: "u1", Role: RolePlayer}},
			session:     &auth.Session{UserID: "u1"},
			wantStatus:  http.StatusOK,
			wantReached: true,
			wantRole:    RolePlayer,
		},
		{
			name:        "site admin non-member → private campaign → 200 (admin bypass)",
			svc:         &stubPublicSvc{campaign: privateCampaign(), memberErr: notMember},
			session:     &auth.Session{UserID: "admin-1", IsAdmin: true},
			wantStatus:  http.StatusOK,
			wantReached: true,
			wantRole:    RoleNone, // admin views without a membership row → RoleNone, admitted via IsSiteAdmin
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, reached := serveViewChain(tc.svc, tc.session, RequireViewAccess())
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusFound {
				if loc := rec.Header().Get("Location"); loc != "/login" {
					t.Errorf("redirect Location = %q, want /login", loc)
				}
			}
			if (reached != nil) != tc.wantReached {
				t.Fatalf("handler reached = %v, want %v", reached != nil, tc.wantReached)
			}
			if tc.wantReached && reached.MemberRole != tc.wantRole {
				t.Errorf("handler saw MemberRole = %d, want %d", reached.MemberRole, tc.wantRole)
			}
		})
	}
}

// TestPublicViewChain_478InvariantPreserved pins that the fix did NOT restore
// anon -> RolePlayer: public visitors still reach the handler with RoleNone
// (VisibilityRole 0), so the visibility filter downstream still strips
// Player-only content. Restoring RolePlayer would reopen the #478 leak.
func TestPublicViewChain_478InvariantPreserved(t *testing.T) {
	notMember := apperror.NewNotFound("not a member")
	for _, tc := range []struct {
		name    string
		session *auth.Session
	}{
		{"anonymous", nil},
		{"authenticated non-member", &auth.Session{UserID: "stranger"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubPublicSvc{campaign: publicCampaign(), memberErr: notMember}
			_, reached := serveViewChain(svc, tc.session, RequireViewAccess())
			if reached == nil {
				t.Fatal("public visitor must reach the handler (200), not be gated")
			}
			if reached.MemberRole != RoleNone {
				t.Errorf("MemberRole = %d, want RoleNone — the fix must NOT elevate public visitors to Player", reached.MemberRole)
			}
			if reached.VisibilityRole() != int(RoleNone) {
				t.Errorf("VisibilityRole() = %d, want 0 — public visitors must see only public content", reached.VisibilityRole())
			}
		})
	}
}

// TestPublicViewChain_RegressionContrast is the pin for the exact gap: with the
// SAME composed chain, the OLD gate (RequireRole(RolePlayer)) 403s an anonymous
// visitor on a PUBLIC campaign — the production bug — while RequireViewAccess
// admits them. Isolated middleware tests could never surface this because the
// gate only misbehaves in composition with AllowPublicCampaignAccess's RoleNone.
func TestPublicViewChain_RegressionContrast(t *testing.T) {
	svc := &stubPublicSvc{campaign: publicCampaign(), memberErr: apperror.NewNotFound("not a member")}

	recOld, reachedOld := serveViewChain(svc, nil, RequireRole(RolePlayer))
	if recOld.Code != http.StatusForbidden {
		t.Errorf("regression baseline: RequireRole(RolePlayer) should 403 an anon public visitor, got %d", recOld.Code)
	}
	if reachedOld != nil {
		t.Error("regression baseline: the old gate must NOT reach the handler for anon")
	}

	recNew, reachedNew := serveViewChain(svc, nil, RequireViewAccess())
	if recNew.Code != http.StatusOK {
		t.Errorf("fix: RequireViewAccess must admit an anon public visitor (200), got %d", recNew.Code)
	}
	if reachedNew == nil {
		t.Error("fix: RequireViewAccess must reach the handler for an anon public visitor")
	}
}
