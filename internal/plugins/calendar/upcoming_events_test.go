// upcoming_events_test.go: table-driven tests for ListUpcomingEvents's
// viewer-filtering, the "Upcoming Events" dashboard card's data source.
// Pins the fix for a real leak: the card listed every future event
// regardless of whether it had been announced yet, unlike the older
// month-JSON endpoint and this PR's own aiexport Safe-mode renderer, which
// both already treat a not-yet-announced future event as secret.
package calendar

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// upcomingEventsFixture builds a calendar service whose default/only
// calendar is "everyone", currently on year 1000, month 1, day 10, with a
// "battle" kind defaulting to on_day announcement. The event list mixes:
//   - an on_day event later THIS month (day 20): not yet announced, future
//   - an on_day event TODAY (day 10): not yet announced, but not future
//   - an ahead-announced event next month: future, but common knowledge
//   - an event whose kind is on_day but Announced is explicitly overridden
//     to "ahead": future, but the override makes it common knowledge
//   - a dm_only event later this month: excluded by filterEventsByUser
//     before dropUnannouncedFutureEvents even runs
func upcomingEventsFixture(t *testing.T) CalendarService {
	t.Helper()
	cal := &Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Mode: ModeFantasy,
		Name: "Upcoming Test Calendar", Visibility: "everyone",
		CurrentYear: 1000, CurrentMonth: 1, CurrentDay: 10,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	months := []Month{
		{Name: "First", Days: 30, SortOrder: 0},
		{Name: "Second", Days: 30, SortOrder: 1},
	}
	kinds := []EventKind{
		{ID: 1, Slug: "battle", Name: "Battle", DefaultAnnounced: AnnouncedOnDay},
	}
	overrideAhead := AnnouncedAhead
	events := []Event{
		{ID: "evt-secret-future", CalendarID: "cal-1", Name: "Secret Future Battle",
			Year: 1000, Month: 1, Day: 20, Visibility: "everyone", KindID: intPtr(1)},
		{ID: "evt-today", CalendarID: "cal-1", Name: "Today's Battle",
			Year: 1000, Month: 1, Day: 10, Visibility: "everyone", KindID: intPtr(1)},
		{ID: "evt-ahead-future", CalendarID: "cal-1", Name: "Yearly Festival",
			Year: 1000, Month: 2, Day: 1, Visibility: "everyone",
			IsRecurring: true, RecurrenceType: strPtr(RecurrenceYearly)},
		{ID: "evt-override-future", CalendarID: "cal-1", Name: "Announced Battle",
			Year: 1000, Month: 1, Day: 25, Visibility: "everyone", KindID: intPtr(1),
			Announced: &overrideAhead},
		{ID: "evt-dm-only-future", CalendarID: "cal-1", Name: "Secret War Council",
			Year: 1000, Month: 1, Day: 22, Visibility: "dm_only"},
	}

	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id != cal.ID {
				return nil, apperror.NewNotFound("calendar not found")
			}
			return cal, nil
		},
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) {
			return months, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listUpcomingFn: func(_ context.Context, _ string, _, _, _, _, _ int) ([]Event, error) {
			return events, nil
		},
	}
	kindRepo := &fakeEventKindRepo{
		listFn: func(_ context.Context, _ string) ([]EventKind, error) {
			return kinds, nil
		},
	}
	return newTestCalendarService(calRepo, eventRepo, kindRepo, nil)
}

func eventNames(events []Event) map[string]bool {
	out := make(map[string]bool, len(events))
	for _, e := range events {
		out[e.Name] = true
	}
	return out
}

func TestListUpcomingEvents_UnannouncedFutureEventsHiddenFromPlayers(t *testing.T) {
	svc := upcomingEventsFixture(t)

	for _, tc := range []struct {
		name string
		v    permissions.Viewer
	}{
		{"player", playerViewer("player-1")},
		{"anonymous", publicViewer()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := svc.ListUpcomingEvents(context.Background(), "cal-1", testCampaignA, 10, tc.v)
			if err != nil {
				t.Fatalf("ListUpcomingEvents: %v", err)
			}
			names := eventNames(events)
			if names["Secret Future Battle"] {
				t.Error("a not-yet-announced future event must not appear in a Player/anonymous Upcoming Events list")
			}
			if names["Secret War Council"] {
				t.Error("a dm_only event must not appear in a Player/anonymous Upcoming Events list")
			}
			if !names["Today's Battle"] {
				t.Error("an on_day event dated TODAY must still appear — it is knowable as of today")
			}
			if !names["Yearly Festival"] {
				t.Error("an ahead-announced (yearly recurring) future event must appear — it's common knowledge")
			}
			if !names["Announced Battle"] {
				t.Error("a future event with an explicit ahead override must appear")
			}
		})
	}
}

func TestListUpcomingEvents_OwnerAndCoDMSeeUnannouncedFutureEvents(t *testing.T) {
	svc := upcomingEventsFixture(t)

	for _, tc := range []struct {
		name string
		v    permissions.Viewer
	}{
		{"owner", ownerViewer("owner-1")},
		{"co-DM", coDMViewer("codm-1")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := svc.ListUpcomingEvents(context.Background(), "cal-1", testCampaignA, 10, tc.v)
			if err != nil {
				t.Fatalf("ListUpcomingEvents: %v", err)
			}
			names := eventNames(events)
			if !names["Secret Future Battle"] {
				t.Error("an Owner/co-DM must see a not-yet-announced future event — they authored it")
			}
			if !names["Secret War Council"] {
				t.Error("an Owner/co-DM must see the dm_only event")
			}
		})
	}
}

// TestListUpcomingEvents_HiddenCalendarIsNotFound proves a dm_only calendar
// stays unreachable through this endpoint too — the calendar-level gate
// (calendarInCampaignForViewer) runs before any event is read.
func TestListUpcomingEvents_HiddenCalendarIsNotFound(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-hidden", CampaignID: testCampaignA, Visibility: "dm_only"}, nil
		},
	}
	svc := newTestCalendarService(calRepo, &fakeEventRepo{}, &fakeEventKindRepo{}, nil)

	_, err := svc.ListUpcomingEvents(context.Background(), "cal-hidden", testCampaignA, 10, playerViewer("player-1"))
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
		t.Errorf("a dm_only calendar's upcoming events must read as NotFound to a Player, got: %v", err)
	}
}
