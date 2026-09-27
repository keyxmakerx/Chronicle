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
