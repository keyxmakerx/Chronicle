// v1_v2_cutover_test.go — C-CAL-V1-V2-CUTOVER. The V1 calendar VIEW routes
// (Index list, Show month, week, day) and the bare /calendar legacy path 301 to
// the V2 shell; the create flow keeps a stable entry (/calendars/new → the
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

// INVERTED BY C-CALV4-V2SUNSET R2-4 ([VS-9] SIGNED). It asserted
// "/campaigns/camp-1/calendar/v2/cal-1". THE NEW CLAIM: a retired V1 view route
// 301s to THE BENCH, the surface the product now links to everywhere.
//
// THE :calId IS DROPPED, and that is a real reduction: a bookmark to one
// specific V1 calendar lands on the Bench's default selection, because
// [VS-12] measured that AppDashboard reads `sort`, `y` and `m` and never
// `calId`. Booked as C-CALV4-BENCH-CALID. The 301 itself is unchanged — the
// target does not vary by requester on this leg.
func TestCutover_ShowRedirectsToTheBench(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "cal-1")
	if err := h.RedirectShowV2(c); err != nil {
		t.Fatalf("RedirectShowV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/apps/calendar")
}

// INVERTED BY C-CALV4-V2SUNSET R2-4 ([VS-9] SIGNED) — AND THE COMMENT MUST SAY
// WHY THE WEEK SEGMENT VANISHES, because a feature loss must not hide inside a
// green test.
//
// This asserted "/campaigns/camp-1/calendar/v2/cal-1/week". THE NEW CLAIM: it
// lands on the Bench, which is a MONTH.
//
// THE SEGMENT DOES NOT MOVE — IT HAS NOWHERE TO GO. The V2 shell's
// `case "week", "day"` (handler_v2.go) is the ONLY week view in the product.
// v4 has none: the Block is a month, the Shelf's tabs are Month / Upcoming /
// Filters / Almanac, and /schedule is a scheduling surface rather than a
// calendar week. So a /week bookmark now resolves to a month grid.
//
// THIS IS [VS-1]'s FIRST GAP, AND IT IS A PREREQUISITE, NOT A BOOKING.
// C-CALV4-WEEKDAY-VIEWS must MERGE before C-CALV4-SHELL-REMOVAL may start —
// deleting the shell today would delete the only week and day views that exist.
// When that slice lands, this test inverts once more, back toward a week.
func TestCutover_WeekRedirectsToTheBenchAndLosesTheWeek(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "cal-1")
	if err := h.RedirectWeekV2(c); err != nil {
		t.Fatalf("RedirectWeekV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/apps/calendar")
	if strings.Contains(rec.Header().Get("Location"), "week") {
		t.Error("a week segment reappeared — if v4 grew a week view, re-point this redirect at " +
			"it and say so, rather than letting the segment ride to a page that ignores it")
	}
}

// INVERTED BY C-CALV4-V2SUNSET R2-4 ([VS-9] SIGNED), same claim and same reason
// as the week above: the DAY view exists only inside the V2 shell, v4 has no
// replacement, and C-CALV4-WEEKDAY-VIEWS is the prerequisite slice that builds
// one. A /day bookmark lands on the Bench's month.
func TestCutover_DayRedirectsToTheBenchAndLosesTheDay(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "cal-1")
	if err := h.RedirectDayV2(c); err != nil {
		t.Fatalf("RedirectDayV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/apps/calendar")
	if strings.Contains(rec.Header().Get("Location"), "day") {
		t.Error("a day segment reappeared — see the week test's reasoning")
	}
}

// A redirect with no :calId (the dashboard-block fallthrough) lands on the bare
// V2 shell, which resolves the active calendar itself.
// INVERTED BY C-CALV4-V2SUNSET R2-4 ([VS-9] SIGNED): the no-:calId fallthrough
// lands on the Bench. It is now the SAME target as the with-:calId case above,
// which is the honest shape — the Bench cannot honour a calendar id, so
// pretending the two differ would be a lie in a URL.
func TestCutover_ShowRedirectNoCalIdGoesToTheBench(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "")
	if err := h.RedirectShowV2(c); err != nil {
		t.Fatalf("RedirectShowV2: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/apps/calendar")
}

// Index for a campaign WITH calendars 301s to the calendar (the list/single V1
// views retire).
//
// INVERTED BY C-CALV4-V2SUNSET R2-4 ([VS-9] SIGNED): the destination is the
// Bench. The cutover reasoning is untouched — only the surface that replaced
// the V1 views has itself been replaced.
func TestCutover_IndexWithCalendarsRedirectsToTheBench(t *testing.T) {
	h := NewHandler(&cutoverStub{cals: []Calendar{{ID: "cal-1", CampaignID: "camp-1"}}})
	c, rec := ownerCtx(t, "")
	if err := h.Index(c); err != nil {
		t.Fatalf("Index: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/apps/calendar")
}

// Index for a campaign with ZERO calendars still renders THE CREATE FLOW rather
// than bouncing to V2 — and since C-CALV4-WIZARD-P13 [WZ-13] SIGNED, the create
// flow is the BUILDER WIZARD rather than the three-card V1 chooser. A campaign's
// first calendar is exactly the case the wizard was designed for.
//
// PIN REFRESHED, NOT DELETED: the assertion still proves that a zero-calendar
// campaign gets a create surface at 200 and never a redirect. Only the surface
// it names changed, and the string it looks for is one the wizard's Start
// station actually prints.
func TestCutover_IndexNoCalendarsShowsSetup(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "")
	if err := h.Index(c); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (the create flow)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Start from a shape you know") {
		t.Errorf("Index with 0 calendars should render the builder wizard")
	}
}

