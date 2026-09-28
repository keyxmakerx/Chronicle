// calendar_timeline_adapters_test.go pins that calendarListerAdapter,
// calendarEventListerAdapter and calendarEraListerAdapter forward
// campaignID/calendarID/role to CalendarService unchanged, and map each
// calendar.* field onto its timeline.* counterpart correctly. The role and
// campaign gating itself is CalendarService's job, pinned in
// internal/plugins/calendar/events_eras_for_calendar_test.go.
package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// --- calendarListerAdapter ---

// spyCalendarListerSvc records what ListCalendars was called with; the
// route (timeline/routes.go) is Owner-gated and this interface carries no
// per-request identity, so the adapter must always read as Owner.
type spyCalendarListerSvc struct {
	calendar.CalendarService
	gotCampaignID string
	gotViewer     permissions.Viewer
	cals          []calendar.Calendar
}

func (f *spyCalendarListerSvc) ListCalendars(_ context.Context, campaignID string, v permissions.Viewer) ([]calendar.Calendar, error) {
	f.gotCampaignID = campaignID
	f.gotViewer = v
	return f.cals, nil
}

func TestCalendarListerAdapter_ListCalendars(t *testing.T) {
	svc := &spyCalendarListerSvc{cals: []calendar.Calendar{{ID: "cal-1", Name: "Adventure Calendar"}}}
	a := &calendarListerAdapter{svc: svc}

	refs, err := a.ListCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("ListCalendars: %v", err)
	}
	if svc.gotCampaignID != "camp-1" {
		t.Errorf("campaignID forwarded = %q, want camp-1", svc.gotCampaignID)
	}
	if svc.gotViewer.Role() != permissions.RoleOwner || svc.gotViewer.UserID() != "" {
		t.Errorf("viewer = role %d userID %q, want Owner with no user id", svc.gotViewer.Role(), svc.gotViewer.UserID())
	}
	if len(refs) != 1 || refs[0].ID != "cal-1" || refs[0].Name != "Adventure Calendar" {
		t.Errorf("field mapping wrong: %+v", refs)
	}
}

// --- calendarEventListerAdapter ---

type spyEventListerSvc struct {
	calendar.CalendarService
	gotCampaignID string
	gotCalendarID string
	gotRole       int
	events        []calendar.Event
}

func (f *spyEventListerSvc) ListEventsForCalendar(_ context.Context, campaignID, calendarID string, role int) ([]calendar.Event, error) {
	f.gotCampaignID, f.gotCalendarID, f.gotRole = campaignID, calendarID, role
	return f.events, nil
}

func TestCalendarEventListerAdapter_ForwardsArgsAndMapsFields(t *testing.T) {
	entityID := "ent-1"
	desc := "A festival"
	endYear := 101
	svc := &spyEventListerSvc{events: []calendar.Event{{
		ID: "evt-1", Name: "Harvest Fair", Description: &desc,
		Year: 100, Month: 2, Day: 3, EndYear: &endYear,
		Visibility: "everyone",
		EntityID:   &entityID, EntityName: "Bob", EntityIcon: "fa-user",
		KindSlug: "festival",
	}}}
	a := &calendarEventListerAdapter{svc: svc}

	refs, err := a.ListEventsForCalendar(context.Background(), "camp-1", "cal-1", permissions.RoleScribe)
	if err != nil {
		t.Fatalf("ListEventsForCalendar: %v", err)
	}
	if svc.gotCampaignID != "camp-1" || svc.gotCalendarID != "cal-1" || svc.gotRole != permissions.RoleScribe {
		t.Errorf("forwarded (campaignID=%q, calendarID=%q, role=%d), want (camp-1, cal-1, %d)",
			svc.gotCampaignID, svc.gotCalendarID, svc.gotRole, permissions.RoleScribe)
	}
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1", len(refs))
	}
	r := refs[0]
	if r.ID != "evt-1" || r.Name != "Harvest Fair" || r.Year != 100 || r.Month != 2 || r.Day != 3 {
		t.Errorf("core fields not mapped: %+v", r)
	}
	if r.Description == nil || *r.Description != "A festival" {
		t.Errorf("Description not mapped: %v", r.Description)
	}
	if r.EndYear == nil || *r.EndYear != 101 {
		t.Errorf("EndYear not mapped: %v", r.EndYear)
	}
	if r.Category == nil || *r.Category != "festival" {
		t.Errorf("Category (from KindSlug) = %v, want festival", r.Category)
	}
	if r.EntityID == nil || *r.EntityID != "ent-1" || r.EntityName != "Bob" || r.EntityIcon != "fa-user" {
		t.Errorf("entity link fields not mapped: %+v", r)
	}
}

func TestCalendarEventListerAdapter_NoKindLeavesCategoryNil(t *testing.T) {
	svc := &spyEventListerSvc{events: []calendar.Event{{ID: "evt-1", Name: "Plain Event", Visibility: "everyone"}}}
	a := &calendarEventListerAdapter{svc: svc}

	refs, err := a.ListEventsForCalendar(context.Background(), "camp-1", "cal-1", permissions.RoleOwner)
	if err != nil {
		t.Fatalf("ListEventsForCalendar: %v", err)
	}
	if len(refs) != 1 || refs[0].Category != nil {
		t.Errorf("Category = %v, want nil for an event with no kind slug", refs[0].Category)
	}
}

// --- calendarEraListerAdapter ---

type spyEraListerSvc struct {
	calendar.CalendarService
	gotCampaignID string
	gotCalendarID string
	gotRole       int
	eras          []calendar.Era
}

func (f *spyEraListerSvc) ListErasForCalendar(_ context.Context, campaignID, calendarID string, role int) ([]calendar.Era, error) {
	f.gotCampaignID, f.gotCalendarID, f.gotRole = campaignID, calendarID, role
	return f.eras, nil
}

func TestCalendarEraListerAdapter_ForwardsArgsAndMapsFields(t *testing.T) {
	end := 500
	svc := &spyEraListerSvc{eras: []calendar.Era{{Name: "Age of Heroes", StartYear: 0, EndYear: &end, Color: "#888"}}}
	a := &calendarEraListerAdapter{svc: svc}

	refs, err := a.ListEras(context.Background(), "camp-1", "cal-1", permissions.RoleOwner)
	if err != nil {
		t.Fatalf("ListEras: %v", err)
	}
	if svc.gotCampaignID != "camp-1" || svc.gotCalendarID != "cal-1" || svc.gotRole != permissions.RoleOwner {
		t.Errorf("forwarded (campaignID=%q, calendarID=%q, role=%d), want (camp-1, cal-1, %d)",
			svc.gotCampaignID, svc.gotCalendarID, svc.gotRole, permissions.RoleOwner)
	}
	if len(refs) != 1 || refs[0].Name != "Age of Heroes" || refs[0].EndYear == nil || *refs[0].EndYear != 500 || refs[0].Color != "#888" {
		t.Errorf("field mapping wrong: %+v", refs)
	}
}
