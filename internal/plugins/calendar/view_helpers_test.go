package calendar

import "testing"

func twoMonthCalendar() *Calendar {
	return &Calendar{
		ID: "cal-1", CurrentYear: 1, CurrentMonth: 1, CurrentDay: 3,
		Months:   []Month{{Name: "Firstmonth", Days: 5}, {Name: "Secondmonth", Days: 5}},
		Weekdays: []Weekday{{Name: "One"}, {Name: "Two"}, {Name: "Three"}, {Name: "Four"}, {Name: "Five"}},
	}
}

func TestBuildMonthGrid_PadsToFullWeeksAndMarksToday(t *testing.T) {
	cal := twoMonthCalendar()
	weeks := buildMonthGrid(cal, 1, 1, nil, nil)

	total := 0
	var todays int
	for _, wk := range weeks {
		if len(wk)%len(cal.Weekdays) != 0 && len(wk) != len(cal.Weekdays) {
			t.Fatalf("every row must be a full week (%d cols), got %d", len(cal.Weekdays), len(wk))
		}
		for _, cell := range wk {
			total++
			if !cell.Blank {
				if cell.Day < 1 || cell.Day > 5 {
					t.Errorf("day out of range: %d", cell.Day)
				}
				if cell.IsToday {
					todays++
					if cell.Day != 3 {
						t.Errorf("today marked on day %d, want 3", cell.Day)
					}
				}
			}
		}
	}
	if todays != 1 {
		t.Errorf("expected exactly one 'today' cell, got %d", todays)
	}
}

func TestBuildMonthGrid_MoonPhasePerDay(t *testing.T) {
	cal := twoMonthCalendar()
	moon := &Moon{CycleDays: 10, PhaseOffset: 0}
	weeks := buildMonthGrid(cal, 1, 1, nil, moon)
	found := false
	for _, wk := range weeks {
		for _, cell := range wk {
			if !cell.Blank && cell.Day == 1 {
				found = true
				if !cell.HasMoon {
					t.Error("expected HasMoon for day 1 when a moon is passed")
				}
			}
		}
	}
	if !found {
		t.Fatal("day 1 not found in grid")
	}
}

func TestCountEventsByDay_RecurrenceAware(t *testing.T) {
	cal := twoMonthCalendar()
	weekly := RecurrenceWeekly
	events := []Event{
		{ID: "base", Year: 1, Month: 1, Day: 1},
		{ID: "weekly", Year: 1, Month: 1, Day: 1, IsRecurring: true, RecurrenceType: &weekly},
	}
	counts := countEventsByDay(cal, events, 1, 1)
	if counts[1] != 2 {
		t.Errorf("day 1: got %d events, want 2 (base + weekly's first occurrence)", counts[1])
	}
	// The weekly event should recur again exactly WeekLength() days later.
	next := 1 + cal.WeekLength()
	if next <= 5 {
		if counts[next] != 1 {
			t.Errorf("day %d: got %d events, want 1 (the weekly recurrence)", next, counts[next])
		}
	}
}

func TestPresetFacts(t *testing.T) {
	ir := &ImportResult{
		Months:   []MonthInput{{Days: 30}, {Days: 31}},
		Weekdays: []WeekdayInput{{}, {}, {}, {}, {}, {}, {}},
		Moons:    []MoonInput{{}},
		Seasons:  []Season{{}, {}},
	}
	got := presetFacts(ir)
	want := "2 months, 61 days · 7-day weeks · 1 moon · 2 seasons"
	if got != want {
		t.Errorf("presetFacts() = %q, want %q", got, want)
	}
}

func TestMonthBarWidths_SumsToWhole(t *testing.T) {
	months := []MonthInput{{Days: 25}, {Days: 25}, {Days: 50}}
	widths := monthBarWidths(months)
	var sum float64
	for _, w := range widths {
		sum += w
	}
	if sum < 99.99 || sum > 100.01 {
		t.Errorf("widths sum to %v, want ~100", sum)
	}
	if widths[2] != 50 {
		t.Errorf("the 50-day month should be 50%%, got %v", widths[2])
	}
}

func TestCalendar_FullDateLabelAndMonthName(t *testing.T) {
	cal := twoMonthCalendar()
	if got := cal.CurrentMonthName(); got != "Firstmonth" {
		t.Errorf("CurrentMonthName() = %q, want %q", got, "Firstmonth")
	}
	if got := cal.FullDateLabel(); got != "Firstmonth 3, 1" {
		t.Errorf("FullDateLabel() = %q, want %q", got, "Firstmonth 3, 1")
	}
	if got := cal.MonthName(99); got != "Month 99" {
		t.Errorf("MonthName(99) = %q, want the numeric fallback", got)
	}
}

func TestVisibilityLabel(t *testing.T) {
	tests := []struct {
		name string
		cal  Calendar
		want string
	}{
		{"everyone, no rules", Calendar{Visibility: "everyone"}, "Everyone in the campaign"},
		{"dm only", Calendar{Visibility: "dm_only"}, "Director only"},
		{"restricted to one", Calendar{Visibility: "everyone", VisibilityRules: strPtr(`{"allowed_users":["u-1"]}`)}, "Restricted to 1 player"},
		{"restricted to several", Calendar{Visibility: "everyone", VisibilityRules: strPtr(`{"allowed_users":["u-1","u-2"]}`)}, "Restricted to 2 players"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visibilityLabel(tt.cal); got != tt.want {
				t.Errorf("visibilityLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
