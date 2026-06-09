// app_dashboard_w5d_test.go — C-CAL-DASHBOARD-W5d: the batch upcoming read +
// next-event sort + the adaptive calendar widget (both container-query variants).
package calendar

import (
	"context"
	"strings"
	"testing"
)

func names(ds []CalendarEventDate) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return out
}

func TestUpcomingByCalendar_NextAndAgenda(t *testing.T) {
	// Current date 2026/6/8; one past event then several upcoming, ordered.
	repo := &mockCalendarRepo{
		eventDatesForCalsFn: func(_ context.Context, _ []string, _ int) (map[string][]CalendarEventDate, error) {
			return map[string][]CalendarEventDate{
				"c1": {
					{CalendarID: "c1", Year: 2026, Month: 6, Day: 1, Name: "Past"},
					{CalendarID: "c1", Year: 2026, Month: 6, Day: 8, Name: "Today"},
					{CalendarID: "c1", Year: 2026, Month: 6, Day: 20, Name: "Soon"},
					{CalendarID: "c1", Year: 2026, Month: 7, Day: 1, Name: "NextMonth"},
					{CalendarID: "c1", Year: 2026, Month: 8, Day: 1, Name: "Later"},
				},
			}, nil
		},
	}
	svc := newTestCalendarService(repo)
	cals := []Calendar{{ID: "c1", CurrentYear: 2026, CurrentMonth: 6, CurrentDay: 8}}

	up, err := svc.UpcomingByCalendar(context.Background(), cals, 3)
	if err != nil {
		t.Fatalf("UpcomingByCalendar: %v", err)
	}
	u := up["c1"]
	if u.Next == nil || u.Next.Name != "Today" {
		t.Errorf("Next = %v; want the first event ON/AFTER the current date (Today)", u.Next)
	}
	// Agenda capped at dashboardAgendaCap (3), starting from the first upcoming.
	if got := names(u.Agenda); len(got) != 3 || got[0] != "Today" || got[2] != "NextMonth" {
		t.Errorf("agenda = %v; want [Today Soon NextMonth] (capped, upcoming only)", got)
	}
}

func TestUpcomingByCalendar_NoUpcoming(t *testing.T) {
	repo := &mockCalendarRepo{
		eventDatesForCalsFn: func(_ context.Context, _ []string, _ int) (map[string][]CalendarEventDate, error) {
			return map[string][]CalendarEventDate{"c1": {{Year: 2020, Month: 1, Day: 1, Name: "Old"}}}, nil
		},
	}
	svc := newTestCalendarService(repo)
	up, _ := svc.UpcomingByCalendar(context.Background(), []Calendar{{ID: "c1", CurrentYear: 2026, CurrentMonth: 6, CurrentDay: 8}}, 3)
	if up["c1"].Next != nil {
		t.Errorf("a calendar with only past events has no Next; got %v", up["c1"].Next)
	}
}

func TestSortDashboardCalendars_NextEvent(t *testing.T) {
	cals := []Calendar{
		{ID: "far", CurrentYear: 2026, CurrentMonth: 1, CurrentDay: 1},
		{ID: "none", CurrentYear: 2026, CurrentMonth: 1, CurrentDay: 1},
		{ID: "soon", CurrentYear: 2026, CurrentMonth: 1, CurrentDay: 1},
	}
	upcoming := map[string]CalendarUpcoming{
		"far":  {Next: &CalendarEventDate{Year: 2026, Month: 12, Day: 1}},
		"soon": {Next: &CalendarEventDate{Year: 2026, Month: 2, Day: 1}},
		// "none" has no Next.
	}
	sortDashboardCalendars(cals, "nextevent", upcoming)
	if got := ids(cals); got[0] != "soon" || got[1] != "far" || got[2] != "none" {
		t.Errorf("nextevent order = %v; want [soon far none] (soonest first, none last)", got)
	}
	if normalizeCalendarSort("nextevent") != "nextevent" {
		t.Error("nextevent must be an accepted sort key")
	}
}

func renderAdaptive(t *testing.T, awd adaptiveCalData) string {
	t.Helper()
	var sb strings.Builder
	if err := adaptiveCalendarWidget(awd).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render adaptive widget: %v", err)
	}
	return sb.String()
}

func TestAdaptiveCalendarWidget_RendersBothVariants(t *testing.T) {
	// Card placement: no grid data, an agenda → compact "next" line + full agenda,
	// NO month-grid.
	card := renderAdaptive(t, adaptiveCalData{
		CampaignID: "camp",
		Cal:        &Calendar{},
		Agenda:     []CalendarEventDate{{Month: 6, Day: 20, Name: "Festival"}},
	})
	for _, want := range []string{"cal-adaptive__compact", "cal-adaptive__full", "data-cal-adaptive-agenda", "Festival", "Next: Festival"} {
		if !strings.Contains(card, want) {
			t.Errorf("card widget missing %q", want)
		}
	}
	if strings.Contains(card, "data-cal-adaptive-grid") {
		t.Error("a card (no eager grid data) must NOT render the month-grid")
	}

	// Embed placement: eager calendar → the full mode renders the mini month-grid.
	cal := gregorian2026()
	embed := renderAdaptive(t, adaptiveCalData{CampaignID: "camp", Cal: cal, GridView: embedGridView(cal)})
	for _, want := range []string{"cal-adaptive__compact", "cal-adaptive__full", "data-cal-adaptive-grid"} {
		if !strings.Contains(embed, want) {
			t.Errorf("embed widget missing %q", want)
		}
	}
}
