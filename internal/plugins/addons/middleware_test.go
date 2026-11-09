package addons

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// stubAddonService reports the addon as disabled for every campaign, so
// RequireAddon always exercises its refusal branches.
type stubAddonService struct {
	AddonService
}

func (stubAddonService) IsEnabledForCampaign(ctx context.Context, campaignID, addonSlug string) (bool, error) {
	return false, nil
}

// TestRequireAddon_DisabledRefusal pins the three shapes RequireAddon must
// answer a disabled addon with: an HTMX 404, a not-found AppError for
// fetch()/JSON callers (which the app's top-level error handler renders as
// JSON — see internal/app's isAPIRequest/errorHandler), and — only once
// neither applies — a redirect to the campaign dashboard. Before this fix,
// any non-HTMX request, including a plain fetch() call from a widget like
// entity_notes.js, fell straight to c.Redirect(), bypassing the error
// handler entirely, so a JSON caller got an HTML redirect body it couldn't
// parse.
func TestRequireAddon_DisabledRefusal(t *testing.T) {
	cases := []struct {
		name         string
		headers      map[string]string
		wantAppError bool
		wantRedirect bool
	}{
		{
			name:         "htmx request gets a not-found AppError",
			headers:      map[string]string{"HX-Request": "true"},
			wantAppError: true,
		},
		{
			name:         "fetch caller with Accept: application/json gets a not-found AppError",
			headers:      map[string]string{"Accept": "application/json"},
			wantAppError: true,
		},
		{
			name:         "fetch caller with JSON content type gets a not-found AppError",
			headers:      map[string]string{"Content-Type": "application/json"},
			wantAppError: true,
		},
		{
			name:         "plain browser navigation gets redirected",
			headers:      map[string]string{},
			wantRedirect: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/notes", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}})

			mw := RequireAddon(stubAddonService{}, "player-notes")
			handler := mw(func(c *echo.Context) error {
				t.Fatal("next() must not be called when the addon is disabled")
				return nil
			})

			err := handler(c)

			if tc.wantAppError {
				var appErr *apperror.AppError
				if !errors.As(err, &appErr) {
					t.Fatalf("err = %v, want an *apperror.AppError", err)
				}
				if appErr.Code != http.StatusNotFound {
					t.Errorf("appErr.Code = %d, want %d", appErr.Code, http.StatusNotFound)
				}
				return
			}

			// Redirect branch: RequireAddon writes the redirect itself instead
			// of returning an error.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.Code != http.StatusSeeOther {
				t.Errorf("status = %d, want %d (redirect)", rec.Code, http.StatusSeeOther)
			}
			if loc := rec.Header().Get("Location"); loc != "/campaigns/c1" {
				t.Errorf("Location = %q, want /campaigns/c1", loc)
			}
		})
	}
}
