// wizard_realworld_test.go covers the Real-world calendar wizard step end
// to end: the Review step's gate/content, and WizardCreate's source=
// "reallife" branch — both the wall-clock-computed "today" path and the
// manual-date path, plus the server-side zone validation neither the
// browser's own <select> options nor a well-behaved client would ever
// actually violate. Uses the REAL calendarService (see
// wizard_create_htmx_error_test.go's own doc comment for why: this exercises
// the real CreateCalendarFromImport validation, not a stand-in for it).
package calendar

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newRealWorldTestRouter builds a router over the REAL calendarService, so
// CreateCalendarFromImport's own validation (time.LoadLocation,
// validateImportCurrentDate) actually runs.
func newRealWorldTestRouter(calRepo *fakeCalendarRepo) (*echo.Echo, *Handler) {
	if calRepo == nil {
		calRepo = &fakeCalendarRepo{}
	}
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
	svc := NewCalendarService(calRepo, &fakeEventRepo{}, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	h := NewHandler(svc)
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner, "u-player": campaigns.RolePlayer}
	RegisterRoutes(e, h, guardCampaignSvc{roles: roles}, guardAuthSvc{}, guardAddonSvc{enabled: true})
	return e, h
}

func TestWizardRealWorldReview_GateAndContent(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)
	const path = "/campaigns/camp-real/calendars/wizard/reallife"

	if rec := doRequest(e, http.MethodGet, path, ""); !isLoginRedirect(rec) {
		t.Errorf("expected a login redirect for an unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(e, http.MethodGet, path, "u-player"); rec.Code != http.StatusForbidden {
		t.Errorf("a Player must be forbidden from the wizard, got %d: %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(e, http.MethodGet, path, "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("Owner must be able to open the real-world review step, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Real world", "Time zone", "Today follows the real date", "January", "December"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the real-world review to mention %q, body:\n%s", want, body)
		}
	}
	if !strings.Contains(body, `name="real_time_zone"`) {
		t.Errorf("expected a real_time_zone select, body:\n%s", body)
	}
	if !strings.Contains(body, `name="tracks_real_time"`) {
		t.Errorf("expected a tracks_real_time field, body:\n%s", body)
	}
	if !strings.Contains(body, `value="UTC"`) {
		t.Errorf("expected UTC among the offered zones, body:\n%s", body)
	}
}

// TestWizardRealWorldReview_SuggestsOwnerAccountZone: when
// SetTimezoneLookup is wired and the owner has a stored timezone, the
// review step's starting default is that zone (the browser's own Intl
// detection, client-side, still takes priority when it resolves — this only
// covers the server-rendered fallback).
func TestWizardRealWorldReview_SuggestsOwnerAccountZone(t *testing.T) {
	e, h := newRealWorldTestRouter(nil)
	zone := "Europe/London"
	h.SetTimezoneLookup(fakeTimezoneLookup{zone: &zone})

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-real/calendars/wizard/reallife", "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Europe/London") {
		t.Errorf("expected the owner's stored zone to seed the default, body:\n%s", rec.Body.String())
	}
}

// TestWizardRealWorldReview_NoTimezoneLookupWiredFallsBackSafely: a Handler
// that never had SetTimezoneLookup called (production shape before this
// PR, and every other test's Handler) must still render a working default
// rather than panicking on a nil interface.
func TestWizardRealWorldReview_NoTimezoneLookupWiredFallsBackSafely(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)
	rec := doRequest(e, http.MethodGet, "/campaigns/camp-real/calendars/wizard/reallife", "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
}

type fakeTimezoneLookup struct {
	zone *string
	err  error
}

func (f fakeTimezoneLookup) GetUser(_ context.Context, _ string) (*auth.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &auth.User{Timezone: f.zone}, nil
}

