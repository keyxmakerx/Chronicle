package campaigns

// middleware_anon_test.go — C-PERM-ANON-IDENTITY. Pins that AllowPublicCampaignAccess
// gives non-member viewers the public identity (RoleNone, below RolePlayer)
// rather than RolePlayer, so Player-only / Player-role-tag-granted content never
// leaks to the public. Real members keep their actual role.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// stubPublicSvc satisfies CampaignService for the middleware under test by
// embedding the interface (only GetByID/GetMember are exercised; any other call
// would nil-panic, which is the desired "not expected here" signal).
type stubPublicSvc struct {
	CampaignService
	campaign  *Campaign
	member    *CampaignMember
	memberErr error
}

func (s *stubPublicSvc) GetByID(context.Context, string) (*Campaign, error) {
	return s.campaign, nil
}

func (s *stubPublicSvc) GetMember(context.Context, string, string) (*CampaignMember, error) {
	if s.memberErr != nil {
		return nil, s.memberErr
	}
	return s.member, nil
}

// runPublicAccess invokes AllowPublicCampaignAccess with the given session
// (nil = anonymous) and returns the resolved CampaignContext plus the recorder.
func runPublicAccess(t *testing.T, svc CampaignService, session *auth.Session) (*CampaignContext, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("camp-1")
	if session != nil {
		auth.SetSession(c, session)
	}

	var got *CampaignContext
	h := AllowPublicCampaignAccess(svc)(func(c echo.Context) error {
		got = GetCampaignContext(c)
		return nil
	})
	if err := h(c); err != nil {
		// Surface the error to the recorder so redirect/forbidden cases assert on it.
		e.HTTPErrorHandler(err, c)
	}
	return got, rec
}

func TestAllowPublicAccess_AnonymousVisitorIsBelowPlayer(t *testing.T) {
	svc := &stubPublicSvc{
		campaign:  &Campaign{ID: "camp-1", IsPublic: true},
		memberErr: apperror.NewNotFound("not a member"),
	}
	cc, _ := runPublicAccess(t, svc, nil) // nil session = logged out
	if cc == nil {
		t.Fatal("expected a CampaignContext for an anonymous public visitor")
	}
	if cc.MemberRole != RoleNone {
		t.Errorf("anonymous MemberRole = %d, want RoleNone (%d) — must be below RolePlayer", cc.MemberRole, RoleNone)
	}
	if cc.MemberRole >= RolePlayer {
		t.Errorf("anonymous must be strictly below RolePlayer; got role %d", cc.MemberRole)
	}
	if !cc.IsAnonymous {
		t.Error("IsAnonymous must be true for a logged-out visitor")
	}
	if cc.IsMember {
		t.Error("IsMember must be false for an anonymous visitor")
	}
	if cc.VisibilityRole() != int(RoleNone) {
		t.Errorf("VisibilityRole() = %d, want 0 (anonymous sees only public content)", cc.VisibilityRole())
	}
}

func TestAllowPublicAccess_AuthenticatedNonMemberIsBelowPlayer(t *testing.T) {
	svc := &stubPublicSvc{
		campaign:  &Campaign{ID: "camp-1", IsPublic: true},
		memberErr: apperror.NewNotFound("not a member"),
	}
	cc, _ := runPublicAccess(t, svc, &auth.Session{UserID: "stranger-1"})
	if cc == nil {
		t.Fatal("expected a CampaignContext for an authenticated non-member")
	}
	if cc.MemberRole != RoleNone {
		t.Errorf("authenticated non-member MemberRole = %d, want RoleNone — Player means members only", cc.MemberRole)
	}
	if cc.IsAnonymous {
		t.Error("IsAnonymous must be false for an authenticated viewer")
	}
	if cc.IsMember {
		t.Error("IsMember must be false for a non-member")
	}
}

func TestAllowPublicAccess_RealMemberKeepsRole(t *testing.T) {
	svc := &stubPublicSvc{
		campaign: &Campaign{ID: "camp-1", IsPublic: true},
		member:   &CampaignMember{UserID: "u1", Role: RolePlayer},
	}
	cc, _ := runPublicAccess(t, svc, &auth.Session{UserID: "u1"})
	if cc == nil {
		t.Fatal("expected a CampaignContext for a member")
	}
	if cc.MemberRole != RolePlayer {
		t.Errorf("real Player MemberRole = %d, want RolePlayer (%d) — unchanged", cc.MemberRole, RolePlayer)
	}
	if !cc.IsMember {
		t.Error("IsMember must be true for a real member")
	}
	if cc.IsAnonymous {
		t.Error("IsAnonymous must be false for a member")
	}
}

func TestAllowPublicAccess_AnonymousOnPrivateCampaignRedirects(t *testing.T) {
	svc := &stubPublicSvc{
		campaign: &Campaign{ID: "camp-1", IsPublic: false},
	}
	_, rec := runPublicAccess(t, svc, nil)
	if rec.Code != http.StatusFound {
		t.Errorf("anonymous on a private campaign must redirect (302); got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("redirect Location = %q, want /login", loc)
	}
}
