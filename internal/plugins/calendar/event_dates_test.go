// event_dates_test.go: table-driven tests for the date checks CreateEvent
// and UpdateEvent run against the calendar's own geometry.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// eventDatesFixture is setDateFixture's calendar (30 and 20 days, the first
// month one longer every 4th year, 24 hours of 60 minutes) with an event
// repo holding one stored event, "evt-1", on 1000-01-10.
func eventDatesFixture(t *testing.T) (CalendarService, *int) {
	t.Helper()
	svc, _ := setDateFixture(t, nil)
	writes := 0
	stored := Event{ID: "evt-1", CalendarID: "cal-1", Name: "Feast", Year: 1000, Month: 1, Day: 10, Visibility: "everyone"}
	eventRepo := &fakeEventRepo{
		createEventFn: func(_ context.Context, _ *Event) error { writes++; return nil },
		updateEventFn: func(_ context.Context, _ *Event) error { writes++; return nil },
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			e := stored
			return &e, nil
		},
	}
	svc.(*calendarService).eventRepo = eventRepo
	return svc, &writes
}

func TestCreateEvent_DateChecks(t *testing.T) {
	i := intPtrForTest
	tests := []struct {
		name string
		mut  func(*CreateEventInput)
		ok   bool
	}{
		{"ordinary date", func(*CreateEventInput) {}, true},
		{"leap day in a leap year", func(in *CreateEventInput) { in.Day = 31 }, true},
		{"negative year", func(in *CreateEventInput) { in.Year = -5 }, true},
		{"end on a later day", func(in *CreateEventInput) { in.EndYear, in.EndMonth, in.EndDay = i(1000), i(2), i(3) }, true},
		{"end time after start time", func(in *CreateEventInput) { in.StartHour, in.EndHour = i(19), i(22) }, true},
		{"last hour and minute", func(in *CreateEventInput) { in.StartHour, in.StartMinute = i(23), i(59) }, true},
		{"custom repeat every 2", func(in *CreateEventInput) { in.RecurrenceInterval = i(2) }, true},

		{"month 99", func(in *CreateEventInput) { in.Month = 99 }, false},
		{"month 0", func(in *CreateEventInput) { in.Month = 0 }, false},
		{"day -3", func(in *CreateEventInput) { in.Day = -3 }, false},
		{"day past the month", func(in *CreateEventInput) { in.Month, in.Day = 2, 21 }, false},
		{"leap day in a common year", func(in *CreateEventInput) { in.Year, in.Day = 1001, 31 }, false},
		{"year beyond the stored range", func(in *CreateEventInput) { in.Year = maxDayWeatherYear + 1 }, false},
		{"hour 40", func(in *CreateEventInput) { in.StartHour = i(40) }, false},
		{"minute 60", func(in *CreateEventInput) { in.StartHour, in.StartMinute = i(10), i(60) }, false},
		{"end hour -1", func(in *CreateEventInput) { in.EndHour = i(-1) }, false},
		{"end date not in the calendar", func(in *CreateEventInput) { in.EndYear, in.EndMonth, in.EndDay = i(1000), i(3), i(1) }, false},
		{"end day before start", func(in *CreateEventInput) { in.EndYear, in.EndMonth, in.EndDay = i(1000), i(1), i(9) }, false},
		{"end year before start", func(in *CreateEventInput) { in.EndYear, in.EndMonth, in.EndDay = i(999), i(2), i(20) }, false},
		{"end time before start on the same day", func(in *CreateEventInput) { in.StartHour, in.EndHour = i(22), i(2) }, false},
		{"repeat-until not in the calendar", func(in *CreateEventInput) {
			in.RecurrenceEndYear, in.RecurrenceEndMonth, in.RecurrenceEndDay = i(1001), i(2), i(40)
		}, false},
		{"repeat interval 0", func(in *CreateEventInput) { in.RecurrenceInterval = i(0) }, false},
		{"repeat count 0", func(in *CreateEventInput) { in.RecurrenceMaxOccurrences = i(0) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, writes := eventDatesFixture(t)
			in := CreateEventInput{Name: "Feast", Year: 1000, Month: 1, Day: 10, Author: ownerViewer("u-owner")}
			tt.mut(&in)
			_, err := svc.CreateEvent(context.Background(), "cal-1", testCampaignA, in)
			if tt.ok {
				if err != nil {
					t.Fatalf("CreateEvent: %v", err)
				}
				return
			}
			wantValidationErr(t, err, tt.name)
			if *writes != 0 {
				t.Errorf("a refused event must not be stored; got %d writes", *writes)
			}
		})
	}
}

func TestUpdateEvent_DateChecks(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateEventInput
		ok   bool
	}{
		{"rename only", UpdateEventInput{Name: patch.Of("Harvest")}, true},
		{"move to a real day", UpdateEventInput{Month: patch.Of(2), Day: patch.Of(20)}, true},
		{"month 99", UpdateEventInput{Month: patch.Of(99)}, false},
		{"day 0", UpdateEventInput{Day: patch.Of(0)}, false},
		{"end before the stored start", UpdateEventInput{EndYear: patch.Of(1000), EndMonth: patch.Of(1), EndDay: patch.Of(2)}, false},
		{"repeat interval 0", UpdateEventInput{RecurrenceInterval: patch.Of(0)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, writes := eventDatesFixture(t)
			err := svc.UpdateEvent(context.Background(), "evt-1", "cal-1", testCampaignA, tt.in, ownerViewer("u-owner"))
			if tt.ok {
				if err != nil {
					t.Fatalf("UpdateEvent: %v", err)
				}
				return
			}
			wantValidationErr(t, err, tt.name)
			if *writes != 0 {
				t.Errorf("a refused update must not be stored; got %d writes", *writes)
			}
		})
	}
}
