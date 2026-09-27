// wizard_double_submit_test.go pins that the wizard's two form submits (the
// Import step's "Continue" and the Review step's "Create calendar") render
// with hx-disabled-elt + hx-indicator, so a second click during the
// in-flight request can't fire a second request — see calendar_wizard.templ.
package calendar

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// doHTMXGetRequest issues a GET with the HX-Request header set, the way the
// wizard's step-to-step navigation does.
func doHTMXGetRequest(e *echo.Echo, path, userID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("HX-Request", "true")
	if userID != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func newWizardTestEcho(t *testing.T) (*echo.Echo, string) {
	t.Helper()
	const campaignID = "camp-wizard-dblsubmit"
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
	return e, campaignID
}

func TestWizardImportBody_GuardsAgainstDoubleSubmit(t *testing.T) {
	e, campaignID := newWizardTestEcho(t)
	rec := doHTMXGetRequest(e, "/campaigns/"+campaignID+"/calendars/wizard/import", "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET wizard/import: got %d, body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`hx-disabled-elt="#calv5-import-submit"`,
		`hx-indicator="#calv5-import-spinner"`,
		`id="calv5-import-submit"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the Import step's form to contain %q, body:\n%s", want, body)
		}
	}
}

func TestWizardReviewBody_GuardsAgainstDoubleSubmit(t *testing.T) {
	e, campaignID := newWizardTestEcho(t)
	rec := doHTMXGetRequest(e, "/campaigns/"+campaignID+"/calendars/wizard/presets/blank", "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET wizard/presets/blank: got %d, body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`hx-disabled-elt="#calv5-review-submit"`,
		`hx-indicator="#calv5-review-spinner"`,
		`id="calv5-review-submit"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the Review step's form to contain %q, body:\n%s", want, body)
		}
	}
}
