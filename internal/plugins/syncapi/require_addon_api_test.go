package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

type stubAddonChecker struct {
	enabled bool
	err     error
}

func (s stubAddonChecker) IsEnabledForCampaign(context.Context, string, string) (bool, error) {
	return s.enabled, s.err
}

// TestRequireAddonAPI pins the refusal shapes: a switched-off add-on is a 403
// "addon_disabled" (never 404, which API clients read as "route missing"), and
// an unverifiable state still fails closed with 503.
func TestRequireAddonAPI(t *testing.T) {
	tests := []struct {
		name     string
		checker  stubAddonChecker
		wantCode int
		wantType string
		wantNext bool
	}{
		{"enabled passes through", stubAddonChecker{enabled: true}, 0, "", true},
		{"disabled is 403 addon_disabled", stubAddonChecker{enabled: false}, http.StatusForbidden, "addon_disabled", false},
		{"check failure fails closed", stubAddonChecker{err: errors.New("db down")}, http.StatusServiceUnavailable, "service_unavailable", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			c.SetParamNames("id")
			c.SetParamValues("camp-1")

			calledNext := false
			h := RequireAddonAPI(tc.checker, "calendar")(func(echo.Context) error {
				calledNext = true
				return nil
			})
			err := h(c)

			if calledNext != tc.wantNext {
				t.Fatalf("next called = %v, want %v", calledNext, tc.wantNext)
			}
			if tc.wantNext {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var appErr *apperror.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("error = %v, want *AppError", err)
			}
			if appErr.Code != tc.wantCode || appErr.Type != tc.wantType {
				t.Errorf("got %d/%q, want %d/%q", appErr.Code, appErr.Type, tc.wantCode, tc.wantType)
			}
			if tc.wantType == "addon_disabled" && !strings.Contains(appErr.Message, "calendar") {
				t.Errorf("message %q does not name the add-on", appErr.Message)
			}
		})
	}
}
