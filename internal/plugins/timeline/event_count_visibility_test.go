// event_count_visibility_test.go pins that a timeline's EventCount must
// match what the viewer can actually open via ListTimelineEvents, not the
// SQL subquery's role-only count. The SQL count applies the dm_only
// predicate but cannot see per-user visibility_rules (allowed_users/
// denied_users), so a Player excluded from an event only by rules used to
// see a count one higher than their event list — a small existence oracle.
package timeline

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// TestListTimelines_EventCountMatchesFilteredEvents is the triage note's
// test plan: a Player denied one of two standalone events must see
// EventCount == 1, matching ListTimelineEvents; an Owner keeps the cheap
// SQL count unchanged (SkipsPerUserRules).
func TestListTimelines_EventCountMatchesFilteredEvents(t *testing.T) {
	const (
		campaignID = "camp-1"
		timelineID = "tl-1"
		playerID   = "u-player"
	)

	standalone := []TimelineEvent{
		{ID: "ev-open", TimelineID: timelineID, Name: "Open Event", Visibility: "everyone", Year: 1},
		{
			ID: "ev-hidden", TimelineID: timelineID, Name: "Hidden From Player",
			Visibility: "everyone", Year: 2,
			VisibilityRules: strPtr(`{"denied_users":["u-player"]}`),
		},
	}

	repo := &mockTimelineRepo{
		listFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
			// SQL's own count: both events are 'everyone', so the SQL-computed
			// EventCount is 2 regardless of role — that's the value under test.
			return []Timeline{{ID: timelineID, CampaignID: campaignID, Name: "Test Timeline", Visibility: "everyone", EventCount: 2}}, nil
		},
		listEventLinksFn: func(_ context.Context, _ string, _ int) ([]EventLink, error) {
			return nil, nil
		},
		listStandaloneEventsFn: func(_ context.Context, _ string, _ int) ([]TimelineEvent, error) {
			return append([]TimelineEvent(nil), standalone...), nil
		},
	}
	svc := newTestTimelineService(repo)
	ctx := context.Background()

	t.Run("player sees EventCount matching what they can open", func(t *testing.T) {
		player := permissions.RequestViewer(permissions.RolePlayer, playerID)

		tls, err := svc.ListTimelines(ctx, campaignID, player)
		if err != nil {
			t.Fatalf("ListTimelines: %v", err)
		}
		if len(tls) != 1 {
			t.Fatalf("expected exactly 1 timeline, got %d", len(tls))
		}

		rows, err := svc.ListTimelineEvents(ctx, timelineID, campaignID, player)
		if err != nil {
			t.Fatalf("ListTimelineEvents: %v", err)
		}

		if len(rows) != 1 {
			t.Fatalf("fixture drift: ListTimelineEvents returned %d rows, want 1", len(rows))
		}
		if tls[0].EventCount != len(rows) {
			t.Errorf("EventCount = %d, but the player can only open %d event(s) — the count leaks a hidden event's existence", tls[0].EventCount, len(rows))
		}
		if tls[0].EventCount != 1 {
			t.Errorf("EventCount = %d, want 1", tls[0].EventCount)
		}
	})

	t.Run("owner keeps the cheap SQL count unchanged", func(t *testing.T) {
		owner := permissions.RequestViewer(permissions.RoleOwner, "u-owner")

		tls, err := svc.ListTimelines(ctx, campaignID, owner)
		if err != nil {
			t.Fatalf("ListTimelines: %v", err)
		}
		if len(tls) != 1 {
			t.Fatalf("expected exactly 1 timeline, got %d", len(tls))
		}
		if tls[0].EventCount != 2 {
			t.Errorf("owner EventCount = %d, want 2 (unchanged SQL count)", tls[0].EventCount)
		}
	})
}

// TestListTimelinesForCalendar_EventCountMatchesFilteredEvents is the same
// claim through the calendar-facing list method, which carries the
// identical SQL-count gap (repository.go's ListByCalendar).
func TestListTimelinesForCalendar_EventCountMatchesFilteredEvents(t *testing.T) {
	const (
		calendarID = "cal-1"
		timelineID = "tl-1"
		playerID   = "u-player"
	)

	standalone := []TimelineEvent{
		{ID: "ev-open", TimelineID: timelineID, Name: "Open Event", Visibility: "everyone", Year: 1},
		{
			ID: "ev-hidden", TimelineID: timelineID, Name: "Hidden From Player",
			Visibility: "everyone", Year: 2,
			VisibilityRules: strPtr(`{"denied_users":["u-player"]}`),
		},
	}

	repo := &mockTimelineRepo{
		listByCalendarFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
			cal := calendarID
			return []Timeline{{ID: timelineID, CalendarID: &cal, Name: "Test Timeline", Visibility: "everyone", EventCount: 2}}, nil
		},
		listEventLinksFn: func(_ context.Context, _ string, _ int) ([]EventLink, error) {
			return nil, nil
		},
		listStandaloneEventsFn: func(_ context.Context, _ string, _ int) ([]TimelineEvent, error) {
			return append([]TimelineEvent(nil), standalone...), nil
		},
	}
	svc := newTestTimelineService(repo)
	ctx := context.Background()

	player := permissions.RequestViewer(permissions.RolePlayer, playerID)
	tls, err := svc.ListTimelinesForCalendar(ctx, calendarID, player)
	if err != nil {
		t.Fatalf("ListTimelinesForCalendar: %v", err)
	}
	if len(tls) != 1 {
		t.Fatalf("expected exactly 1 timeline, got %d", len(tls))
	}
	if tls[0].EventCount != 1 {
		t.Errorf("EventCount = %d, want 1 (matching the player's filtered event list)", tls[0].EventCount)
	}
}
