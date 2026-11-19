package campaigns

// leave_route_test.go drives POST /campaigns/:id/leave through the real route
// table behind the real access gate and the app's CSRF check, so the role and
// token rules are tested where a browser meets them.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// sessionOnlyAuth answers the one auth call the route table's gate makes.
type sessionOnlyAuth struct {
	auth.AuthService
	session *auth.Session
}

func (a sessionOnlyAuth) ValidateSession(context.Context, string) (*auth.Session, error) {
	return a.session, nil
}

func leaveRouter(t *testing.T, role Role, member bool, userID string) (*echo.Echo, *leaveSpyRepo) {
	t.Helper()
	return leaveRouterFor(t, role, member, &auth.Session{UserID: userID})
}

func leaveRouterFor(t *testing.T, role Role, member bool, session *auth.Session) (*echo.Echo, *leaveSpyRepo) {
	t.Helper()
	base := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, uid string) (*CampaignMember, error) {
			if !member {
				return nil, apperror.NewNotFound("member not found")
			}
			return &CampaignMember{UserID: uid, Role: role, DisplayName: "Tam"}, nil
		},
	}
	repo := &leaveSpyRepo{mockCampaignRepo: base}
	svc := NewCampaignService(repo, &mockUserFinder{}, nil, nil, "http://localhost")
	e := echo.New()
	e.HTTPErrorHandler = func(c *echo.Context, err error) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.NoContent(ae.Code)
			return
		}
		var sc echo.HTTPStatusCoder
		if errors.As(err, &sc) {
			_ = c.NoContent(sc.StatusCode())
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	e.Use(middleware.CSRF())
	RegisterRoutes(e, NewHandler(svc), svc, sessionOnlyAuth{session: session})
	return e, repo
}

func postLeave(e *echo.Echo, withCSRF bool, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(""))
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "s"})
	if withCSRF {
		req.AddCookie(&http.Cookie{Name: "chronicle_csrf", Value: "tok"})
		req.Header.Set("X-CSRF-Token", "tok")
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestLeaveRoute(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		member   bool
		csrf     bool
		wantCode int
		wantGone bool
	}{
		{"player leaves", RolePlayer, true, true, http.StatusOK, true},
		{"scribe leaves", RoleScribe, true, true, http.StatusOK, true},
		{"no CSRF token is refused", RolePlayer, true, false, http.StatusForbidden, false},
		{"owner is refused", RoleOwner, true, true, http.StatusBadRequest, false},
		{"non-member is refused", RoleNone, false, true, http.StatusForbidden, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, repo := leaveRouter(t, tc.role, tc.member, "tam")
			rec := postLeave(e, tc.csrf, "/campaigns/camp-1/leave")
			if rec.Code != tc.wantCode {
				t.Fatalf("status %d, want %d (body %q)", rec.Code, tc.wantCode, rec.Body.String())
			}
			gone := len(repo.writes) == 1 && repo.writes[0] == "RemoveMember:tam"
			if gone != tc.wantGone {
				t.Errorf("membership ended = %v, want %v (writes %v)", gone, tc.wantGone, repo.writes)
			}
		})
	}
}

// The route takes no user id: a request can only ever end the signed-in
// person's own membership.
func TestLeaveRouteHasNoTargetParameter(t *testing.T) {
	e, repo := leaveRouter(t, RolePlayer, true, "tam")
	rec := postLeave(e, true, "/campaigns/camp-1/leave/other-user")
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d for a path that names another user, want 404 or 405", rec.Code)
	}
	if len(repo.writes) != 0 {
		t.Errorf("writes %v", repo.writes)
	}
}

// A guest's account ends when they leave their campaign, so leaving is the
// DM's call for them: the route refuses and nothing changes.
func TestLeaveRouteRefusesGuests(t *testing.T) {
	e, repo := leaveRouterFor(t, RolePlayer, true, &auth.Session{UserID: "guest", GuestCampaignID: "camp-1"})
	rec := postLeave(e, true, "/campaigns/camp-1/leave")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", rec.Code)
	}
	if len(repo.writes) != 0 {
		t.Errorf("writes %v", repo.writes)
	}
}
