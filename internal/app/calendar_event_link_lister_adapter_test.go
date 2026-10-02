// calendar_event_link_lister_adapter_test.go pins the fix for a real leak:
// calendarEventLinkListerAdapter.CalendarName used to read the calendar as a
// trusted SYSTEM caller with no visibility check at all, so a dm_only
// calendar's real name reached a Player/anonymous timeline viewer through
// Timeline.CalendarName (a display-only field). It must now behave exactly
// like every other calendar read gated by role: hidden calendar -> "".
package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// visibilityAwareCalendarService is a CalendarService test double that
// actually enforces cal.Visibility against the viewer's role — unlike
// fakeCalendarService in export_calendar_roundtrip_test.go (which returns
// its fixture unconditionally, fine for that file's system-only export
// path, but not sufficient to exercise this adapter's role gate).
type visibilityAwareCalendarService struct {
	calendar.CalendarService
	cal *calendar.Calendar
}

func (f *visibilityAwareCalendarService) GetCalendarForViewer(_ context.Context, calendarID, campaignID string, v permissions.Viewer) (*calendar.Calendar, error) {
	if f.cal == nil || f.cal.ID != calendarID || f.cal.CampaignID != campaignID {
		return nil, apperror.NewNotFound("calendar not found")
	}
	if f.cal.Visibility == "dm_only" && !v.SkipsPerUserRules() {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return f.cal, nil
}

func (f *visibilityAwareCalendarService) GetCalendarNameForViewer(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (string, error) {
	cal, err := f.GetCalendarForViewer(ctx, calendarID, campaignID, v)
	if err != nil {
		return "", err
	}
	return cal.Name, nil
}

func TestCalendarEventLinkListerAdapter_CalendarName_DmOnlyCalendarHiddenFromPlayer(t *testing.T) {
	svc := &visibilityAwareCalendarService{cal: &calendar.Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Inner Circle Calendar", Visibility: "dm_only",
	}}
	a := &calendarEventLinkListerAdapter{svc: svc}

	for _, tc := range []struct {
		name string
		role int
		want string
	}{
		{"anonymous/public visitor", int(permissions.RoleNone), ""},
		{"player", permissions.RolePlayer, ""},
		{"scribe", permissions.RoleScribe, ""},
		{"owner sees the real name", permissions.RoleOwner, "Inner Circle Calendar"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := a.CalendarName(context.Background(), "camp-1", "cal-1", tc.role)
			if got != tc.want {
				t.Errorf("CalendarName(role=%d) = %q, want %q", tc.role, got, tc.want)
			}
		})
	}
}

func TestCalendarEventLinkListerAdapter_CalendarName_EveryoneCalendarVisibleToAll(t *testing.T) {
	svc := &visibilityAwareCalendarService{cal: &calendar.Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Public Calendar", Visibility: "everyone",
	}}
	a := &calendarEventLinkListerAdapter{svc: svc}

	for _, role := range []int{int(permissions.RoleNone), permissions.RolePlayer, permissions.RoleOwner} {
		if got := a.CalendarName(context.Background(), "camp-1", "cal-1", role); got != "Public Calendar" {
			t.Errorf("CalendarName(role=%d) = %q, want the real name for an everyone-visible calendar", role, got)
		}
	}
}

func TestCalendarEventLinkListerAdapter_CalendarName_EmptyCalendarIDIsBlank(t *testing.T) {
	a := &calendarEventLinkListerAdapter{svc: &visibilityAwareCalendarService{}}
	if got := a.CalendarName(context.Background(), "camp-1", "", permissions.RoleOwner); got != "" {
		t.Errorf("CalendarName with no calendarID = %q, want \"\"", got)
	}
}

// batchRecordingCalendarService counts how the adapter reaches the calendar
// service, so a regression to one call per event id is caught.
type batchRecordingCalendarService struct {
	calendar.CalendarService
	batchCalls  int
	singleCalls int
	nameCalls   int
	fullCalls   int
	events      []calendar.Event
}

func (f *batchRecordingCalendarService) ListEventsByIDsForViewer(_ context.Context, _, _ string, _ []string, _ permissions.Viewer) ([]calendar.Event, error) {
	f.batchCalls++
	return f.events, nil
}
func (f *batchRecordingCalendarService) GetEventForViewer(context.Context, string, string, string, permissions.Viewer) (*calendar.Event, error) {
	f.singleCalls++
	return nil, apperror.NewNotFound("event not found")
}
func (f *batchRecordingCalendarService) GetCalendarNameForViewer(context.Context, string, string, permissions.Viewer) (string, error) {
	f.nameCalls++
	return "Name", nil
}
func (f *batchRecordingCalendarService) GetCalendarForViewer(context.Context, string, string, permissions.Viewer) (*calendar.Calendar, error) {
	f.fullCalls++
	return nil, apperror.NewNotFound("calendar not found")
}

func TestCalendarEventLinkListerAdapter_EventsByIDs_OneBatchCallAndMapsFields(t *testing.T) {
	kind := "festival"
	svc := &batchRecordingCalendarService{events: []calendar.Event{
		{ID: "e1", Name: "One", KindSlug: kind, Year: 5, Month: 2, Day: 3, Visibility: "everyone"},
		{ID: "e2", Name: "Two", Visibility: "dm_only"},
	}}
	a := &calendarEventLinkListerAdapter{svc: svc}

	ids := make([]string, 200)
	for i := range ids {
		ids[i] = "id"
	}
	refs, err := a.EventsByIDs(context.Background(), "cal-1", "camp-1", ids, permissions.RoleOwner)
	if err != nil {
		t.Fatalf("EventsByIDs: %v", err)
	}
	if svc.batchCalls != 1 || svc.singleCalls != 0 {
		t.Errorf("batch=%d single=%d, want 1 batch call and no per-event calls", svc.batchCalls, svc.singleCalls)
	}
	if len(refs) != 2 || refs[0].Category == nil || *refs[0].Category != kind || refs[1].Category != nil {
		t.Errorf("refs = %+v, want two refs with the kind slug mapped to Category only for e1", refs)
	}

	if _, err := a.EventsByIDs(context.Background(), "cal-1", "camp-1", nil, permissions.RoleOwner); err != nil || svc.batchCalls != 1 {
		t.Errorf("empty ids must not query: err=%v batchCalls=%d", err, svc.batchCalls)
	}
}

func TestCalendarEventLinkListerAdapter_CalendarName_UsesNameOnlyRead(t *testing.T) {
	svc := &batchRecordingCalendarService{}
	a := &calendarEventLinkListerAdapter{svc: svc}
	if got := a.CalendarName(context.Background(), "camp-1", "cal-1", permissions.RolePlayer); got != "Name" {
		t.Errorf("CalendarName = %q, want Name", got)
	}
	if svc.nameCalls != 1 || svc.fullCalls != 0 {
		t.Errorf("name=%d full=%d, want the name-only read and no eager-loading read", svc.nameCalls, svc.fullCalls)
	}
}
