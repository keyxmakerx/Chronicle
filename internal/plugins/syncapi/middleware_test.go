package syncapi

import (
	stderrors "errors"

	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Narrow test doubles ---
//
// We embed the real service interfaces as fields so we only have to
// override the single method we care about per test; any other method
// the middleware happens to call will nil-deref instead of silently
// returning zero values.

type fakeAuthService struct {
	auth.AuthService
	validateSessionFn func(ctx context.Context, token string) (*auth.Session, error)
}

func (f *fakeAuthService) ValidateSession(ctx context.Context, token string) (*auth.Session, error) {
	if f.validateSessionFn == nil {
		return nil, stderrors.New("no session")
	}
	return f.validateSessionFn(ctx, token)
}

type fakeCampaignService struct {
	campaigns.CampaignService
	getMemberFn func(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error)
}

func (f *fakeCampaignService) GetMember(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	if f.getMemberFn == nil {
		return nil, stderrors.New("no membership")
	}
	return f.getMemberFn(ctx, campaignID, userID)
}

// --- Fixtures ---

// newMultiAuthFixture returns a fresh echo instance with the
// RequireAuthOrAPIKey middleware wired against the supplied doubles, plus a
// terminal "pong" handler that asserts an APIKey landed on the context and
// echoes its campaign/user/permissions for verification.
func newMultiAuthFixture(t *testing.T, authSvc auth.AuthService, campaignSvc campaigns.CampaignService, syncSvc SyncAPIService) *echo.Echo {
	t.Helper()
	e := echo.New()
	// Mirror the production error handler's shape for AppError: extract
	// Code and Message from *apperror.AppError, emit {error, message}
	// JSON. Anything else becomes 500 with a generic message — tests
	// that exercise AppError paths don't rely on the 500 branch.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		type errBody struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		var appErr *apperror.AppError
		if stderrors.As(err, &appErr) {
			_ = c.JSON(appErr.Code, errBody{
				Error:   appErr.Type,
				Message: appErr.Message,
			})
			return
		}
		msg := "internal error"
		if err != nil {
			msg = err.Error()
		}
		_ = c.JSON(http.StatusInternalServerError, errBody{
			Error:   "internal_error",
			Message: msg,
		})
	}

	cg := e.Group("/api/v1/campaigns/:id",
		RequireAuthOrAPIKey(authSvc, campaignSvc, syncSvc),
	)
	cg.POST("/entities", func(c echo.Context) error {
		key := GetAPIKey(c)
		if key == nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"error": "no key in context"})
		}
		return c.JSON(http.StatusOK, map[string]any{
			"key_id":       key.ID,
			"user_id":      key.UserID,
			"campaign_id":  key.CampaignID,
			"permissions":  key.Permissions,
			"is_synthetic": key.ID == synthKeySessionID,
		})
	})
	return e
}

// --- Tests ---

