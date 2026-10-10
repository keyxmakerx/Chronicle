package calendar

import "testing"

func TestAdvanceClock(t *testing.T) {
	// Two months of 30 and 20 days, a 24-hour day.
	earth := func(y, m, d, h, mi int) *Calendar {
		return &Calendar{
			Months: []Month{{Days: 30}, {Days: 20}}, HoursPerDay: 24, MinutesPerHour: 60,
			CurrentYear: y, CurrentMonth: m, CurrentDay: d, CurrentHour: h, CurrentMinute: mi,
		}
	}
	// A short world day: 10 hours of 100 minutes.
	odd := &Calendar{
		Months: []Month{{Days: 5}, {Days: 5}}, HoursPerDay: 10, MinutesPerHour: 100,
		CurrentYear: 7, CurrentMonth: 2, CurrentDay: 5, CurrentHour: 9, CurrentMinute: 50,
	}
	leap := &Calendar{
		LeapYearEvery: 4, Months: []Month{{Days: 28, LeapYearDays: 1}, {Days: 10}}, HoursPerDay: 24, MinutesPerHour: 60,
		CurrentYear: 4, CurrentMonth: 1, CurrentDay: 28, CurrentHour: 23, CurrentMinute: 0,
	}
	tests := []struct {
		name         string
		cal          *Calendar
		minutes      int
		want         DayDate
		wantH, wantM int
		wantOK       bool
	}{
		{"one hour within the day", earth(5, 1, 3, 10, 30), 60, DayDate{5, 1, 3}, 11, 30, true},
		{"eight hours across midnight", earth(5, 1, 3, 20, 0), 8 * 60, DayDate{5, 1, 4}, 4, 0, true},
		{"a day keeps the time", earth(5, 1, 3, 20, 15), 24 * 60, DayDate{5, 1, 4}, 20, 15, true},
		{"hour lands exactly on midnight", earth(5, 1, 3, 23, 0), 60, DayDate{5, 1, 4}, 0, 0, true},
		{"across a month end", earth(5, 1, 30, 23, 0), 2 * 60, DayDate{5, 2, 1}, 1, 0, true},
		{"across a year end", earth(5, 2, 20, 22, 0), 3 * 60, DayDate{6, 1, 1}, 1, 0, true},
		{"a day over a year end", earth(5, 2, 20, 8, 0), 24 * 60, DayDate{6, 1, 1}, 8, 0, true},
		{"the calendar's own short day", odd, 100, DayDate{8, 1, 1}, 0, 50, true},
		{"a calendar day on the short calendar", odd, 10 * 100, DayDate{8, 1, 1}, 9, 50, true},
		{"leap day is counted", leap, 60, DayDate{4, 1, 29}, 0, 0, true},
		{"zero moves nothing", earth(5, 1, 3, 10, 30), 0, DayDate{5, 1, 3}, 10, 30, true},
		{"negative refused", earth(5, 1, 3, 10, 30), -1, DayDate{}, 0, 0, false},
		{"no hours in the day refused", &Calendar{Months: []Month{{Days: 5}}}, 60, DayDate{}, 0, 0, false},
		{"too far refused", earth(5, 1, 3, 0, 0), 24 * 60 * 400, DayDate{}, 0, 0, false},
		{"month-less calendar gives up", &Calendar{Months: []Month{{Days: 0}}, HoursPerDay: 24, MinutesPerHour: 60, CurrentMonth: 1, CurrentDay: 1}, 24 * 60, DayDate{}, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, h, m, ok := tt.cal.advanceClock(tt.minutes)
			if ok != tt.wantOK || got != tt.want || h != tt.wantH || m != tt.wantM {
				t.Errorf("advanceClock = %v %d:%d %v; want %v %d:%d %v", got, h, m, ok, tt.want, tt.wantH, tt.wantM, tt.wantOK)
			}
		})
	}
}
