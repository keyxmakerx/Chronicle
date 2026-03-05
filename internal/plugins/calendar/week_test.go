package calendar

import (
	"testing"
)

func TestWeekViewData_WeekDays(t *testing.T) {
	cal := &Calendar{
		Months: []Month{
			{Name: "January", Days: 30},
			{Name: "February", Days: 28},
		},
		Weekdays: []Weekday{
			{Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"},
			{Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"}, {Name: "Sun"},
		},
		CurrentYear:  1,
		CurrentMonth: 1,
		CurrentDay:   10,
	}

	data := WeekViewData{
		Calendar:   cal,
		Year:       1,
		MonthIndex: 1,
		StartDay:   8,
		Events:     []Event{{Year: 1, Month: 1, Day: 10, Name: "Test Event"}},
	}

	days := data.WeekDays()
	if len(days) != 7 {
		t.Fatalf("expected 7 days, got %d", len(days))
	}
	if days[0].Day != 8 {
		t.Errorf("expected first day 8, got %d", days[0].Day)
	}
	if days[6].Day != 14 {
		t.Errorf("expected last day 14, got %d", days[6].Day)
	}

	// Day 10 should have 1 event.
	if len(days[2].Events) != 1 {
		t.Errorf("expected 1 event on day 10, got %d", len(days[2].Events))
	}
	if days[2].Events[0].Name != "Test Event" {
		t.Errorf("expected event name 'Test Event', got %q", days[2].Events[0].Name)
	}

	// Day 10 should be today.
	if !days[2].IsToday {
		t.Error("expected day 10 to be today")
	}
	if days[0].IsToday {
		t.Error("expected day 8 to not be today")
	}
}

func TestWeekViewData_CrossMonth(t *testing.T) {
	cal := &Calendar{
		Months: []Month{
			{Name: "January", Days: 30},
			{Name: "February", Days: 28},
		},
		Weekdays: []Weekday{
			{Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"},
			{Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"}, {Name: "Sun"},
		},
		CurrentYear: 99,
	}

	data := WeekViewData{
		Calendar:   cal,
		Year:       1,
		MonthIndex: 1,
		StartDay:   27,
	}

	days := data.WeekDays()
	if len(days) != 7 {
		t.Fatalf("expected 7 days, got %d", len(days))
	}
	// Days 27-30 in January, then 1-3 in February.
	if days[0].Day != 27 || days[0].Month != 1 {
		t.Errorf("expected day 27 month 1, got day %d month %d", days[0].Day, days[0].Month)
	}
	if days[3].Day != 30 || days[3].Month != 1 {
		t.Errorf("expected day 30 month 1, got day %d month %d", days[3].Day, days[3].Month)
	}
	if days[4].Day != 1 || days[4].Month != 2 {
		t.Errorf("expected day 1 month 2, got day %d month %d", days[4].Day, days[4].Month)
	}
}

func TestWeekViewData_PrevNextWeek(t *testing.T) {
	cal := &Calendar{
		Months: []Month{
			{Name: "January", Days: 30},
			{Name: "February", Days: 28},
		},
		Weekdays: []Weekday{
			{Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"},
			{Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"}, {Name: "Sun"},
		},
	}

	data := WeekViewData{
		Calendar:   cal,
		Year:       1,
		MonthIndex: 1,
		StartDay:   15,
	}

	// Next week should be day 22.
	ny, nm, nd := data.NextWeek()
	if ny != 1 || nm != 1 || nd != 22 {
		t.Errorf("expected next week 1/1/22, got %d/%d/%d", ny, nm, nd)
	}

	// Prev week should be day 8.
	py, pm, pd := data.PrevWeek()
	if py != 1 || pm != 1 || pd != 8 {
		t.Errorf("expected prev week 1/1/8, got %d/%d/%d", py, pm, pd)
	}
}

func TestWeekViewData_PrevWeek_CrossMonth(t *testing.T) {
	cal := &Calendar{
		Months: []Month{
			{Name: "January", Days: 30},
			{Name: "February", Days: 28},
		},
		Weekdays: []Weekday{
			{Name: "Mon"}, {Name: "Tue"}, {Name: "Wed"},
			{Name: "Thu"}, {Name: "Fri"}, {Name: "Sat"}, {Name: "Sun"},
		},
	}

	data := WeekViewData{
		Calendar:   cal,
		Year:       1,
		MonthIndex: 2,
		StartDay:   1,
	}

	// Prev week should wrap to January.
	py, pm, pd := data.PrevWeek()
	if pm != 1 {
		t.Errorf("expected prev week in month 1, got month %d", pm)
	}
	if pd != 24 {
		t.Errorf("expected prev week day 24, got %d", pd)
	}
	if py != 1 {
		t.Errorf("expected prev week year 1, got %d", py)
	}
}

func TestWeekViewData_WeekdayName(t *testing.T) {
	cal := &Calendar{
		Weekdays: []Weekday{
			{Name: "Moonday"},
			{Name: "Starday"},
		},
	}

	data := WeekViewData{Calendar: cal}

	if data.WeekdayName(0) != "Moonday" {
		t.Errorf("expected Moonday, got %s", data.WeekdayName(0))
	}
	if data.WeekdayName(1) != "Starday" {
		t.Errorf("expected Starday, got %s", data.WeekdayName(1))
	}
	if data.WeekdayName(5) != "" {
		t.Errorf("expected empty for out of bounds, got %s", data.WeekdayName(5))
	}
}
