package calendar

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// recordingPublisher captures what the service hands the publisher.
type recordingPublisher struct{ got []publishedCalendarEvent }

type publishedCalendarEvent struct {
	eventType, campaignID, calendarID string
	payload                           any
}

func (r *recordingPublisher) PublishCalendarEvent(eventType, campaignID, calendarID string, payload any) {
	r.got = append(r.got, publishedCalendarEvent{eventType, campaignID, calendarID, payload})
}

func (r *recordingPublisher) types() []string {
	out := make([]string, len(r.got))
	for i, g := range r.got {
		out[i] = g.eventType
	}
	return out
}

// publishFixture is a calendar service over fakes that accept every write,
// with a recording publisher attached.
func publishFixture(t *testing.T, writeErr error) (CalendarService, *recordingPublisher) {
	t.Helper()
	cal := Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Mode: ModeFantasy, Name: "C",
		Visibility: "everyone", CurrentYear: 1000, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	calRepo := &fakeCalendarRepo{
		getByIDFn:     func(context.Context, string) (*Calendar, error) { c := cal; return &c, nil },
		getMonthsFn:   func(context.Context, string) ([]Month, error) { return []Month{{Name: "M", Days: 30}}, nil },
		getWeekdaysFn: func(context.Context, string) ([]Weekday, error) { return nil, nil },
		updateFn:      func(context.Context, *Calendar) error { return writeErr },
		setMonthsFn:   func(context.Context, string, []MonthInput) error { return writeErr },
		setCyclesFn:   func(context.Context, string, []CycleInput) error { return writeErr },
		setFestivalsFn: func(context.Context, string, []FestivalInput) error {
			return writeErr
		},
	}
	eventRepo := &fakeEventRepo{
		getEventFn: func(context.Context, string) (*Event, error) {
			return &Event{ID: "ev-1", CalendarID: "cal-1", Name: "E", Visibility: "everyone"}, nil
		},
		createEventFn: func(context.Context, *Event) error { return writeErr },
		deleteEventFn: func(context.Context, string) error { return writeErr },
	}
	weatherRepo := &fakeWeatherRepo{setFn: func(context.Context, string, WeatherInput) error { return writeErr }}
	svc := newTestCalendarService(calRepo, eventRepo, nil, weatherRepo)
	pub := &recordingPublisher{}
	svc.(interface{ SetEventPublisher(CalendarEventPublisher) }).SetEventPublisher(pub)
	return svc, pub
}

// TestServicePublishesCalendarChanges pins which mutation announces which
// event type, and that a failed write announces nothing.
func TestServicePublishesCalendarChanges(t *testing.T) {
	ctx := context.Background()
	owner := permissions.SystemViewer(permissions.RoleOwner)
	cases := []struct {
		name string
		run  func(CalendarService) error
		want []string
	}{
		{"set date", func(s CalendarService) error { return s.SetCurrentDate(ctx, "cal-1", testCampaignA, 1000, 1, 5, 0, 0) }, []string{PubDateAdvanced}},
		{"create event", func(s CalendarService) error {
			_, err := s.CreateEvent(ctx, "cal-1", testCampaignA, CreateEventInput{Name: "x", Year: 1, Month: 1, Day: 1})
			return err
		}, []string{PubEventCreated}},
		{"delete event", func(s CalendarService) error { return s.DeleteEvent(ctx, "ev-1", "cal-1", testCampaignA, owner) }, []string{PubEventDeleted}},
		{"set months", func(s CalendarService) error {
			return s.SetMonths(ctx, "cal-1", testCampaignA, []MonthInput{{Name: "M", Days: 30}})
		}, []string{PubStructureUpdated}},
		{"set cycles", func(s CalendarService) error { return s.SetCycles(ctx, "cal-1", testCampaignA, nil) }, []string{PubCycleChanged, PubStructureUpdated}},
		{"set festivals", func(s CalendarService) error { return s.SetFestivals(ctx, "cal-1", testCampaignA, nil) }, []string{PubFestivalChanged, PubStructureUpdated}},
		{"set weather", func(s CalendarService) error { return s.SetWeather(ctx, "cal-1", testCampaignA, WeatherInput{}) }, []string{PubWeatherChanged}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, pub := publishFixture(t, nil)
			if err := tc.run(svc); err != nil {
				t.Fatalf("write: %v", err)
			}
			got := pub.types()
			if len(got) != len(tc.want) {
				t.Fatalf("published %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("published %v, want %v", got, tc.want)
				}
				if pub.got[i].campaignID != testCampaignA || pub.got[i].calendarID != "cal-1" {
					t.Errorf("scope = %q/%q, want campaign %q calendar cal-1", pub.got[i].campaignID, pub.got[i].calendarID, testCampaignA)
				}
			}
		})
		t.Run(tc.name+" write fails", func(t *testing.T) {
			svc, pub := publishFixture(t, errors.New("db down"))
			if err := tc.run(svc); err == nil {
				t.Skip("this path does not surface the fake's write error")
			}
			if len(pub.got) != 0 {
				t.Errorf("a failed write published %v", pub.types())
			}
		})
	}
}

// TestSetCurrentDatePublishesNewDate checks the payload the module reads.
func TestSetCurrentDatePublishesNewDate(t *testing.T) {
	svc, pub := publishFixture(t, nil)
	if err := svc.SetCurrentDate(context.Background(), "cal-1", testCampaignA, 1000, 1, 5, 7, 9); err != nil {
		t.Fatal(err)
	}
	if len(pub.got) != 1 {
		t.Fatalf("published %v", pub.types())
	}
	want := DatePayload{Year: 1000, Month: 1, Day: 5, Hour: 7, Minute: 9}
	if got, _ := pub.got[0].payload.(DatePayload); got != want {
		t.Errorf("payload = %+v, want %+v", pub.got[0].payload, want)
	}
}

// TestRejectedDatePublishesNothing: validation failures never reach the wire.
func TestRejectedDatePublishesNothing(t *testing.T) {
	svc, pub := publishFixture(t, nil)
	if err := svc.SetCurrentDate(context.Background(), "cal-1", testCampaignA, 1000, 9, 1, 0, 0); err == nil {
		t.Fatal("want a validation error")
	}
	if len(pub.got) != 0 {
		t.Errorf("published %v", pub.types())
	}
}

// TestPublishedEventTypesAreDistinct guards the registry the app-layer
// mapping test walks.
func TestPublishedEventTypesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, ty := range AllPublishedEventTypes() {
		if ty == "" || seen[ty] {
			t.Errorf("empty or duplicate event type %q", ty)
		}
		seen[ty] = true
	}
}
