// app_dashboard_test.go — C-APPS-CAL-DASH-W1, refreshed by C-CALV4-BENCH-P4.
//
// Two surfaces are covered here now, and the split is deliberate:
//
//   - THE ROUTE'S PAGE. GET /campaigns/:id/apps/calendar renders the Bench, so
//     every assertion that ever meant "what the Calendar tab shows" renders
//     BenchPage. The guarantees are unchanged — every visible calendar is
//     reachable, the empty and load-error states are friendly and role-aware,
//     selection is a full navigation and not an HTMX detail swap.
//   - THE RETAINED CARD-GRID COMPONENTS. calendarAppDashboardDetail and its
//     children are no longer fed by the route but are RETAINED (dispatch
//     Bounds: dead-code removal is a post-wave slice), so their tests are kept
//     verbatim rather than deleted. They are marked below so nobody mistakes
//     them for coverage of the live page.
//
// The calendar-side EntitiesForCalendar service passthrough is untouched: the
// read is still on the service and still forwards the viewer context.
package calendar

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The service delegates EntitiesForCalendar to the repo, forwarding the id AND
// the viewer context (role + userID) so entity visibility can be enforced at
// the SQL layer (cordinator#32 gap #1). If this seam ever drops role/userID,
// the repo can't filter and the leak returns.
func TestEntitiesForCalendar_ServiceDelegates(t *testing.T) {
	var gotID, gotUser string
	var gotRole int
	repo := &mockCalendarRepo{
		entitiesForCalendarFn: func(_ context.Context, calendarID string, role int, userID string) ([]EntityTieRef, error) {
			gotID, gotRole, gotUser = calendarID, role, userID
			return []EntityTieRef{{EntityID: "e1", EntityName: "Gandalf"}}, nil
		},
	}
	svc := NewCalendarService(repo)
	out, err := svc.EntitiesForCalendar(context.Background(), "cal-1", 1, "user-7")
	if err != nil {
		t.Fatalf("EntitiesForCalendar: %v", err)
	}
	if gotID != "cal-1" {
		t.Errorf("repo got id %q, want cal-1", gotID)
	}
	if gotRole != 1 || gotUser != "user-7" {
		t.Errorf("viewer context not forwarded to repo: role=%d user=%q, want role=1 user=user-7", gotRole, gotUser)
	}
	if len(out) != 1 || out[0].EntityName != "Gandalf" {
		t.Errorf("unexpected passthrough result: %+v", out)
	}
}

func renderDashboardPage(t *testing.T, data CalendarAppDashboardData) string {
	t.Helper()
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: data.CampaignID, Name: "Test"}}
	var buf bytes.Buffer
	if err := CalendarAppDashboardPage(cc, data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render page: %v", err)
	}
	return buf.String()
}

func renderDashboardDetail(t *testing.T, data CalendarAppDashboardData) string {
	t.Helper()
	var buf bytes.Buffer
	if err := calendarAppDashboardDetail(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render detail: %v", err)
	}
	return buf.String()
}

func sampleDashboardData() CalendarAppDashboardData {
	sel := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos", Mode: ModeFantasy, CurrentYear: 1492, CurrentMonth: 4, CurrentDay: 14, IsDefault: true}
	return CalendarAppDashboardData{
		CampaignID: "camp-1",
		Calendars: []Calendar{
			*sel,
			{ID: "cal-2", CampaignID: "camp-1", Name: "Gregorian", Mode: ModeRealLife, CurrentYear: 2026},
		},
		Selected:  sel,
		ActiveID:  "cal-1",
		Entities:  []EntityTieRef{{EntityID: "e1", EntityName: "Gandalf", EntityType: "npc"}},
		Timelines: []TimelineRef{{ID: "t1", Name: "Main Arc", EventCount: 7}},
		IsOwner:   true,
		CSRFToken: "csrf",
	}
}

