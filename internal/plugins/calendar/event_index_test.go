// event_index_test.go: the calendar-wide event index must name exactly the
// events a month read would show the same viewer, no more.
package calendar

import (
	"context"
	"fmt"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func newIndexService(cal Calendar, eras []Era, events []Event) CalendarService {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id != cal.ID {
				return nil, apperror.NewNotFound("calendar not found")
			}
			c := cal
			return &c, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) { return eraMonths(), nil },
		getErasFn:   func(context.Context, string) ([]Era, error) { return append([]Era(nil), eras...), nil },
	}
	eventRepo := &fakeEventRepo{
		listAllFn: func(context.Context, string) ([]Event, error) { return append([]Event(nil), events...), nil },
	}
	return NewCalendarService(calRepo, eventRepo, &fakeEventKindRepo{}, &fakeWeatherRepo{})
}

func TestListEventIndexForViewer(t *testing.T) {
	cal, eras, events := eraFixture()
	events = append(events, Event{ID: "ev-dm", CalendarID: eraCalendarID, Name: "Assassination plot",
		Year: 1022, Month: 8, Day: 1, Visibility: "dm_only"})
	svc := newIndexService(cal, eras, events)

	tests := []struct {
		name string
		role int
		q    string
		want []string
	}{
		{"player sees neither the dm-only nor the secret-era event", permissions.RolePlayer, "", []string{"ev-visible"}},
		{"scribe is still not a Director", permissions.RoleScribe, "", []string{"ev-visible"}},
		{"owner sees every event", permissions.RoleOwner, "", []string{"ev-visible", "ev-secret", "ev-dm"}},
		{"name filter is case-insensitive", permissions.RoleOwner, "FOUNDING", []string{"ev-visible"}},
		{"a player's filter cannot surface a hidden event", permissions.RolePlayer, "plot", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, truncated, err := svc.ListEventIndexForViewer(context.Background(), eraCalendarID, eraCampaignID, tt.q,
				permissions.RequestViewer(tt.role, "u-1"))
			if err != nil {
				t.Fatalf("ListEventIndexForViewer: %v", err)
			}
			if truncated {
				t.Error("truncated = true, want false")
			}
			var ids []string
			for _, e := range got {
				ids = append(ids, e.ID)
			}
			if !equalStrings(ids, tt.want) {
				t.Errorf("got %v, want %v", ids, tt.want)
			}
		})
	}

	t.Run("other campaign's calendar is not found", func(t *testing.T) {
		_, _, err := svc.ListEventIndexForViewer(context.Background(), eraCalendarID, "other-campaign", "",
			permissions.RequestViewer(permissions.RoleOwner, "u-1"))
		if err == nil {
			t.Fatal("want an error for a calendar outside the campaign")
		}
	})
}

func TestListEventIndexForViewer_Cap(t *testing.T) {
	cal, eras, _ := eraFixture()
	var many []Event
	for i := 0; i < maxEventIndex+5; i++ {
		many = append(many, Event{ID: fmt.Sprintf("ev-%d", i), CalendarID: eraCalendarID, Name: "Fair",
			Year: 1020, Month: 1, Day: 1, Visibility: "everyone"})
	}
	svc := newIndexService(cal, eras, many)
	got, truncated, err := svc.ListEventIndexForViewer(context.Background(), eraCalendarID, eraCampaignID, "",
		permissions.RequestViewer(permissions.RoleOwner, "u-1"))
	if err != nil {
		t.Fatalf("ListEventIndexForViewer: %v", err)
	}
	if len(got) != maxEventIndex || !truncated {
		t.Errorf("got %d entries truncated=%v, want %d and true", len(got), truncated, maxEventIndex)
	}
}