// TestWizardCreate_RealWorld_TracksRealTimeComputesTodayServerSide: with
// "today follows the real date" on, the browser sends no current_year/
// month/day at all (they're hidden client-side) — WizardCreate must still
// create a calendar whose current date is genuinely today in the chosen
// zone, computed server-side, never left at some invented default.
func TestWizardCreate_RealWorld_TracksRealTimeComputesTodayServerSide(t *testing.T) {
	var created *Calendar
	var appliedIR *ImportResult
	calRepo := &fakeCalendarRepo{
		createFn: func(_ context.Context, cal *Calendar) error {
			created = cal
			return nil
		},
		applyImportFn: func(_ context.Context, cal *Calendar, ir *ImportResult) error {
			appliedIR = ir
			return nil
		},
	}
	e, _ := newRealWorldTestRouter(calRepo)

	form := url.Values{}
	form.Set("source", "reallife")
	form.Set("real_time_zone", "UTC")
	form.Set("tracks_real_time", "true")
	// Deliberately no current_year/month/day — the switch is on, so the
	// browser's own review step never sends them (they're x-show hidden).

	rec := doHTMXFormRequest(e, "/campaigns/camp-real/calendars/wizard/create", "u-owner", form)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("expected a redirect on success, got %d (HX-Redirect=%q): %s",
			rec.Code, rec.Header().Get("HX-Redirect"), rec.Body.String())
	}
	if created == nil {
		t.Fatal("expected the calendar to be created")
	}
	if created.Mode != ModeRealLife {
		t.Errorf("Mode = %q, want %q", created.Mode, ModeRealLife)
	}
	if !created.TracksRealTime {
		t.Error("expected TracksRealTime to be true")
	}
	if created.RealTimeZone == nil || *created.RealTimeZone != "UTC" {
		t.Errorf("RealTimeZone = %v, want \"UTC\"", created.RealTimeZone)
	}
	wantY, wantM, wantD := time.Now().UTC().Date()
	if created.CurrentYear != wantY || created.CurrentMonth != int(wantM) || created.CurrentDay != wantD {
		t.Errorf("current date = %d-%d-%d, want today in UTC (%d-%d-%d)",
			created.CurrentYear, created.CurrentMonth, created.CurrentDay, wantY, int(wantM), wantD)
	}
	if appliedIR == nil || len(appliedIR.Months) != 12 || len(appliedIR.Weekdays) != 7 {
		t.Errorf("expected the fixed Gregorian structure to be applied, got %+v", appliedIR)
	}
}

// TestWizardCreate_RealWorld_ManualDateWhenNotTrackingRealTime: with the
// switch off, the review step shows (and submits) an explicit date exactly
// like every other source — this is the "reallife-but-manual" case model.go
// already documents.
func TestWizardCreate_RealWorld_ManualDateWhenNotTrackingRealTime(t *testing.T) {
	var created *Calendar
	calRepo := &fakeCalendarRepo{
		createFn: func(_ context.Context, cal *Calendar) error {
			created = cal
			return nil
		},
	}
	e, _ := newRealWorldTestRouter(calRepo)

	form := url.Values{}
	form.Set("source", "reallife")
	form.Set("real_time_zone", "America/New_York")
	form.Set("tracks_real_time", "false")
	form.Set("current_year", "1850")
	form.Set("current_month", "7")
	form.Set("current_day", "4")

	rec := doHTMXFormRequest(e, "/campaigns/camp-real/calendars/wizard/create", "u-owner", form)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("expected a redirect on success, got %d: %s", rec.Code, rec.Body.String())
	}
	if created == nil {
		t.Fatal("expected the calendar to be created")
	}
	if created.TracksRealTime {
		t.Error("expected TracksRealTime to be false")
	}
	// CreateCalendarFromImport only ever stores RealTimeZone when
	// TracksRealTime is true — see its own branch — so it must stay nil here
	// even though the form carried a zone (the same "ignored and cleared
	// when disabling" rule UpdateCalendarInput.RealTimeZone documents).
	if created.RealTimeZone != nil {
		t.Errorf("RealTimeZone = %v, want nil when not tracking real time", *created.RealTimeZone)
	}
	if created.CurrentYear != 1850 || created.CurrentMonth != 7 || created.CurrentDay != 4 {
		t.Errorf("current date = %d-%d-%d, want 1850-7-4", created.CurrentYear, created.CurrentMonth, created.CurrentDay)
	}
}

// TestWizardCreate_RealWorld_BadZoneReRendersRealWorldReview: a tampered
// (non-UI-reachable, since the browser only ever offers timeutil.
// CommonZones' own values) real_time_zone must be refused by
// CreateCalendarFromImport's own time.LoadLocation check and re-render the
// REAL-WORLD review step specifically — not the generic wizardReviewBody,
// which has no time-zone field to redisplay.
func TestWizardCreate_RealWorld_BadZoneReRendersRealWorldReview(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("source", "reallife")
	form.Set("real_time_zone", "Not/AZone")
	form.Set("tracks_real_time", "true")

	rec := doHTMXFormRequest(e, "/campaigns/camp-real/calendars/wizard/create", "u-owner", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("a bad zone must come back as an HTMX-swappable 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "" {
		t.Errorf("must not redirect on a validation failure, got HX-Redirect=%q", rec.Header().Get("HX-Redirect"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "IANA") {
		t.Errorf("expected the zone-validation error inline, body:\n%s", body)
	}
	if !strings.Contains(body, `name="real_time_zone"`) {
		t.Errorf("expected the re-rendered step to still be the real-world review (with its own zone select), body:\n%s", body)
	}
}
