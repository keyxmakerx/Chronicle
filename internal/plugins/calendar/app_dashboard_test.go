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

// The service delegates EntitiesForCalendar to the repo, forwarding the id.
func TestEntitiesForCalendar_ServiceDelegates(t *testing.T) {
	var gotID string
	repo := &mockCalendarRepo{
		entitiesForCalendarFn: func(_ context.Context, calendarID string) ([]EntityTieRef, error) {
			gotID = calendarID
			return []EntityTieRef{{EntityID: "e1", EntityName: "Gandalf"}}, nil
		},
	}
	svc := NewCalendarService(repo)
	out, err := svc.EntitiesForCalendar(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("EntitiesForCalendar: %v", err)
	}
	if gotID != "cal-1" {
		t.Errorf("repo got id %q, want cal-1", gotID)
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
