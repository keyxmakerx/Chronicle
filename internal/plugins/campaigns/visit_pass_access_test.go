package campaigns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// stubVisitSvc is a campaign service that also answers for look-around
// passes, as the real one does once passes are wired.
type stubVisitSvc struct {
	stubPublicSvc
	until time.Time
	live  bool
	found bool
}

func (s *stubVisitSvc) VisitPass(context.Context, string, string) (time.Time, bool, bool, error) {
	return s.until, s.live, s.found, nil
}

// A live pass opens a private campaign's view pages as a public visitor
// would see them; an ended one says so; and no pass ever gets through the
// write gate, which needs a membership.
func TestVisitPassAccess(t *testing.T) {
	notMember := apperror.NewNotFound("not a member")
	soon := time.Now().Add(48 * time.Hour)
	visitor := &auth.Session{UserID: "mira"}

	cases := []struct {
		name       string
		svc        *stubVisitSvc
		wantStatus int
		wantUntil  bool
	}{
		{"live pass, private campaign", &stubVisitSvc{stubPublicSvc: stubPublicSvc{campaign: privateCampaign(), memberErr: notMember}, until: soon, live: true, found: true}, http.StatusOK, true},
		{"ended pass, private campaign", &stubVisitSvc{stubPublicSvc: stubPublicSvc{campaign: privateCampaign(), memberErr: notMember}, until: time.Now().Add(-time.Hour), found: true}, http.StatusForbidden, false},
		{"no pass, private campaign", &stubVisitSvc{stubPublicSvc: stubPublicSvc{campaign: privateCampaign(), memberErr: notMember}}, http.StatusForbidden, false},
		{"ended pass, public campaign", &stubVisitSvc{stubPublicSvc: stubPublicSvc{campaign: publicCampaign(), memberErr: notMember}, until: time.Now().Add(-time.Hour), found: true}, http.StatusOK, false},
		{"member with an old pass keeps their role", &stubVisitSvc{stubPublicSvc: stubPublicSvc{campaign: privateCampaign(), member: &CampaignMember{UserID: "mira", Role: RolePlayer}}, until: soon, live: true, found: true}, http.StatusOK, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, cc := serveViewChain(tc.svc, visitor, RequireViewAccess())
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if cc == nil {
				return
			}
			if got := cc.VisitUntil != nil; got != tc.wantUntil {
				t.Fatalf("VisitUntil set = %v, want %v", got, tc.wantUntil)
			}
			if tc.wantUntil && (cc.MemberRole != RoleNone || cc.IsMember) {
				t.Fatalf("a visitor got role %v, member %v", cc.MemberRole, cc.IsMember)
			}
		})
	}

	// The write gate never consults passes: a visitor is a non-member there.
	live := &stubVisitSvc{stubPublicSvc: stubPublicSvc{campaign: privateCampaign(), memberErr: notMember}, until: soon, live: true, found: true}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if code := serveWriteGate(live, visitor, method); code != http.StatusForbidden {
			t.Fatalf("%s through RequireCampaignAccess = %d, want 403", method, code)
		}
	}
}

// serveWriteGate sends one request through RequireCampaignAccess.
func serveWriteGate(svc CampaignService, session *auth.Session, method string) int {
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	setSession := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			auth.SetSession(c, session)
			return next(c)
		}
	}
	ok := func(c *echo.Context) error { return c.NoContent(http.StatusOK) }
	g := e.Group("/campaigns/:id", setSession, RequireCampaignAccess(svc))
	g.GET("", ok)
	g.POST("", ok)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(method, "/campaigns/camp-1", nil))
	return rec.Code
}