// TestRequireAuthOrAPIKey_SessionCookie drives the same POST target that an
// in-app browser widget would hit. A valid Chronicle session cookie should
// authenticate the request without any Authorization header — the bug
// originally reported was that this path returned 401. The middleware must
// synthesise an APIKey from the session + campaign role so downstream
// RequirePermission / RequireCampaignMatch logic continues to work.
func TestRequireAuthOrAPIKey_SessionCookie(t *testing.T) {
	authSvc := &fakeAuthService{
		validateSessionFn: func(_ context.Context, token string) (*auth.Session, error) {
			if token != "valid-session-token" {
				return nil, stderrors.New("invalid session")
			}
			return &auth.Session{UserID: "user-abc"}, nil
		},
	}
	campSvc := &fakeCampaignService{
		getMemberFn: func(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
			if campaignID != "camp-1" || userID != "user-abc" {
				return nil, stderrors.New("not a member")
			}
			return &campaigns.CampaignMember{
				CampaignID: campaignID,
				UserID:     userID,
				Role:       campaigns.RoleScribe,
			}, nil
		},
	}
	e := newMultiAuthFixture(t, authSvc, campSvc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/entities", nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "valid-session-token"})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("session-cookie POST: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

// TestRequireAuthOrAPIKey_APIKeyBearer confirms that the traditional
// Bearer-token path still works unchanged when no session cookie is
// present. External clients (Foundry VTT, curl) depend on this path.
func TestRequireAuthOrAPIKey_APIKeyBearer(t *testing.T) {
	rawKey := "chron_apikey12345678901234567890123456789012345678901234567890ab"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	repo := &mockSyncAPIRepo{
		findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
			if prefix != rawKey[:keyPrefixLen] {
				return nil, stderrors.New("wrong prefix")
			}
			return &APIKey{
				ID:         99,
				KeyHash:    string(hash),
				KeyPrefix:  prefix,
				CampaignID: "camp-1",
				UserID:     "user-bearer",
				IsActive:   true,
			}, nil
		},
		isIPBlockedFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
		// LogRequest is fired in a goroutine; no-op to avoid nil deref in tests.
		logRequestFn: func(_ context.Context, _ *APIRequestLog) error { return nil },
	}
	syncSvc := NewSyncAPIService(repo)
	// authSvc is present but empty — should never be consulted when there's
	// no cookie on the request.
	authSvc := &fakeAuthService{}
	campSvc := &fakeCampaignService{}
	e := newMultiAuthFixture(t, authSvc, campSvc, syncSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/entities", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("bearer POST: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

// TestRequireAuthOrAPIKey_NoAuth guards the failure path: no cookie AND no
// Authorization header means 401, and the response body carries a
// human-readable `message` field the widget can surface to the user. The
// Coordinator explicitly asked that widgets never have to render a bare
// status code; the fallback for 401 says "reload and sign in again".
func TestRequireAuthOrAPIKey_NoAuth(t *testing.T) {
	// Bearer path needs a sync service to reach its 401 branch.
	repo := &mockSyncAPIRepo{
		isIPBlockedFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
	}
	syncSvc := NewSyncAPIService(repo)
	authSvc := &fakeAuthService{}
	campSvc := &fakeCampaignService{}
	e := newMultiAuthFixture(t, authSvc, campSvc, syncSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/entities", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-auth POST: status = %d, want 401; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("no-auth POST: empty body; expected a JSON envelope with message field")
	}
	// The fixture's error handler mirrors err.Error() into Message; real
	// production envelope adds a user-friendly default via app.go's handler.
	// Either way the test validates the body is non-empty JSON — the
	// widget-side assertion (Bug D) is covered by the central handler, not
	// by this middleware-scoped test.
	if !containsIgnoreCase(body, "api key") && !containsIgnoreCase(body, "unauth") {
		t.Errorf("no-auth POST: body %q does not reference auth failure", body)
	}
}

// TestRequireAuthOrAPIKey_SessionPrecedence confirms session-cookie auth
// wins when BOTH a cookie and a Bearer header are present. This is the
// "widget on a machine that also has a saved API key" case. The widget
// session identity (real user) should carry rather than the API key
// identity, so downstream audit logs record the human who took the
// action rather than an ambient key.
func TestRequireAuthOrAPIKey_SessionPrecedence(t *testing.T) {
	authSvc := &fakeAuthService{
		validateSessionFn: func(_ context.Context, _ string) (*auth.Session, error) {
			return &auth.Session{UserID: "session-user"}, nil
		},
	}
	campSvc := &fakeCampaignService{
		getMemberFn: func(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
			return &campaigns.CampaignMember{
				CampaignID: campaignID,
				UserID:     userID,
				Role:       campaigns.RoleOwner,
			}, nil
		},
	}
	// Bearer path present but must not be reached — nil sync service would
	// nil-deref if RequireAPIKey ran.
	e := newMultiAuthFixture(t, authSvc, campSvc, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns/camp-1/entities", nil)
	req.AddCookie(&http.Cookie{Name: "chronicle_session", Value: "any-valid-token"})
	req.Header.Set("Authorization", "Bearer chron_someotherkey00000000000000000000000000000000000000000000ab")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("precedence POST: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !containsIgnoreCase(rec.Body.String(), "session-user") {
		t.Errorf("precedence POST: response did not identify session user; body = %s", rec.Body.String())
	}
}

// TestPermissionsForCampaignRole locks in the mapping from campaign role
// to API-key permissions used by RequireAuthOrAPIKey. The default shape
// has Owner > Scribe > Player with progressively fewer permissions; a
// future change that silently widens Player or narrows Owner should
// trip this test.
func TestPermissionsForCampaignRole(t *testing.T) {
	cases := []struct {
		role campaigns.Role
		want []APIKeyPermission
	}{
		{campaigns.RoleOwner, []APIKeyPermission{PermRead, PermWrite, PermSync}},
		{campaigns.RoleScribe, []APIKeyPermission{PermRead, PermWrite}},
		{campaigns.RolePlayer, []APIKeyPermission{PermRead}},
		{campaigns.RoleNone, nil},
	}
	for _, tc := range cases {
		got := permissionsForCampaignRole(tc.role)
		if !permsEqual(got, tc.want) {
			t.Errorf("role %s: got %v, want %v", tc.role, got, tc.want)
		}
	}
}

// --- Helpers ---

func containsIgnoreCase(s, sub string) bool {
	// Tiny local helper to keep the test file self-contained.
	sl, subl := toLower(s), toLower(sub)
	if len(subl) == 0 {
		return true
	}
	for i := 0; i+len(subl) <= len(sl); i++ {
		if sl[i:i+len(subl)] == subl {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func permsEqual(a, b []APIKeyPermission) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
