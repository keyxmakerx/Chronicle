// calendar_v2_gmpanel_test.go — C-CAL-WORLDSTATE-GM-LIVE-CONTROL-PANEL 4a.
// The GM panel renders only for capability holders; the Part-6 advance verbs
// roll over correctly in both directions; the PUT advance path persists.
package calendar

import (
	"context"
	"strings"
	"testing"
)

func gmTestCalendar() *Calendar {
	months := make([]Month, 12)
	for i := range months {
		months[i] = Month{Name: "M" + string(rune('A'+i)), Days: 30}
	}
	return &Calendar{
		ID: "cal-1", CampaignID: "camp-1", Name: "Harptos",
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		CurrentYear: 1492, CurrentMonth: 6, CurrentDay: 15, CurrentHour: 12, CurrentMinute: 0,
		Months: months,
	}
}

func TestAdvanceClock_Rollover(t *testing.T) {
	base := gmTestCalendar()
	set := func(h, mi, d, mo, y int) *Calendar {
		c := *base
		c.CurrentHour, c.CurrentMinute, c.CurrentDay, c.CurrentMonth, c.CurrentYear = h, mi, d, mo, y
		return &c
	}
	cases := []struct {
		name                         string
		cal                          *Calendar
		days, hours, minutes         int
		wantY, wantMo, wantD, wH, wM int
	}{
		{"+1hr midday", set(12, 0, 15, 6, 1492), 0, 1, 0, 1492, 6, 15, 13, 0},
		{"+1hr rolls day", set(23, 0, 15, 6, 1492), 0, 1, 0, 1492, 6, 16, 0, 0},
		{"+1day", set(12, 0, 15, 6, 1492), 1, 0, 0, 1492, 6, 16, 12, 0},
		{"+1day rolls month", set(9, 0, 30, 6, 1492), 1, 0, 0, 1492, 7, 1, 9, 0},
		{"long rest 8h", set(20, 0, 15, 6, 1492), 0, 8, 0, 1492, 6, 16, 4, 0},
		{"+90min", set(0, 0, 15, 6, 1492), 0, 0, 90, 1492, 6, 15, 1, 30},
		{"step back 1hr rolls day", set(0, 0, 15, 6, 1492), 0, -1, 0, 1492, 6, 14, 23, 0},
		{"step back at month start", set(0, 0, 1, 7, 1492), 0, -1, 0, 1492, 6, 30, 23, 0},
		{"step back at year start", set(0, 0, 1, 1, 1492), 0, -1, 0, 1491, 12, 30, 23, 0},
	}
	for _, tc := range cases {
		y, mo, d, h, m := advanceClock(tc.cal, tc.days, tc.hours, tc.minutes)
		if y != tc.wantY || mo != tc.wantMo || d != tc.wantD || h != tc.wH || m != tc.wM {
			t.Errorf("%s: got %d-%d-%d %d:%02d, want %d-%d-%d %d:%02d",
				tc.name, y, mo, d, h, m, tc.wantY, tc.wantMo, tc.wantD, tc.wH, tc.wM)
		}
	}
}

// TestSetWorldState_AdvancePersists: the advance path writes the rolled-over
// date/time through the service.
func TestSetWorldState_AdvancePersists(t *testing.T) {
	cal := gmTestCalendar()
	cal.CurrentHour = 23
	var saved *Calendar
	repo := &mockCalendarRepo{
		getByIDFn:   func(_ context.Context, _ string) (*Calendar, error) { c := *cal; return &c, nil },
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return cal.Months, nil },
		updateFn:    func(_ context.Context, c *Calendar) error { saved = c; return nil },
	}
	svc := NewCalendarService(repo)
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{
		Advance: &WorldStateAdvance{Hours: 1},
	}); err != nil {
		t.Fatalf("SetWorldState advance: %v", err)
	}
	if saved == nil {
		t.Fatal("advance did not persist")
	}
	if saved.CurrentDay != 16 || saved.CurrentHour != 0 {
		t.Errorf("advance +1hr at 23:00 → day %d hour %d; want day 16 hour 0", saved.CurrentDay, saved.CurrentHour)
	}
}

// TestSetWorldState_WeatherPersists: the GM weather override writes the
// current date's authored weather (calendar_day_weather) via the PUT path.
func TestSetWorldState_WeatherPersists(t *testing.T) {
	cal := gmTestCalendar() // current = 1492-6-15
	var gotCal, gotType string
	var gotY, gotMo, gotD int
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { c := *cal; return &c, nil },
		setDayWeatherFn: func(_ context.Context, calendarID string, year, month, day int, wt string) error {
			gotCal, gotY, gotMo, gotD, gotType = calendarID, year, month, day, wt
			return nil
		},
	}
	svc := NewCalendarService(repo)
	rain := "rain"
	if err := svc.SetWorldState(context.Background(), "cal-1", WorldStateUpdateInput{Weather: &rain}); err != nil {
		t.Fatalf("SetWorldState weather: %v", err)
	}
	if gotCal != "cal-1" || gotType != "rain" || gotY != 1492 || gotMo != 6 || gotD != 15 {
		t.Errorf("weather override wrote (%s, %d-%d-%d, %q); want (cal-1, 1492-6-15, rain)",
			gotCal, gotY, gotMo, gotD, gotType)
	}
}

// TestGMPanel_AuthorityGated: the panel renders ONLY for capability holders.
func TestGMPanel_AuthorityGated(t *testing.T) {
	render := func(canControl bool) string {
		data := CalendarV2ViewData{ActiveCalendar: gmTestCalendar(), CanControlWorldState: canControl}
		var sb strings.Builder
		if err := gmControlPanelV2(data).Render(context.Background(), &sb); err != nil {
			t.Fatalf("render panel: %v", err)
		}
		return sb.String()
	}

	holder := render(true)
	for _, want := range []string{
		"data-gm-panel", "data-gm-advance", "data-gm-set-time", "data-gm-set-date", "data-gm-pause",
		"data-gm-weather", "data-gm-set-weather", "data-gm-mood", "data-gm-mood-clear", // 4b
	} {
		if !strings.Contains(holder, want) {
			t.Errorf("capability holder panel missing %q", want)
		}
	}
	// Players / Scribes (no control) receive NO panel markup at all — the gate
	// is server-side, not a CSS hide.
	if got := strings.TrimSpace(render(false)); got != "" {
		t.Errorf("non-holder must receive no GM panel markup, got: %q", got)
	}
}
