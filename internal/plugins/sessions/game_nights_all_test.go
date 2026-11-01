package sessions

import (
	"strings"
	"testing"
)

// TestAllNightsGroups pins what the calendar's "All game nights" list gets:
// planned nights carry the day the calendar opens them on, and nights the
// calendar never shows link to their own page instead.
func TestAllNightsGroups(t *testing.T) {
	list := []Session{
		{ID: "up", Name: "Next week", Status: StatusPlanned, ScheduledDate: strp("2026-11-05")},
		{ID: "series", Name: "Thursdays", Status: StatusPlanned, ScheduledDate: strp("2026-09-24"), IsRecurring: true, RecurrenceType: strp(RecurrenceWeekly)},
		{ID: "played", Name: "Last week", Status: StatusPlanned, ScheduledDate: strp("2026-10-22")},
		{ID: "done", Name: "Done", Status: StatusCompleted, ScheduledDate: strp("2026-09-01")},
		{ID: "off", Name: "Off", Status: StatusCancelled},
	}
	groups := allNightsGroups("c 1", groupSessions(list, "2026-10-31"), "2026-10-31", "")
	var got []string
	for _, g := range groups {
		for _, n := range g.Nights {
			got = append(got, g.Key+":"+n.SessionID+":"+n.Date+":"+n.Href+":"+n.Repeat)
		}
	}
	want := []string{
		"upcoming:up:2026-11-05::",
		"upcoming:series:2026-11-05::",
		"played:played:2026-10-22::",
		"completed:done::/campaigns/c%201/sessions/done:",
		"cancelled:off::/campaigns/c%201/sessions/off:",
	}
	for i := range want {
		if i >= len(got) || !strings.HasPrefix(got[i], want[i]) {
			t.Fatalf("nights = %q, want %q", got, want)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("nights = %q, want %q", got, want)
	}
	for _, g := range groups {
		for _, n := range g.Nights {
			if n.SessionID == "series" && n.Repeat == "" {
				t.Errorf("a series says how it repeats")
			}
		}
	}
}
