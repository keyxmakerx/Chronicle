package syncapi

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestRequireKeyOwnerStillOwner_TrashedCampaign pins that a key of a campaign
// in the site Trash gets the plain 404 a deleted campaign gives, not the
// "creator lost access" 403 (untrue, and it raises a security signal), while a
// lookup that fails for any other reason is not read as not-found.
func TestRequireKeyOwnerStillOwner_TrashedCampaign(t *testing.T) {
	rawKey := "chron_trashgate012345678901234567890123456789012345678901234567"
	hash, err := bcrypt.GenerateFromPassword([]byte(rawKey), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	cases := []struct {
		name     string
		getByID  func(context.Context, string) (*campaigns.Campaign, error)
		wantCode int
	}{
		{"trashed campaign is not found", func(context.Context, string) (*campaigns.Campaign, error) {
			return nil, apperror.NewNotFound("campaign not found")
		}, http.StatusNotFound},
		{"a lookup failure is not read as not-found", func(context.Context, string) (*campaigns.Campaign, error) {
			return nil, stderrors.New("db down")
		}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockSyncAPIRepo{
				findKeyByPrefixFn: func(_ context.Context, prefix string) (*APIKey, error) {
					return &APIKey{ID: 201, KeyHash: string(hash), KeyPrefix: prefix, CampaignID: "camp-1", UserID: "creator-1", IsActive: true}, nil
				},
				isIPBlockedFn:      func(_ context.Context, _ string) (bool, error) { return false, nil },
				logRequestFn:       func(_ context.Context, _ *APIRequestLog) error { return nil },
				logSecurityEventFn: func(_ context.Context, _ *SecurityEvent) error { return nil },
			}
			syncSvc := NewSyncAPIService(repo)
			campSvc := &fakeCampaignService{getByIDFn: tc.getByID, getMemberFn: memberWithRole(campaigns.RoleOwner)}

			e := echo.New()
			e.HTTPErrorHandler = func(err error, c echo.Context) {
				var appErr *apperror.AppError
				if stderrors.As(err, &appErr) {
					_ = c.JSON(appErr.Code, map[string]string{"error": appErr.Type})
					return
				}
				_ = c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			}
			g := e.Group("/api/v1/campaigns/:id", RequireAPIKey(syncSvc), RequireKeyOwnerStillOwner(campSvc, syncSvc))
			g.GET("", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1", nil)
			req.Header.Set("Authorization", "Bearer "+rawKey)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.wantCode, rec.Body.String())
			}
		})
	}
}
