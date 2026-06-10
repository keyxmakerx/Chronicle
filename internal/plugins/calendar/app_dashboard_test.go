// app_dashboard_test.go — C-APPS-CAL-DASH-W1. Covers the calendar-side
// EntitiesForCalendar read (service passthrough) and the dashboard view:
// list + detail render, CRUD-compose actions, owner-gating, and the
// read-only associations panel with/without associations + empty states.
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

func TestCalendarAppDashboardPage_ListAndDetail(t *testing.T) {
	html := renderDashboardPage(t, sampleDashboardData())
	for _, want := range []string{
		"data-cal-dashboard",                    // page container
		"data-cal-dashboard-list",               // the list column
		`data-calendar-id="cal-1"`,              // list row + detail
		"Harptos", "Gregorian",                  // both calendars in the list
		"id=\"cal-dash-detail\"",                // detail swap target
		"/campaigns/camp-1/apps/calendar?calId=cal-2", // list selection link
		// CRUD-compose actions (existing surfaces, not reimplemented):
		"/campaigns/camp-1/calendars/cal-1",          // Open calendar
		"/campaigns/camp-1/calendars/cal-1/settings", // Settings (edit)
		// associations:
		"Linked entities (1)", "Gandalf",
		"Timelines (1)", "Main Arc", "7 events",
		"/campaigns/camp-1/entities/e1",  // entity link
		"/campaigns/camp-1/timelines/t1", // timeline link
	} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard page missing %q", want)
		}
	}
}

// When the selected calendar isn't the user's active one, the detail offers
// the "Set active" compose action (POST to the existing switch endpoint).
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

func TestCalendarAppDashboard_EmptyStates(t *testing.T) {
	// No calendars at all → friendly create CTA, no detail pane.
	empty := renderDashboardPage(t, CalendarAppDashboardData{CampaignID: "camp-1", IsOwner: true})
	for _, want := range []string{"data-cal-dashboard-empty", "No calendars yet", "Create calendar", "/campaigns/camp-1/calendars"} {
		if !strings.Contains(empty, want) {
			t.Errorf("no-calendars state missing %q", want)
		}
	}
	if strings.Contains(empty, "id=\"cal-dash-detail\"") {
		t.Errorf("no-calendars state must not render the detail pane")
	}

	// Calendars exist but none selected → detail prompts a selection.
	noSel := renderDashboardDetail(t, CalendarAppDashboardData{CampaignID: "camp-1"})
	if !strings.Contains(noSel, "data-cal-dashboard-detail-empty") || !strings.Contains(noSel, "Select a calendar") {
		t.Errorf("no-selection detail missing the prompt")
	}
}

func TestCalendarAppDashboard_LoadError(t *testing.T) {
	html := renderDashboardPage(t, CalendarAppDashboardData{CampaignID: "camp-1", LoadError: true})
	if !strings.Contains(html, "data-cal-dashboard-error") || !strings.Contains(html, "load the calendars") {
		t.Errorf("load-error state missing friendly message")
	}
}

// --- W2: live "see in action" embeds (C-APPS-CAL-DASH-W2) ---

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

// The page loads D3 (for timeline-viz) only when the selection has timelines.
// W1 (R1): D3 is the VENDORED copy (`/static/vendor/d3.min.js`), not the
// jsdelivr CDN — `script-src 'self'` blocked the CDN script so the viz never ran.
func TestCalendarAppDashboard_LoadsD3ForTimelines(t *testing.T) {
	const d3Src = "/static/vendor/d3.min.js"
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

// W2 selection is a full navigation: list rows are plain hrefs (no HTMX detail
// swap), so the embed/engine scripts execute on load and teardown is the page
// unload (one live surface, clean teardown).
func TestCalendarAppDashboard_RowsAreFullNav(t *testing.T) {
	html := renderDashboardPage(t, sampleDashboardData())
	if strings.Contains(html, `hx-target="#cal-dash-detail"`) {
		t.Errorf("W2 list rows must not HTMX-swap the detail (full-nav for engine scripts)")
	}
	if !strings.Contains(html, `href="/campaigns/camp-1/apps/calendar?calId=cal-2"`) {
		t.Errorf("list rows should be plain navigation links")
	}
}
