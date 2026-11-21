package app

import (
	"testing"

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
