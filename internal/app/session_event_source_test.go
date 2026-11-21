package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

func TestSessionEventsOf(t *testing.T) {
	story := &calendar.Calendar{Mode: "fantasy", Months: []calendar.Month{{Name: "Alturiak"}, {Name: "Ches"}, {Name: "Hammer"}}}
	real := &calendar.Calendar{Mode: calendar.ModeRealLife}
	page := "page-1"
	ev := func(id, name, kind string, y, m, d int, recurring bool) calendar.Event {
		return calendar.Event{ID: id, CalendarID: "cal", Name: name, KindSlug: kind, Year: y, Month: m, Day: d, IsRecurring: recurring}
	}
	linked := ev("e1", "Session 1", "session", 1492, 3, 14, false)
	linked.EntityID, linked.EntityName = &page, "The Sunken Chapel"

	tests := []struct {
		name     string
		cal      *calendar.Calendar
		events   []calendar.Event
		wantIDs  []string
		wantWhen string
		wantReal string
	}{
		{"only the Session kind, in the calendar's months", story, []calendar.Event{linked, ev("e2", "Festival", "festival", 1492, 3, 15, false)}, []string{"e1"}, "14 Hammer, 1492", ""},
		{"the kind is matched without regard to case", story, []calendar.Event{ev("e3", "Session 2", "Session", 1492, 1, 2, false)}, []string{"e3"}, "2 Alturiak, 1492", ""},
		{"a repeating event is not one night", story, []calendar.Event{ev("e4", "Weekly", "session", 1492, 1, 2, true)}, nil, "", ""},
		{"an unnamed event is skipped", story, []calendar.Event{ev("e5", " ", "session", 1492, 1, 2, false)}, nil, "", ""},
		{"a month the calendar lacks keeps its number", story, []calendar.Event{ev("e6", "Odd", "session", 1492, 9, 2, false)}, []string{"e6"}, "2 9, 1492", ""},
		{"a real-life date is a real day", real, []calendar.Event{ev("e7", "Friday", "session", 2026, 10, 24, false)}, []string{"e7"}, "24 Oct 2026", "2026-10-24"},
		{"an impossible real date is not made a real day", real, []calendar.Event{ev("e8", "Bad", "session", 2026, 2, 31, false)}, []string{"e8"}, "31 2, 2026", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionEventsOf(tt.cal, tt.events)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d events, want %d: %+v", len(got), len(tt.wantIDs), got)
			}
			for i, id := range tt.wantIDs {
				if got[i].ID != id {
					t.Fatalf("event %d = %s, want %s", i, got[i].ID, id)
				}
			}
			if len(got) > 0 && (got[0].When != tt.wantWhen || got[0].RealDate != tt.wantReal) {
				t.Fatalf("when = %q real = %q, want %q %q", got[0].When, got[0].RealDate, tt.wantWhen, tt.wantReal)
			}
		})
	}
	if got := sessionEventsOf(story, []calendar.Event{linked}); got[0].EntityID != "page-1" || got[0].EntityName != "The Sunken Chapel" {
		t.Fatalf("the event's page is not carried: %+v", got[0])
	}
}

// fakeWorldCalendars serves calendars the way the real service does:
// ListCalendars returns them WITHOUT months, and only the by-id and primary
// reads carry months. Any use of ListCalendars (nil-embedded here) panics.
type fakeWorldCalendars struct {
	calendar.CalendarService
	full    map[string]calendar.Calendar
	primary string
	reads   int
}

func (f *fakeWorldCalendars) GetCalendarForViewer(_ context.Context, id, _ string, _ permissions.Viewer) (*calendar.Calendar, error) {
	f.reads++
	c, ok := f.full[id]
	if !ok {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return &c, nil
}

func (f *fakeWorldCalendars) GetPrimaryCalendarForViewer(_ context.Context, _ string, _ permissions.Viewer) (*calendar.Calendar, error) {
	f.reads++
	c, ok := f.full[f.primary]
	if !ok {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return &c, nil
}

// A date-less night's in-world day is worded in the months of the calendar it
// was recorded against ("22 Hammer, 1523", never "22 1, 1523"), and falls back
// to the primary calendar when that one is gone or was never recorded.
func TestWorldDateWordsUsesLoadedMonths(t *testing.T) {
	cal := func(id, month string) calendar.Calendar {
		return calendar.Calendar{ID: id, Months: []calendar.Month{{Name: month}, {Name: month + "2"}, {Name: month + "3"}}}
	}
	tests := []struct {
		name, calID, want string
	}{
		{"its own calendar", "b", "14 Frost3, 1492"},
		{"no calendar recorded", "", "14 Hammer3, 1492"},
		{"a calendar that is gone", "zzz", "14 Hammer3, 1492"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeWorldCalendars{full: map[string]calendar.Calendar{"a": cal("a", "Hammer"), "b": cal("b", "Frost")}, primary: "a"}
			words := (&worldDateWorderAdapter{svc: f}).WorldDateWordsFor(context.Background(), "camp")
			if got := words(tt.calID, 1492, 3, 14); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Many nights on one calendar load it once.
func TestWorldDateWordsLoadsEachCalendarOnce(t *testing.T) {
	f := &fakeWorldCalendars{full: map[string]calendar.Calendar{"a": {ID: "a", Months: []calendar.Month{{Name: "Hammer"}}}}, primary: "a"}
	words := (&worldDateWorderAdapter{svc: f}).WorldDateWordsFor(context.Background(), "camp")
	for i := 0; i < 5; i++ {
		if got := words("a", 1523, 1, 22); got != "22 Hammer, 1523" {
			t.Fatalf("got %q", got)
		}
	}
	if f.reads != 1 {
		t.Fatalf("calendar reads = %d, want 1", f.reads)
	}
}