// THE ROUTE'S PAGE. Predecessor: TestCalendarAppDashboardPage_ListAndDetail,
// which pinned "every calendar the viewer may see is on the page, with a door
// to it and — for an owner — a door to its settings". That guarantee is
// unchanged; only the shape it takes is (a Block, a subordinate row, or a
// named attention item, never a card grid).
func TestCalendarAppPage_ReachesEveryVisibleCalendar(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	for _, want := range []string{
		"data-cal-dashboard",      // the page container marker, kept for continuity
		"data-cal-bench",          // the Bench itself
		"data-bench-ribbon",       // the ribbon
		"data-cal-dashboard-list", // the subordinate-row list
		// every visible calendar is named somewhere on the page…
		"Harptos of Imix", "Real world / Gregorian", "Elven Reckoning",
		// …including the misconfigured one, whose NAME reaches the GM through
		// the attention tile even though its row prints the fault instead.
		"Dwarven Deep-count",
		// doors to the existing surfaces — no CRUD is reimplemented:
		"/campaigns/camp-1/calendar/v2/cal-harptos",        // Open calendar
		"/campaigns/camp-1/calendars/cal-harptos/settings", // Builder / Settings
		"/campaigns/camp-1/calendars/cal-elven/settings",   // per-row settings
		"/campaigns/camp-1/calendars/new",                  // the New-calendar slot
	} {
		if !strings.Contains(html, want) {
			t.Errorf("bench page missing %q", want)
		}
	}
}

// RETAINED COMPONENT. When the selected calendar isn't the user's active one,
// the detail offers the "Set active" compose action (POST to the existing
// switch endpoint).
func TestCalendarAppDashboard_SetActiveWhenNotActive(t *testing.T) {
	data := sampleDashboardData()
	data.ActiveID = "cal-2" // selected cal-1 is not active → switch offered
	html := renderDashboardDetail(t, data)
	if !strings.Contains(html, "/campaigns/camp-1/calendar/v2/switch") {
		t.Errorf("non-active selection should offer the Set active action")
	}
	if !strings.Contains(html, `name="calendar_id" value="cal-1"`) {
		t.Errorf("switch form should carry the selected calendar id")
	}
}

// RETAINED COMPONENT. NOTE ON THE LAST ASSERTION: the "non-owner still gets
// the Open action" check passes on `/campaigns/camp-1/calendars/cal-1`, which
// is the "see in action" embed's hx-get (…/calendars/cal-1/embed), NOT the Open
// button — the Open button targets `/campaigns/camp-1/calendar/v2/cal-1`
// (singular). Read before rewriting, or the wrong intent gets preserved.
func TestCalendarAppDashboard_OwnerGating(t *testing.T) {
	owner := renderDashboardDetail(t, sampleDashboardData())
	if !strings.Contains(owner, "/calendars/cal-1/settings") || !strings.Contains(owner, "Delete") {
		t.Errorf("owner should see Settings + Delete compose actions")
	}
	data := sampleDashboardData()
	data.IsOwner = false
	player := renderDashboardDetail(t, data)
	if strings.Contains(player, "/calendars/cal-1/settings") {
		t.Errorf("non-owner must not see the Settings action")
	}
	if strings.Contains(player, ">Delete<") || strings.Contains(player, "Delete this calendar") {
		t.Errorf("non-owner must not see the Delete action")
	}
	// Non-owners can still open the calendar (read).
	if !strings.Contains(player, "/campaigns/camp-1/calendars/cal-1") {
		t.Errorf("non-owner should still get the Open action")
	}
}

// RETAINED COMPONENT.
func TestCalendarAppDashboard_NoAssociations(t *testing.T) {
	data := sampleDashboardData()
	data.Entities = nil
	data.Timelines = nil
	html := renderDashboardDetail(t, data)
	if !strings.Contains(html, "Linked entities (0)") || !strings.Contains(html, "No entities are linked") {
		t.Errorf("empty entities panel missing friendly state")
	}
	if !strings.Contains(html, "Timelines (0)") || !strings.Contains(html, "No timelines use this calendar") {
		t.Errorf("empty timelines panel missing friendly state")
	}
}

// THE ROUTE'S PAGE. The role-aware empty state is REUSED verbatim by the Bench
// (calendarAppDashboardEmpty), which is why it keeps its markers: an owner is
// prompted to create a calendar, a player gets the calm "nothing shared yet"
// with no create affordance, and neither gets the retired detail pane.
func TestCalendarAppDashboard_EmptyStates(t *testing.T) {
	empty := renderBench(t, BenchData{CampaignID: "camp-1", CampaignName: "Imix", IsOwner: true, IsGM: true})
	for _, want := range []string{"data-cal-dashboard-empty", "No calendars yet", "Create calendar", "/campaigns/camp-1/calendars"} {
		if !strings.Contains(empty, want) {
			t.Errorf("no-calendars state missing %q", want)
		}
	}
	for _, forbidden := range []string{`id="cal-dash-detail"`, "data-bench-ribbon", "data-bench-stack"} {
		if strings.Contains(empty, forbidden) {
			t.Errorf("a campaign with no calendars must not render %q", forbidden)
		}
	}
	player := renderBench(t, BenchData{CampaignID: "camp-1", CampaignName: "Imix"})
	if !strings.Contains(player, "data-cal-dashboard-empty-player") {
		t.Error("a player with nothing shared gets the calm empty state")
	}
	if strings.Contains(player, "Create calendar") {
		t.Error("a player gets no create affordance")
	}

	// RETAINED COMPONENT (see the file header): the card grid's detail pane.
	noSel := renderDashboardDetail(t, CalendarAppDashboardData{CampaignID: "camp-1"})
	if !strings.Contains(noSel, "data-cal-dashboard-detail-empty") || !strings.Contains(noSel, "Select a calendar") {
		t.Errorf("no-selection detail missing the prompt")
	}
}

