// list_by_calendar_test.go — C-APPS-CAL-DASH-W1. The cross-plugin
// "timelines for this calendar" read the Calendars dashboard consumes:
// repo filter + the same role/visibility filtering as ListTimelines.
package timeline

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func TestListTimelinesForCalendar_FiltersByCalendar(t *testing.T) {
	var gotCalendarID string
	var gotRole int
	repo := &mockTimelineRepo{
		listByCalendarFn: func(_ context.Context, calendarID string, role int) ([]Timeline, error) {
			gotCalendarID = calendarID
			gotRole = role
			return []Timeline{
				{ID: "t1", Name: "Public", Visibility: "everyone"},
			}, nil
		},
	}
	svc := newTestTimelineService(repo)
	out, err := svc.ListTimelinesForCalendar(context.Background(), "cal-1", permissions.RoleOwner, "u1")
	if err != nil {
		t.Fatalf("ListTimelinesForCalendar: %v", err)
	}
	if gotCalendarID != "cal-1" {
		t.Errorf("repo got calendarID %q, want cal-1", gotCalendarID)
	}
	if gotRole != permissions.RoleOwner {
		t.Errorf("repo got role %d, want owner", gotRole)
	}
	if len(out) != 1 || out[0].ID != "t1" {
		t.Errorf("unexpected result: %+v", out)
	}
}

// A non-DM viewer must not see dm_only timelines (same per-user visibility
// filter as ListTimelines).
func TestListTimelinesForCalendar_HidesDmOnlyFromPlayers(t *testing.T) {
	repo := &mockTimelineRepo{
		listByCalendarFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
			return []Timeline{
				{ID: "pub", Name: "Public", Visibility: "everyone"},
				{ID: "secret", Name: "Secret", Visibility: "dm_only"},
			}, nil
		},
	}
	svc := newTestTimelineService(repo)

	player, err := svc.ListTimelinesForCalendar(context.Background(), "cal-1", permissions.RolePlayer, "u1")
	if err != nil {
		t.Fatalf("player list: %v", err)
	}
	for _, tl := range player {
		if tl.ID == "secret" {
			t.Errorf("player must not see the dm_only timeline")
		}
	}
	if len(player) != 1 {
		t.Errorf("player should see exactly the public timeline; got %d", len(player))
	}

	owner, err := svc.ListTimelinesForCalendar(context.Background(), "cal-1", permissions.RoleOwner, "u1")
	if err != nil {
		t.Fatalf("owner list: %v", err)
	}
	if len(owner) != 2 {
		t.Errorf("owner should see both timelines; got %d", len(owner))
	}
}
