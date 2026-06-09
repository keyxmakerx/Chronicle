// v1_v2_cutover_test.go — C-CAL-V1-V2-CUTOVER. The V1 calendar VIEW routes
// (Index list, Show month, week, day) and the bare /calendar legacy path 301 to
// the V2 shell; the create flow keeps a stable entry (/calendars/new → setup
// chooser); and the TIMELINE + EMBED routes are PRESERVED (no V2 equivalent
// yet — Timeline V2 is a deferred arc, V2 has no standalone embed). These tests
// pin the redirect targets and the route-table so a future edit can't silently
// re-point a preserved route at the redirect (or vice versa).
package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// cutoverStub backs only ListCalendars — Index branches on the calendar count.
type cutoverStub struct {
	CalendarService
	cals []Calendar
}

func (s *cutoverStub) ListCalendars(_ context.Context, _ string) ([]Calendar, error) {
	return s.cals, nil
}

// ownerCtx builds an echo.Context with an Owner campaign context for camp-1,
// optionally seeding the :calId path param.
func ownerCtx(t *testing.T, calID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner,
	})
	if calID != "" {
		c.SetParamNames("calId")
		c.SetParamValues(calID)
	}
	return c, rec
}

// assertMovedPermanently fails unless the recorder holds a 301 to want.
func assertMovedPermanently(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status=%d want 301", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != want {
		t.Errorf("Location=%q want %q", loc, want)
	}
}

func TestCutover_ShowRedirectsToV2(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "cal-1")
	if err := h.RedirectShowV2(c); err != nil {
		t.Fatalf("RedirectShowV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/calendar/v2/cal-1")
}

func TestCutover_WeekRedirectsToV2(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "cal-1")
	if err := h.RedirectWeekV2(c); err != nil {
		t.Fatalf("RedirectWeekV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/calendar/v2/cal-1/week")
}

func TestCutover_DayRedirectsToV2(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "cal-1")
	if err := h.RedirectDayV2(c); err != nil {
		t.Fatalf("RedirectDayV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/calendar/v2/cal-1/day")
}

// A redirect with no :calId (the dashboard-block fallthrough) lands on the bare
// V2 shell, which resolves the active calendar itself.
func TestCutover_ShowRedirectNoCalIdGoesToBareV2(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "")
	if err := h.RedirectShowV2(c); err != nil {
		t.Fatalf("RedirectShowV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/calendar/v2")
}

// Index for a campaign WITH calendars 301s to V2 (the list/single views retire).
func TestCutover_IndexWithCalendarsRedirectsToV2(t *testing.T) {
	h := NewHandler(&cutoverStub{cals: []Calendar{{ID: "cal-1", CampaignID: "camp-1"}}})
	c, rec := ownerCtx(t, "")
	if err := h.Index(c); err != nil {
		t.Fatalf("Index: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/calendar/v2")
}

// Index for a campaign with ZERO calendars still renders the setup chooser (the
// create flow) rather than bouncing to V2.
func TestCutover_IndexNoCalendarsShowsSetup(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "")
	if err := h.Index(c); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (setup chooser)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sync to Real Life") {
		t.Errorf("Index with 0 calendars should render the setup chooser")
	}
}

// ShowSetup is the stable create entry — it renders the chooser regardless of
// how many calendars already exist (so "New calendar" can add a second).
func TestCutover_ShowSetupRendersChooser(t *testing.T) {
	h := NewHandler(&cutoverStub{cals: []Calendar{{ID: "cal-1", CampaignID: "camp-1"}}})
	c, rec := ownerCtx(t, "")
	if err := h.ShowSetup(c); err != nil {
		t.Fatalf("ShowSetup: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
}

// The bare /calendar legacy path 301s straight to V2.
func TestCutover_LegacyRedirectGoesToV2(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "")
	if err := h.legacyRedirect(c); err != nil {
		t.Fatalf("legacyRedirect: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/calendar/v2")
}

// TestCutover_RouteTablePreservesTimelineAndEmbed walks the registered route
// table and pins which V1 view routes redirect to V2 versus which are PRESERVED
// (served by their real handler). Middleware is never invoked here — registration
// only stores the service refs — so nil services are safe for the route walk.
func TestCutover_RouteTablePreservesTimelineAndEmbed(t *testing.T) {
	e := echo.New()
	RegisterRoutes(e, NewHandler(&cutoverStub{}), nil, nil, nil)

	// path → substring its handler's func name must contain.
	want := map[string]string{
		"/campaigns/:id/calendars":                 "Index",
		"/campaigns/:id/calendars/:calId":          "RedirectShowV2",
		"/campaigns/:id/calendars/:calId/week":     "RedirectWeekV2",
		"/campaigns/:id/calendars/:calId/day":      "RedirectDayV2",
		"/campaigns/:id/calendars/:calId/timeline": "ShowTimeline",   // PRESERVE
		"/campaigns/:id/calendars/:calId/embed":    "EmbedCalendar",  // PRESERVE
		"/campaigns/:id/calendars/new":             "ShowSetup",      // stable create
	}
	got := map[string]string{}
	for _, r := range e.Routes() {
		if r.Method == http.MethodGet {
			got[r.Path] = r.Name
		}
	}
	for path, sub := range want {
		name, ok := got[path]
		if !ok {
			t.Errorf("GET %s not registered", path)
			continue
		}
		if !strings.Contains(name, sub) {
			t.Errorf("GET %s → handler %q, want it to contain %q", path, name, sub)
		}
	}
}