// THE ROUTE'S PAGE. A failed list degrades to the same friendly card, with the
// same marker, so the operator-facing behaviour is byte-identical.
func TestCalendarAppDashboard_LoadError(t *testing.T) {
	html := renderBench(t, BenchData{CampaignID: "camp-1", CampaignName: "Imix", LoadError: true})
	if !strings.Contains(html, "data-cal-dashboard-error") || !strings.Contains(html, "load the calendars") {
		t.Errorf("load-error state missing friendly message")
	}
	if strings.Contains(html, "data-bench-stack") {
		t.Error("a load error must not render a half-built bench")
	}
}

// --- W2: live "see in action" embeds (C-APPS-CAL-DASH-W2) ---
//
// RETAINED COMPONENTS from here down (see the file header): the detail pane and
// its embeds are no longer fed by the route. Their tests are kept verbatim
// rather than deleted — including the engine-singleton count below, which is
// the invariant that a page may never mount two #cal-v2-worldstate surfaces.

func sampleW2Active() CalendarAppDashboardData {
	d := sampleDashboardData()
	d.SelectedIsActive = true
	d.WorldState = &WorldStateSeed{TimeOfDay: 0.5, Season: "Spring", Date: WorldStateDate{1492, 4, 14}, Weather: WorldStateWeather{Type: "rain", Intensity: 1}}
	d.WorldStateJSON = `{"timeOfDay":0.5}`
	return d
}

// Active selected calendar → the LIVE worldstate band renders (engine seed +
// sky scaffold + the shared engine), exactly one surface.
func TestCalendarAppDashboard_LiveWorldstateWhenActive(t *testing.T) {
	html := renderDashboardDetail(t, sampleW2Active())
	for _, want := range []string{
		"data-cal-dashboard-seeinaction",
		"data-cal-dashboard-worldstate", // the live band wrapper
		"id=\"cal-v2-worldstate\"",      // engine prod-mode seed blob
		"data-cal-sky",                  // sky scaffold (band)
		"cal-almanac-shelf",             // the hourglass shelf
		"/static/js/cal-almanac.js",     // the shared engine
	} {
		if !strings.Contains(html, want) {
			t.Errorf("active worldstate embed missing %q", want)
		}
	}
	if strings.Contains(html, "data-cal-dashboard-worldstate-note") {
		t.Errorf("active calendar should not show the 'set active' note")
	}
	// Exactly ONE live worldstate surface (engine-singleton).
	if n := strings.Count(html, "id=\"cal-v2-worldstate\""); n != 1 {
		t.Errorf("expected exactly one #cal-v2-worldstate surface; got %d", n)
	}
}

// Non-active selected calendar → NO live worldstate; the friendly note + the
// engine-free grid instead (the nuance default, no widget surgery).
func TestCalendarAppDashboard_NonActiveNoWorldstate(t *testing.T) {
	d := sampleDashboardData() // SelectedIsActive=false, WorldState nil
	html := renderDashboardDetail(t, d)
	if !strings.Contains(html, "data-cal-dashboard-worldstate-note") {
		t.Errorf("non-active calendar should show the 'set active' note")
	}
	for _, forbidden := range []string{"id=\"cal-v2-worldstate\"", "data-cal-sky"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("non-active calendar must not render the live worldstate (%q)", forbidden)
		}
	}
	// The engine-free month grid lazy-loads for ANY selected calendar.
	if !strings.Contains(html, `hx-get="/campaigns/camp-1/calendars/cal-1/embed"`) {
		t.Errorf("grid embed lazy-load missing")
	}
}

