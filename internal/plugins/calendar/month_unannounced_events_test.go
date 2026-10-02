// month_unannounced_events_test.go: pins that the month grid and the
// single-event read apply the same "not yet announced" rule as the Upcoming
// Events card, so a player cannot read a secret future event from either.
package calendar

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// monthUnannouncedFixture serves one calendar (today = year 1000, month 1,
// day 10) whose month and single-event reads both return the same events,
// so each test only varies the viewer.
func monthUnannouncedFixture(t *testing.T) (CalendarService, []Event) {
	t.Helper()
	cal := &Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Mode: ModeFantasy,
		Name: "Month Test Calendar", Visibility: "everyone",
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
	// All in month 1 so they all reach the month grid; ahead-by-default comes
	// from having no kind (the default announcement is "ahead").
	events := []Event{
		{ID: "evt-secret-future", CalendarID: "cal-1", Name: "Secret Future Battle",
			Year: 1000, Month: 1, Day: 20, Visibility: "everyone", KindID: intPtr(1)},
		{ID: "evt-today", CalendarID: "cal-1", Name: "Today's Battle",
			Year: 1000, Month: 1, Day: 10, Visibility: "everyone", KindID: intPtr(1)},
		{ID: "evt-ahead-future", CalendarID: "cal-1", Name: "Announced Festival",
			Year: 1000, Month: 1, Day: 15, Visibility: "everyone", Announced: &overrideAhead},
		{ID: "evt-override-future", CalendarID: "cal-1", Name: "Announced Battle",
			Year: 1000, Month: 1, Day: 25, Visibility: "everyone", KindID: intPtr(1),
			Announced: &overrideAhead},
	}

	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id != cal.ID {
				return nil, apperror.NewNotFound("calendar not found")
			}
			c := *cal
			return &c, nil
		},
		getMonthsFn:   func(_ context.Context, _ string) ([]Month, error) { return months, nil },
		getWeekdaysFn: func(_ context.Context, _ string) ([]Weekday, error) { return nil, nil },
	}
	eventRepo := &fakeEventRepo{
		listForMonthFn: func(_ context.Context, _ string, _, _, _ int) ([]Event, error) {
			// Copy: the service filters in place and would corrupt shared state.
			return append([]Event(nil), events...), nil
		},
		getEventFn: func(_ context.Context, id string) (*Event, error) {
			for _, e := range events {
				if e.ID == id {
					e := e
					return &e, nil
				}
			}
			return nil, nil
		},
	}
	kindRepo := &fakeEventKindRepo{
		listFn: func(_ context.Context, _ string) ([]EventKind, error) { return kinds, nil },
	}
	return newTestCalendarService(calRepo, eventRepo, kindRepo, nil), events
}

func TestListEventsForMonth_UnannouncedFutureEvents(t *testing.T) {
	tests := []struct {
		name   string
		viewer permissions.Viewer
		want   map[string]bool
	}{
		{"player", playerViewer("player-1"), map[string]bool{
			"Today's Battle": true, "Announced Festival": true, "Announced Battle": true,
		}},
		{"anonymous", publicViewer(), map[string]bool{
			"Today's Battle": true, "Announced Festival": true, "Announced Battle": true,
		}},
		{"owner", ownerViewer("owner-1"), map[string]bool{
			"Secret Future Battle": true, "Today's Battle": true,
			"Announced Festival": true, "Announced Battle": true,
		}},
		{"co-DM", coDMViewer("codm-1"), map[string]bool{
			"Secret Future Battle": true, "Today's Battle": true,
			"Announced Festival": true, "Announced Battle": true,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := monthUnannouncedFixture(t)
			events, err := svc.ListEventsForMonth(context.Background(), "cal-1", testCampaignA, 1000, 1, tc.viewer)
			if err != nil {
				t.Fatalf("ListEventsForMonth: %v", err)
			}
			got := eventNames(events)
			if len(got) != len(tc.want) {
				t.Errorf("got events %v, want %v", got, tc.want)
			}
			for name := range tc.want {
				if !got[name] {
					t.Errorf("missing %q; got %v", name, got)
				}
			}
		})
	}
}

func TestGetEventForViewer_UnannouncedFutureEvent(t *testing.T) {
	tests := []struct {
		name     string
		viewer   permissions.Viewer
		eventID  string
		wantCode int // 0 = success
	}{
		{"player, future on_day event", playerViewer("player-1"), "evt-secret-future", http.StatusNotFound},
		{"anonymous, future on_day event", publicViewer(), "evt-secret-future", http.StatusNotFound},
		{"owner, future on_day event", ownerViewer("owner-1"), "evt-secret-future", 0},
		{"player, today's on_day event", playerViewer("player-1"), "evt-today", 0},
		{"player, ahead-announced future event", playerViewer("player-1"), "evt-ahead-future", 0},
		{"player, override-to-ahead future event", playerViewer("player-1"), "evt-override-future", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := monthUnannouncedFixture(t)
			evt, err := svc.GetEventForViewer(context.Background(), tc.eventID, "cal-1", testCampaignA, tc.viewer)
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("GetEventForViewer: %v", err)
				}
				if evt == nil || evt.ID != tc.eventID {
					t.Errorf("got %+v, want event %s", evt, tc.eventID)
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tc.wantCode {
				t.Errorf("want AppError %d, got %v", tc.wantCode, err)
			}
		})
	}
}
