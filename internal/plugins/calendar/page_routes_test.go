// page_routes_test.go covers the calendar list, preview and wizard routes end to end
// through the same access-test harness access_test.go builds (real
// RegisterRoutes, real middleware chain, fakeCalendarSvc standing in for the
// business logic service_test.go already covers): the calendars list page,
// the per-card preview fragment, and the new-calendar wizard's Start step.
package calendar

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newHTMXRequest builds a request carrying the HX-Request header
// (middleware.IsHTMX's own check) plus the session cookie doRequest uses.
func newHTMXRequest(method, path, userID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("HX-Request", "true")
	if userID != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: userID})
	}
	return req
}

// serve runs req through e and returns the recorded response.
func serve(e *echo.Echo, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestCalendarsListPage_Unauthenticated(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{})
	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars", "")
	if !isLoginRedirect(rec) {
		t.Fatalf("expected a login redirect for an unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCalendarsListPage_PlayerSeesReadOnlyView(t *testing.T) {
	roles := map[string]campaigns.Role{"u-player": campaigns.RolePlayer}
	e, _ := newAccessTestRouter(false, true, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars", "u-player")
	if rec.Code != http.StatusOK {
		t.Fatalf("Player must be able to view the calendars list, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "New calendar") {
		t.Errorf("a Player must not see the owner-only \"New calendar\" action, body:\n%s", body)
	}
	if strings.Contains(body, "fa-file-import") {
		t.Errorf("a Player must not see the owner-only \"Import\" action, body:\n%s", body)
	}
	if !strings.Contains(body, "The Secret Calendar") {
		t.Errorf("expected the fake service's calendar to render, body:\n%s", body)
	}
}

// TestCalendarsListPage_AnnouncesOnlyAListedCalendar: the post-create toast
// names a calendar only when ?new= is one the viewer can see, and uses its
// stored name; the URL's own text is never shown.
func TestCalendarsListPage_AnnouncesOnlyAListedCalendar(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, true, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars?created=Injected+text&new=cal-1", "u-owner")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("list page: got %d: %s", rec.Code, body)
	}
	if !strings.Contains(body, `data-created-name="The Secret Calendar"`) || !strings.Contains(body, `data-created-id="cal-1"`) {
		t.Errorf("a listed calendar must be announced by its stored name, body:\n%s", body)
	}
	if strings.Contains(body, "Injected text") {
		t.Errorf("the URL's created= text must never reach the page, body:\n%s", body)
	}

	rec = doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars?created=Injected+text&new=cal-unknown", "u-owner")
	body = rec.Body.String()
	if !strings.Contains(body, `data-created-name=""`) || strings.Contains(body, "Injected text") {
		t.Errorf("an id that isn't listed must announce nothing, body:\n%s", body)
	}
}

func TestCalendarsListPage_OwnerSeesOwnerActions(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, true, roles)

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars", "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("Owner must be able to view the calendars list, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "New calendar") {
		t.Errorf("expected the owner-only \"New calendar\" action, body:\n%s", body)
	}
	if !strings.Contains(body, "fa-file-import") {
		t.Errorf("expected the owner-only \"Import\" action, body:\n%s", body)
	}
}

func TestCalendarsListPage_FullPageVsHTMXFragment(t *testing.T) {
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, true, roles)

	full := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars", "u-owner")
	if !strings.Contains(strings.ToLower(full.Body.String()), "<!doctype html") {
		t.Errorf("a direct GET must render the full page (with its document shell), got:\n%s", full.Body.String())
	}

	req := newHTMXRequest(http.MethodGet, "/campaigns/camp-1/calendars", "u-owner")
	rec := serve(e, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for the HTMX fragment request, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "<!doctype html") {
		t.Errorf("an HTMX request must get the fragment only, not the full page shell, got:\n%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "calv5-cards") {
		t.Errorf("expected the card grid in the fragment, got:\n%s", rec.Body.String())
	}
}

func TestCalendarPreview_Unauthenticated(t *testing.T) {
	e, _ := newAccessTestRouter(false, true, map[string]campaigns.Role{})
	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1/preview", "")
	if !isLoginRedirect(rec) {
		t.Fatalf("expected a login redirect for an unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCalendarPreview_PlayerAndOwnerCanOpenIt(t *testing.T) {
	roles := map[string]campaigns.Role{"u-player": campaigns.RolePlayer, "u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, true, roles)

	cases := []struct {
		user    string
		canEdit string
	}{
		{"u-player", `data-can-edit="false"`},
		{"u-owner", `data-can-edit="true"`},
	}
	for _, tc := range cases {
		rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/cal-1/preview", tc.user)
		if rec.Code != http.StatusOK {
			t.Errorf("%s must be able to open the preview, got %d: %s", tc.user, rec.Code, rec.Body.String())
			continue
		}
		body := rec.Body.String()
		for _, want := range []string{"The Secret Calendar", `data-alm="unfold"`, `data-api-base="/campaigns/camp-1/calendars/cal-1"`, tc.canEdit} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the preview is missing %q, body:\n%s", tc.user, want, body)
			}
		}
		// The calendar waits inside the template, so it mounts only when the
		// preview unfolds, never as the preview itself is swapped in.
		open, mount, shut := strings.Index(body, `<template id="calv5-alm-tpl">`), strings.Index(body, `data-widget="calendar_view"`), strings.Index(body, "</template>")
		if open < 0 || mount < open || shut < mount {
			t.Errorf("%s: the calendar's mount must sit inside the preview's template, body:\n%s", tc.user, body)
		}
		if strings.Contains(body, `href="/campaigns/camp-1/calendars/cal-1/view"`) {
			t.Errorf("%s: the preview unfolds into the calendar in place; it must not link away to it, body:\n%s", tc.user, body)
		}
		if strings.Contains(body, "coming soon") {
			t.Errorf("%s: the full calendar view exists; the preview must not call it coming soon, body:\n%s", tc.user, body)
		}
	}
}

func TestWizardStart_GateAndContent(t *testing.T) {
	roles := map[string]campaigns.Role{"u-player": campaigns.RolePlayer, "u-owner": campaigns.RoleOwner}
	e, _ := newAccessTestRouter(false, true, roles)

	if rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/wizard", ""); !isLoginRedirect(rec) {
		t.Errorf("expected a login redirect for an unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/wizard", "u-player"); rec.Code != http.StatusForbidden {
		t.Errorf("a Player must be forbidden from the wizard, got %d: %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(e, http.MethodGet, "/campaigns/camp-1/calendars/wizard", "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("Owner must be able to open the wizard, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Real-world calendar", "Start from a preset", "Build your own", "Import a file", "Generate one"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the Start step to mention %q, body:\n%s", want, body)
		}
	}
	if !strings.Contains(body, `hx-get="/campaigns/camp-1/calendars/wizard/generate"`) {
		t.Errorf("the \"Generate one\" tile must be an active control (hx-get to the generate step), body:\n%s", body)
	}
	if strings.Contains(body, "Under construction") {
		t.Errorf("the \"Generate one\" tile is no longer disabled; it must not carry the old badge, body:\n%s", body)
	}
	if strings.Contains(body, "For your table") == false || strings.Contains(body, "For your world") == false {
		t.Errorf("expected the Start step's two purpose groupings, body:\n%s", body)
	}
	// Scoped, not a guess at specific class names (which a new icon on any
	// tile could slip past): no "fa-" anywhere between the two tilegrp
	// blocks and the reassurance line just past them. The dialog's own
	// close-button icon renders earlier, in wizardShell's chrome, and stays
	// out of this window on purpose.
	start := strings.Index(body, "calv5-tilegrp")
	end := strings.Index(body, "calv5-reassure")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("expected to find both the tile groups and the reassurance line, body:\n%s", body)
	}
	if tiles := body[start:end]; strings.Contains(tiles, "fa-") {
		t.Errorf("no tile may carry a decorative icon (fa-*), tiles region:\n%s", tiles)
	}
}
