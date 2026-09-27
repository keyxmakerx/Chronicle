// wizard_create_htmx_error_test.go: proves the wizard's Review->Create step
// surfaces a validation failure (specifically the current-date range check,
// validateImportCurrentDate) as an HTMX-swappable 200 response rather than a
// bare 4xx htmx would silently drop — see WizardCreate's own error-handling
// branch. Uses the REAL calendarService (real.CreateCalendarFromImport),
// not a fake, so this actually exercises the fix rather than a hand-rolled
// stand-in's behavior.
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// doHTMXFormRequest posts an application/x-www-form-urlencoded body with the
// HX-Request header set, the way the wizard's real <form hx-post> does.
func doHTMXFormRequest(e *echo.Echo, path, userID string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	if userID != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestWizardCreate_OutOfRangeDayIsAnHTMXSwappable200NotA4xx(t *testing.T) {
	const campaignID = "camp-wizard-range"
	e := echo.New()
	e.Use(emw.Recover())
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	svc := NewCalendarService(&fakeCalendarRepo{}, &fakeEventRepo{}, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	h := NewHandler(svc)
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	RegisterRoutes(e, h, guardCampaignSvc{roles: roles}, guardAuthSvc{}, guardAddonSvc{enabled: true})

	form := url.Values{}
	form.Set("source", "preset")
	form.Set("preset_name", "blank")
	form.Set("name", "My Calendar")
	form.Set("current_year", "1")
	form.Set("current_month", "1")
	form.Set("current_day", "999") // every shipped preset's first month has far fewer than 999 days

	rec := doHTMXFormRequest(e, "/campaigns/"+campaignID+"/calendars/wizard/create", "u-owner", form)

	if rec.Code != http.StatusOK {
		t.Fatalf("an out-of-range day must come back as an HTMX-swappable 200 (see WizardCreate's error branch), got %d: %s",
			rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "" {
		t.Errorf("must not redirect on a validation failure, got HX-Redirect=%q", rec.Header().Get("HX-Redirect"))
	}
	if !strings.Contains(rec.Body.String(), "current_day") {
		t.Errorf("expected the range-validation error inline in the re-rendered Review step, body:\n%s", rec.Body.String())
	}
}

// TestWizardCreate_ValidDateSucceedsAndRedirects is the control: a
// same-shaped request with an in-range day must actually create the
// calendar and redirect, so the test above is pinning a real rejection, not
// a handler that always fails.
func TestWizardCreate_ValidDateSucceedsAndRedirects(t *testing.T) {
	const campaignID = "camp-wizard-range-ok"
	e := echo.New()
	e.Use(emw.Recover())
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	var created *Calendar
	calRepo := &fakeCalendarRepo{
		createFn: func(_ context.Context, cal *Calendar) error {
			created = cal
			return nil
		},
	}
	svc := NewCalendarService(calRepo, &fakeEventRepo{}, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	h := NewHandler(svc)
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	RegisterRoutes(e, h, guardCampaignSvc{roles: roles}, guardAuthSvc{}, guardAddonSvc{enabled: true})

	form := url.Values{}
	form.Set("source", "preset")
	form.Set("preset_name", "blank")
	form.Set("name", "My Calendar")
	form.Set("current_year", "1")
	form.Set("current_month", "1")
	form.Set("current_day", "1")

	rec := doHTMXFormRequest(e, "/campaigns/"+campaignID+"/calendars/wizard/create", "u-owner", form)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("a valid submission must redirect (HX-Redirect + 204), got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") == "" {
		t.Error("expected an HX-Redirect header on success")
	}
	if created == nil {
		t.Fatal("expected the calendar to actually be created")
	}
}
