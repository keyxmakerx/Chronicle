// extension_dashboard_test.go — C-EXT-HUB Phase 2 calendar plugin
// dashboard tests. Covers the data projection (without DB) + the
// templ render's three branches (no active cal, active with empty
// upcoming, active with events).

package calendar

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCalendarExtensionDashboard_NoActiveCalendarRendersEmpty(t *testing.T) {
	data := calendarExtensionDashboardData{
		CampaignID: "c-1",
		HasActive:  false,
	}
	var buf bytes.Buffer
	if err := calendarExtensionDashboard(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "No calendar configured") {
		t.Errorf("empty branch should render no-calendar copy; got:\n%s", html)
	}
	if !strings.Contains(html, "/campaigns/c-1/calendars") {
		t.Errorf("empty branch should link to calendar create flow; got:\n%s", html)
	}
	if strings.Contains(html, "Open V2 Calendar") {
		t.Errorf("empty branch should NOT show Open V2 Calendar CTA "+
			"(there's no V2 surface to open); got:\n%s", html)
	}
}

func TestCalendarExtensionDashboard_ActiveCalendarNoEvents(t *testing.T) {
	cal := &Calendar{
		ID:           "cal-1",
		Name:         "The Reckoning",
		CurrentYear:  1492,
		CurrentMonth: 3,
		CurrentDay:   14,
		Months: []Month{
			{Name: "Frostmoon", Days: 30},
			{Name: "Thawmoon", Days: 30},
			{Name: "Sunwane", Days: 31},
		},
	}
	data := calendarExtensionDashboardData{
		CampaignID: "c-1",
		Active:     cal,
		HasActive:  true,
	}
	var buf bytes.Buffer
	if err := calendarExtensionDashboard(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "The Reckoning") {
		t.Errorf("active branch should show calendar name; got:\n%s", html)
	}
	if !strings.Contains(html, "Sunwane 14, 1492") {
		t.Errorf("active branch should show current in-world date; got:\n%s", html)
	}
	if !strings.Contains(html, "No upcoming events") {
		t.Errorf("active branch with no events should show empty-events copy; got:\n%s", html)
	}
	// Operator's first V2 calendar door — must be present.
	if !strings.Contains(html, "/campaigns/c-1/calendar/v2") {
		t.Errorf("active branch must include the Open V2 Calendar link; got:\n%s", html)
	}
	if !strings.Contains(html, "Open V2 Calendar") {
		t.Errorf("CTA label missing; got:\n%s", html)
	}
}

// TestCalendarExtensionDashboard_SingleEventTierColored is the
// single-element-array standing-pattern fixture + the tier-rendering
// guard. Wave 1.6.5 plumbed campaign tier defs end-to-end; the
// dashboard chip uses the same TierDefinitionAlias slice the V2
// pages use.
func TestCalendarExtensionDashboard_SingleEventTierColored(t *testing.T) {
	cal := &Calendar{
		ID:           "cal-1",
		Name:         "Y2K",
		CurrentYear:  1999,
		CurrentMonth: 12,
		CurrentDay:   31,
		Months:       []Month{{Name: "December", Days: 31}},
	}
	tier := "major"
	data := calendarExtensionDashboardData{
		CampaignID: "c-1",
		Active:     cal,
		HasActive:  true,
		Upcoming: []Event{
			{
				ID:    "ev-1",
				Name:  "Coronation",
				Year:  2000,
				Month: 1,
				Day:   1,
				Tier:  &tier,
			},
		},
		TierDefinitions: []TierDefinitionAlias{
			{Slug: "major", Name: "Major", Color: "#facc15", Prominence: 80},
		},
	}
	var buf bytes.Buffer
	if err := calendarExtensionDashboard(data).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "Coronation") {
		t.Errorf("event name should render; got:\n%s", html)
	}
	if !strings.Contains(html, "background-color: #facc15") {
		t.Errorf("tier color should inline-style the chip; got:\n%s", html)
	}
}

func TestExtensionDashboardEventChipStyle_FallbacksDoNotBreakRender(t *testing.T) {
	ev := Event{ID: "ev-1", Name: "X", Year: 1, Month: 1, Day: 1}
	// No tier
	if got := extensionDashboardEventChipStyle(ev, nil); got != "" {
		t.Errorf("no tier should yield empty style, got %q", got)
	}
	// Empty-string tier
	empty := ""
	ev.Tier = &empty
	if got := extensionDashboardEventChipStyle(ev, nil); got != "" {
		t.Errorf("empty-tier should yield empty style, got %q", got)
	}
	// Tier slug not in defs → empty (defensive)
	slug := "unknown"
	ev.Tier = &slug
	defs := []TierDefinitionAlias{{Slug: "major", Color: "#abc"}}
	if got := extensionDashboardEventChipStyle(ev, defs); got != "" {
		t.Errorf("unknown tier slug should yield empty style, got %q", got)
	}
}

// TestExtensionDashboardFactory_ReturnsCalendarSlug pins the factory
// contract — campaigns plugin's wiring path relies on it.
func TestExtensionDashboardFactory_ReturnsCalendarSlug(t *testing.T) {
	h := &Handler{}
	factory := h.ExtensionDashboardFactory()
	if factory == nil {
		t.Fatalf("ExtensionDashboardFactory returned nil")
	}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c-1"}}
	got := factory(cc)
	if got.Slug != "calendar" {
		t.Errorf("slug=%q, want 'calendar'", got.Slug)
	}
	if got.Content == nil {
		t.Errorf("Content should never be nil — dispatcher falls back to missing on nil")
	}
}

// TestBuildCalendarExtensionDashboardData_NilHandlerSafe pins the
// audit §1.4 nil-safe stance: a handler without service wiring (e.g.
// during integration test setup) must not panic when the factory
// runs.
func TestBuildCalendarExtensionDashboardData_NilHandlerSafe(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c-1"}}
	got := buildCalendarExtensionDashboardData(context.Background(), nil, cc)
	if got.HasActive {
		t.Errorf("nil handler should yield HasActive=false; got %+v", got)
	}
	if got.CampaignID != "c-1" {
		t.Errorf("campaign id should still propagate; got %q", got.CampaignID)
	}
}

func TestFormatCalendarCurrentDate(t *testing.T) {
	cal := &Calendar{
		CurrentYear:  100,
		CurrentMonth: 2,
		CurrentDay:   15,
		Months: []Month{
			{Name: "Foo", Days: 30},
			{Name: "Bar", Days: 30},
		},
	}
	if got := formatCalendarCurrentDate(cal); got != "Bar 15, 100" {
		t.Errorf("formatCalendarCurrentDate = %q, want %q", got, "Bar 15, 100")
	}
	if got := formatCalendarCurrentDate(nil); got != "" {
		t.Errorf("nil cal should yield empty string, got %q", got)
	}
}
