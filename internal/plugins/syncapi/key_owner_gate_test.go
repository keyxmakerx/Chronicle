package syncapi

import (
	stderrors "errors"

	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestRequireKeyOwnerStillOwner_RESTPath pins the policy: a stored Bearer
// key stops working, with a clear structured error, once its creator is no
// longer an Owner of the key's campaign, so a demoted or removed creator
// can't keep granting Owner-level sync access.
func TestRequireKeyOwnerStillOwner_RESTPath(t *testing.T) {
	rawKey := "chron_ownergate0123456789012345678901234567890123456789012345678"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	cases := []struct {
		name      string
		getMember func(context.Context, string, string) (*campaigns.CampaignMember, error)
		wantOK    bool
	}{
		{
			name:      "creator still Owner — request proceeds",
			getMember: memberWithRole(campaigns.RoleOwner),
			wantOK:    true,
		},
		{
			name:      "creator demoted to Scribe — refused",
			getMember: memberWithRole(campaigns.RoleScribe),
			wantOK:    false,
		},
		{
			name:      "creator removed from campaign — refused",
			getMember: memberRemoved,
			wantOK:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockSyncAPIRepo{
				findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
					if prefix != rawKey[:keyPrefixLen] {
						return nil, apperror.NewNotFound("key not found")
					}
					return &APIKey{
						ID:         200,
						KeyHash:    string(hash),
						KeyPrefix:  prefix,
						CampaignID: "camp-1",
						UserID:     "creator-1",
						IsActive:   true,
					}, nil
				},
				isIPBlockedFn: func(_ context.Context, _ string) (bool, error) { return false, nil },
				logRequestFn:  func(_ context.Context, _ *APIRequestLog) error { return nil },
				logSecurityEventFn: func(_ context.Context, _ *SecurityEvent) error {
					return nil
				},
			}
			syncSvc := NewSyncAPIService(repo)
			campSvc := &fakeCampaignService{getMemberFn: tc.getMember}

			e := echo.New()
			e.HTTPErrorHandler = func(err error, c echo.Context) {
				var appErr *apperror.AppError
				if stderrors.As(err, &appErr) {
					_ = c.JSON(appErr.Code, map[string]string{"error": appErr.Type, "message": appErr.Message})
					return
				}
				_ = c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			}
			g := e.Group("/api/v1/campaigns/:id",
				RequireAPIKey(syncSvc),
				RequireKeyOwnerStillOwner(campSvc, syncSvc),
			)
			g.GET("", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1", nil)
			req.Header.Set("Authorization", "Bearer "+rawKey)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if tc.wantOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
				}
				return
			}
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body not JSON: %v", err)
			}
			if body["error"] != "key_owner_lost_access" {
				t.Errorf(`body["error"] = %q, want "key_owner_lost_access"`, body["error"])
			}
		})
	}
}

// TestRequireKeyOwnerStillOwner_SkipsSyntheticSessionKeys pins that a
// session-synthesised identity (ID == synthKeySessionID) is untouched by
// this middleware: session callers already track their LIVE membership role
// on every request via RequireAuthOrAPIKey, so there is nothing to enforce
// here, even when that live role has fallen below Owner.
func TestRequireKeyOwnerStillOwner_SkipsSyntheticSessionKeys(t *testing.T) {
	campSvc := &fakeCampaignService{getMemberFn: memberWithRole(campaigns.RolePlayer)}
	syncSvc := NewSyncAPIService(&mockSyncAPIRepo{})

	mw := RequireKeyOwnerStillOwner(campSvc, syncSvc)
	called := false
	next := func(echo.Context) error { called = true; return nil }

	c, _ := newRoleContext(&APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "user-1"})
	if err := mw(next)(c); err != nil {
		t.Fatalf("synthetic session key: want no error, got %v", err)
	}
	if !called {
		t.Error("synthetic session key: next handler was not reached")
	}
}

// TestAuthenticateKeyForWS_RefusesWhenCreatorLostAccess is the WebSocket
// twin of TestRequireKeyOwnerStillOwner_RESTPath: internal/websocket/auth.go
// calls AuthenticateKeyForWS directly, bypassing the REST middleware chain
// entirely, so the same owner-still-owner policy must be enforced here too.
func TestAuthenticateKeyForWS_RefusesWhenCreatorLostAccess(t *testing.T) {
	gate := newFakeAddonGate()
	gate.enabled["camp-ws"] = true

	cases := []struct {
		name   string
		member MembershipChecker
	}{
		{"creator demoted", &fakeCampaignService{getMemberFn: memberWithRole(campaigns.RoleScribe)}},
		{"creator removed", &fakeCampaignService{getMemberFn: memberRemoved}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, rawKey := wsGateService(t, gate, tc.member)

			_, _, _, _, err := svc.AuthenticateKeyForWS(context.Background(), rawKey)
			if err == nil {
				t.Fatal("AuthenticateKeyForWS accepted a key whose creator lost Owner access")
			}
			var appErr *apperror.AppError
			if !stderrors.As(err, &appErr) || appErr.Type != "key_owner_lost_access" {
				t.Errorf("error = %v, want an AppError of type key_owner_lost_access", err)
			}
		})
	}
}

// TestAuthenticateKeyForWS_RefusedWhenMemberCheckerUnwired pins that an
// unwired membership checker refuses rather than silently defaulting to
// Owner — the same fail-closed treatment as an unwired addon gate.
func TestAuthenticateKeyForWS_RefusedWhenMemberCheckerUnwired(t *testing.T) {
	gate := newFakeAddonGate()
	gate.enabled["camp-ws"] = true
	svc, rawKey := wsGateService(t, gate, nil)

	if _, _, _, _, err := svc.AuthenticateKeyForWS(context.Background(), rawKey); err == nil {
		t.Fatal("AuthenticateKeyForWS accepted a key with no membership checker wired")
	}
}
