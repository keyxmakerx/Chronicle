// app_routes_test.go covers the calendar on an allowed app's routes (the
// Foundry calendar window): the fragment it mounts, and that every route
// keeps its site gate there.
package calendar

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newAppTestRouter mounts RegisterAppRoutes on a group shaped like the notes
// grant group: an authenticated member, then the live campaign access check.
// The session cookie stands in for the grant, which notes tests cover.
func newAppTestRouter(addonEnabled bool, roles map[string]campaigns.Role) (*echo.Echo, *fakeCalendarSvc) {
	e := echo.New()
	e.Use(emw.Recover())
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		if he, ok := err.(*echo.HTTPError); ok {
			_ = c.NoContent(he.Code)
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	svc := &fakeCalendarSvc{}
	h := NewHandler(svc)
	campaignSvc := guardCampaignSvc{roles: roles}
	g := e.Group("/api/notes-app/campaigns/:id",
		auth.RequireAuth(guardAuthSvc{}),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	RegisterAppRoutes(g, h, guardAddonSvc{enabled: addonEnabled})
	return e, svc
}

const appBase = "/api/notes-app/campaigns/camp-1"

func TestAppRoutes_EmbedFragmentIsTheMembersCalendar(t *testing.T) {
	e, svc := newAppTestRouter(true, map[string]campaigns.Role{"u-player": campaigns.RolePlayer})
	rec := doRequest(e, http.MethodGet, appBase+"/calendars/embed", "u-player")
	if rec.Code != http.StatusOK {
		t.Fatalf("embed: got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`data-widget="calendar_view"`, `data-calendar-id="cal-default"`, `data-can-edit="false"`, "calendar-view.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("embed fragment missing %s, body:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<html") {
		t.Errorf("embed must be a fragment, not a page")
	}
	if svc.lastViewer.UserID() != "u-player" || svc.lastViewer.Role() != int(campaigns.RolePlayer) {
		t.Errorf("embed must read as the member, got role=%d user=%q", svc.lastViewer.Role(), svc.lastViewer.UserID())
	}
}

func TestAppRoutes_EmbedWithoutACalendar(t *testing.T) {
	e, svc := newAppTestRouter(true, map[string]campaigns.Role{"u-player": campaigns.RolePlayer})
	svc.noDefault = true
	rec := doRequest(e, http.MethodGet, appBase+"/calendars/embed", "u-player")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no calendar yet") {
		t.Fatalf("no calendar: got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAppRoutes_KeepSiteGates(t *testing.T) {
	roles := map[string]campaigns.Role{
		"u-player": campaigns.RolePlayer,
		"u-scribe": campaigns.RoleScribe,
		"u-owner":  campaigns.RoleOwner,
	}
	cases := []struct {
		name, method, path, user, body string
		addon                          bool
		want                           int
	}{
		{"player reads events", http.MethodGet, "/calendars/cal-1/events?year=1&month=1", "u-player", "", true, http.StatusOK},
		{"player cannot add an event", http.MethodPost, "/calendars/cal-1/events", "u-player", `{"name":"x","year":1,"month":1,"day":1}`, true, http.StatusForbidden},
		{"scribe adds an event", http.MethodPost, "/calendars/cal-1/events", "u-scribe", `{"name":"x","year":1,"month":1,"day":1}`, true, http.StatusCreated},
		{"scribe cannot delete an event", http.MethodDelete, "/calendars/cal-1/events/evt-1", "u-scribe", "", true, http.StatusForbidden},
		{"player cannot set a day's weather", http.MethodPut, "/calendars/cal-1/weather/days", "u-player", `{"days":[]}`, true, http.StatusForbidden},
		{"player cannot list event kinds", http.MethodGet, "/calendars/event-kinds", "u-player", "", true, http.StatusForbidden},
		{"non-member is refused", http.MethodGet, "/calendars/embed", "u-stranger", "", true, http.StatusForbidden},
		{"calendar addon off", http.MethodGet, "/calendars/embed", "u-player", "", false, http.StatusNotFound},
		{"calendar settings stay on the site", http.MethodPut, "/calendars/cal-1", "u-owner", `{}`, true, http.StatusNotFound},
		{"structure editor stays on the site", http.MethodPost, "/calendars/cal-1/structure", "u-owner", `{}`, true, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newAppTestRouter(tc.addon, roles)
			rec := doRequestWithBody(e, tc.method, appBase+tc.path, tc.user, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("%s %s as %s: got %d, want %d: %s", tc.method, tc.path, tc.user, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