// TestCutover_IndexNoCalendarsIsOwnerOnly is the pin that was missing when the
// zero-calendar branch was re-pointed at the wizard.
//
// Index is registered on the PUBLIC group — routes.go: pub.GET("/calendars",
// h.Index, campaigns.RequireViewAccess()) — and RequireViewAccess admits any
// member at any role AND, on a public campaign, an authenticated non-member or
// a logged-out visitor at RoleNone. The old zero-calendar branch rendered the
// V1 chooser, which carried no honesty chips; the new one renders the BUILDER,
// which carries two `needs backend` chips and a Create button, and
// decisions/2026-07-27-needs-backend-audience.md is explicit that "for a player
// the zone simply does not appear".
//
// So: a non-owner with zero calendars goes to V2, whose own empty state is the
// designed surface for exactly this case ("No calendar yet … Ask the campaign
// owner to add one"). 302 rather than 301 — a PERMANENT redirect whose target
// depends on the requester's role is a cache poisoning waiting to happen.
func TestCutover_IndexNoCalendarsIsOwnerOnly(t *testing.T) {
	for _, role := range []campaigns.Role{campaigns.RoleNone, campaigns.RolePlayer, campaigns.RoleScribe} {
		h := NewHandler(&cutoverStub{})
		e := echo.New()
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
		c.Set("campaign_context", &campaigns.CampaignContext{
			Campaign: &campaigns.Campaign{ID: "camp-1", IsPublic: true}, MemberRole: role,
		})
		if err := h.Index(c); err != nil {
			t.Fatalf("Index at role %d: %v", role, err)
		}
		// INVERTED BY C-CALV4-V2SUNSET R2-4 ([VS-9] SIGNED — the SEVENTH
		// inversion, and the one the dispatch could not have predicted because
		// this test did not exist when it was scouted). It required the V2
		// shell. THE NEW CLAIM: a non-owner with zero calendars is sent to the
		// BENCH, whose own empty state ("No calendar yet") is now the designed
		// surface for this case.
		//
		// EVERYTHING THIS TEST ACTUALLY GUARDS IS UNCHANGED: the owner gate, the
		// 302-not-301 (a permanent redirect whose target depends on the
		// requester's ROLE is a cache poisoning waiting to happen), and the
		// wizard markup never reaching a non-owner. Only the destination moved.
		if rec.Code != http.StatusFound {
			t.Errorf("role %d: status=%d want 302 — a non-owner must never "+
				"reach the builder", role, rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/campaigns/camp-1/apps/calendar" {
			t.Errorf("role %d: Location=%q want the Bench", role, loc)
		}
		if body := rec.Body.String(); strings.Contains(body, "wz-shell") ||
			strings.Contains(body, "wz-need") {
			t.Errorf("role %d: wizard markup reached a non-owner", role)
		}
	}
}

// ShowBuilder is the stable create entry — it renders regardless of how many
// calendars already exist (so "New calendar" can add a second).
//
// PIN REFRESHED from ShowSetup under [WZ-13].
func TestCutover_ShowBuilderRendersTheWizard(t *testing.T) {
	h := NewHandler(&cutoverStub{cals: []Calendar{{ID: "cal-1", CampaignID: "camp-1"}}})
	c, rec := ownerCtx(t, "")
	if err := h.ShowBuilder(c); err != nil {
		t.Fatalf("ShowBuilder: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
}

// The bare /calendar legacy path 301s straight to the campaign's calendar.
//
// INVERTED BY C-CALV4-V2SUNSET R2-4 ([VS-9] SIGNED). This path has pointed at
// V1 /calendars, then at the V2 shell, and now at the Bench, and the ROUTE has
// never moved once — which is the whole reason it exists. A bookmark made at
// any point in that history still lands on whatever the campaign's calendar
// currently is.
func TestCutover_LegacyRedirectGoesToTheBench(t *testing.T) {
	h := NewHandler(&cutoverStub{})
	c, rec := ownerCtx(t, "")
	if err := h.legacyRedirect(c); err != nil {
		t.Fatalf("legacyRedirect: %v", err)
	}
	assertMovedPermanently(t, rec, "/campaigns/camp-1/apps/calendar")
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
		"/campaigns/:id/calendars/:calId/timeline": "ShowTimeline",  // PRESERVE
		"/campaigns/:id/calendars/:calId/embed":    "EmbedCalendar", // PRESERVE
		// PIN REFRESHED by C-CALV4-WIZARD-P13 [WZ-13] SIGNED: the stable create
		// entry now resolves to the builder wizard. The route is unchanged and
		// therefore so is the snapshot; only the handler behind it moved, which
		// is what makes every existing link and every external bookmark land on
		// the designed surface with no href edited.
		"/campaigns/:id/calendars/new": "ShowBuilder", // stable create
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
