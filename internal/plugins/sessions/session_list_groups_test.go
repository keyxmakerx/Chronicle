package sessions

import (
	"reflect"
	"testing"
)

func TestGroupSessions(t *testing.T) {
	d := func(s string) *string { return &s }
	list := []Session{
		{ID: "oct9", Status: StatusPlanned, ScheduledDate: d("2026-10-09")},
		{ID: "oct23", Status: StatusPlanned, ScheduledDate: d("2026-10-23")},
		{ID: "tonight", Status: StatusPlanned, ScheduledDate: d("2026-10-24")},
		{ID: "weekly", Status: StatusPlanned, ScheduledDate: d("2026-09-01"), IsRecurring: true},
		{ID: "undated", Status: StatusPlanned},
		{ID: "done", Status: StatusCompleted, ScheduledDate: d("2026-10-02")},
		{ID: "off", Status: StatusCancelled, ScheduledDate: d("2026-10-16")},
	}
	got := map[string][]string{}
	var order []string
	for _, g := range groupSessions(list, "2026-10-24") {
		order = append(order, g.Key)
		for _, s := range g.Sessions {
			got[g.Key] = append(got[g.Key], s.ID)
		}
	}
	want := map[string][]string{
		"upcoming":  {"tonight", "weekly", "undated"},
		"played":    {"oct23", "oct9"},
		"completed": {"done"},
		"cancelled": {"off"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("groups = %v, want %v", got, want)
	}
	if want := []string{"upcoming", "played", "completed", "cancelled"}; !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestGroupSessions_DropsEmptyGroups(t *testing.T) {
	d := "2026-12-01"
	groups := groupSessions([]Session{{ID: "a", Status: StatusPlanned, ScheduledDate: &d}}, "2026-10-24")
	if len(groups) != 1 || groups[0].Key != "upcoming" {
		t.Fatalf("got %+v, want only upcoming", groups)
	}
}