// The calendar-in-action grid lazy-loads via the existing embed route.
func TestCalendarAppDashboard_GridEmbedLazyLoad(t *testing.T) {
	html := renderDashboardDetail(t, sampleW2Active())
	if !strings.Contains(html, `hx-get="/campaigns/camp-1/calendars/cal-1/embed"`) {
		t.Errorf("grid should hx-get the existing embed route")
	}
	if !strings.Contains(html, `hx-trigger="load"`) || !strings.Contains(html, "data-cal-dashboard-grid") {
		t.Errorf("grid should lazy-load on insertion")
	}
}

// Associated timelines render as the shipped timeline-viz widget mounts.
func TestCalendarAppDashboard_TimelinePreviews(t *testing.T) {
	html := renderDashboardDetail(t, sampleW2Active())
	for _, want := range []string{
		"data-cal-dashboard-timeline-previews",
		`data-widget="timeline-viz"`,
		`data-timeline-id="t1"`,
		`data-api-url="/campaigns/camp-1/timelines/t1/data"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("timeline preview missing %q", want)
		}
	}
	// No associated timelines → no preview block.
	noTL := sampleW2Active()
	noTL.Timelines = nil
	if strings.Contains(renderDashboardDetail(t, noTL), "data-cal-dashboard-timeline-previews") {
		t.Errorf("no timelines → no preview block")
	}
}

// RETAINED COMPONENT + a live guard. The card grid's page loads D3 (for
// timeline-viz) only when the selection has timelines, and W1 (R1) pinned that
// D3 is the VENDORED copy (`/static/vendor/d3.min.js`), not the jsdelivr CDN —
// `script-src 'self'` blocked the CDN script so the viz never ran.
//
// The CSP half of that guard is what still matters on the live page, so it is
// asserted against the Bench too: the Bench renders no timeline previews and
// must therefore load neither D3 nor, ever, a CDN script.
func TestCalendarAppDashboard_LoadsD3ForTimelines(t *testing.T) {
	const d3Src = "/static/vendor/d3.min.js"
	bench := renderBench(t, benchFxData(true, true))
	if strings.Contains(bench, "jsdelivr") {
		t.Errorf("the Bench must never load a CDN script (CSP script-src 'self')")
	}
	if strings.Contains(bench, d3Src) {
		t.Errorf("the Bench renders no timeline previews, so it must not load D3")
	}

	withTL := renderDashboardPage(t, sampleW2Active())
	if !strings.Contains(withTL, d3Src) {
		t.Errorf("page should load vendored D3 when there are timeline previews")
	}
	// Regression guard: never reintroduce the CSP-blocked CDN URL.
	if strings.Contains(withTL, "jsdelivr") {
		t.Errorf("D3 must be self-hosted, not loaded from jsdelivr (CSP script-src 'self')")
	}
	noTL := sampleW2Active()
	noTL.Timelines = nil
	if strings.Contains(renderDashboardPage(t, noTL), d3Src) {
		t.Errorf("page should not load D3 when there are no timeline previews")
	}
}

// THE ROUTE'S PAGE. W2 made selection a full navigation on purpose: list rows
// are plain hrefs, so the engine/embed scripts a target page loads actually
// execute (htmx.config.allowScriptTags is false, boot.js:163) and teardown is
// the page unload. The Bench keeps that choice — every calendar door on it is a
// plain href, and the ONLY hx-get on the page is the sort control, which swaps
// the row section and nothing else.
func TestCalendarAppDashboard_RowsAreFullNav(t *testing.T) {
	html := renderBench(t, benchFxData(true, true))
	if strings.Contains(html, `hx-target="#cal-dash-detail"`) {
		t.Errorf("bench doors must not HTMX-swap a detail pane (full-nav for engine scripts)")
	}
	if !strings.Contains(html, `href="/campaigns/camp-1/calendar/v2/cal-elven"`) {
		t.Errorf("subordinate rows should be plain navigation links")
	}
	// The sort control's five links are the ONLY HTMX the Bench itself emits,
	// and every one of them swaps the row section. (The app shell's own
	// `hx-target="#main-content"` nav is not the Bench's and is not counted.)
	if n := strings.Count(html, `hx-get="/campaigns/camp-1/apps/calendar`); n != 5 {
		t.Errorf("the Bench should emit exactly the five sort hx-gets; got %d", n)
	}
	if n := strings.Count(html, `hx-target="#cal-dash-grid"`); n != 5 {
		t.Errorf("every Bench hx-get must target the row section; got %d", n)
	}
}
