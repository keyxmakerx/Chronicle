// calendar_eras_role_test.go pins that the timeline service forwards
// campaignID and role unchanged to the injected CalendarEraLister and
// CalendarEventLister — the parameters calendarEraListerAdapter/
// calendarEventListerAdapter (internal/app/routes.go) need to scope a read
// to the timeline's own campaign and gate eras by role. Campaign scoping
// and role gating themselves are CalendarService's job, pinned in
// internal/plugins/calendar/events_eras_for_calendar_test.go.
package timeline

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestListCalendarEras_ForwardsCampaignIDAndRoleUnchanged(t *testing.T) {
	var gotCampaignID, gotCalendarID string
	var gotRole int
	eraLister := &mockCalendarEraLister{
		listFn: func(_ context.Context, campaignID, calendarID string, role int) ([]CalendarEra, error) {
			gotCampaignID, gotCalendarID, gotRole = campaignID, calendarID, role
			return []CalendarEra{{Name: "Age of Heroes"}}, nil
		},
	}
	svc := NewTimelineService(&mockTimelineRepo{}, &mockCalendarLister{}, &mockCalendarEventLister{}, eraLister)

	eras, err := svc.ListCalendarEras(context.Background(), "camp-1", "cal-1", int(permissions.RolePlayer))
	if err != nil {
		t.Fatalf("ListCalendarEras: %v", err)
	}
	if gotCampaignID != "camp-1" {
		t.Errorf("campaignID forwarded = %q, want camp-1", gotCampaignID)
	}
	if gotCalendarID != "cal-1" {
		t.Errorf("calendarID forwarded = %q, want cal-1", gotCalendarID)
	}
	if gotRole != int(permissions.RolePlayer) {
		t.Errorf("role forwarded = %d, want %d (RolePlayer) — the era gate depends on this reaching it unchanged", gotRole, int(permissions.RolePlayer))
	}
	if len(eras) != 1 {
		t.Errorf("got %d eras, want the lister's 1", len(eras))
	}
}

func TestListCalendarEras_NilListerIsSafe(t *testing.T) {
	svc := NewTimelineService(&mockTimelineRepo{}, nil, nil, nil)

	eras, err := svc.ListCalendarEras(context.Background(), "camp-1", "cal-1", int(permissions.RoleOwner))
	if err != nil {
		t.Fatalf("ListCalendarEras with no lister wired must not error, got %v", err)
	}
	if eras != nil {
		t.Errorf("got %v, want nil", eras)
	}
}

func TestListAvailableEvents_ForwardsCampaignIDAndRoleUnchanged(t *testing.T) {
	calID := "cal-1"
	repo := &mockTimelineRepo{
		getByIDFn: func(_ context.Context, id string) (*Timeline, error) {
			return &Timeline{ID: id, CampaignID: "camp-1", CalendarID: &calID}, nil
		},
	}
	var gotCampaignID, gotCalendarID string
	var gotRole int
	eventLister := &mockCalendarEventLister{
		listFn: func(_ context.Context, campaignID, calendarID string, role int) ([]CalendarEventRef, error) {
			gotCampaignID, gotCalendarID, gotRole = campaignID, calendarID, role
			return []CalendarEventRef{{ID: "evt-1", Name: "Harvest Fair"}}, nil
		},
	}
	svc := NewTimelineService(repo, &mockCalendarLister{}, eventLister, &mockCalendarEraLister{})

	available, err := svc.ListAvailableEvents(context.Background(), "tl-1", int(permissions.RoleScribe))
	if err != nil {
		t.Fatalf("ListAvailableEvents: %v", err)
	}
	if gotCampaignID != "camp-1" {
		t.Errorf("campaignID forwarded = %q, want camp-1 (the timeline's own, not a caller-supplied one)", gotCampaignID)
	}
	if gotCalendarID != calID {
		t.Errorf("calendarID forwarded = %q, want %q", gotCalendarID, calID)
	}
	if gotRole != int(permissions.RoleScribe) {
		t.Errorf("role forwarded = %d, want %d (RoleScribe)", gotRole, int(permissions.RoleScribe))
	}
	if len(available) != 1 || available[0].ID != "evt-1" {
		t.Errorf("got %+v, want the lister's one event", available)
	}
}
