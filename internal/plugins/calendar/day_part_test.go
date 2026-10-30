package calendar

import "testing"

func TestDayPart(t *testing.T) {
	months := make([]Month, 12)
	for i := range months {
		months[i] = Month{Name: "M", Days: 30}
	}
	north := &Calendar{HoursPerDay: 24, MinutesPerHour: 60, Months: months}
	south := HemisphereSouth
	southern := &Calendar{HoursPerDay: 24, MinutesPerHour: 60, Months: months, Hemisphere: &south}
	long := &Calendar{HoursPerDay: 20, MinutesPerHour: 100, Months: months}

	tests := []struct {
		name               string
		cal                *Calendar
		month, day, h, min int
		want               string
	}{
		{"noon", north, 4, 1, 12, 0, DayPartDay},
		{"midnight", north, 4, 1, 0, 0, DayPartNight},
		{"equinox sunrise", north, 4, 1, 6, 0, DayPartDawn},
		{"equinox sunset", north, 4, 1, 18, 0, DayPartDusk},
		{"just after sunrise", north, 4, 1, 6, 20, DayPartDawn},
		{"mid-morning", north, 4, 1, 8, 0, DayPartDay},
		{"midwinter eight o'clock is still dark-ish", north, 1, 1, 7, 0, DayPartNight},
		{"midsummer eight in the evening is still light", north, 7, 1, 19, 0, DayPartDay},
		{"the south's first day is midsummer", southern, 1, 1, 19, 0, DayPartDay},
		{"a 20-hour day's noon is hour 10", long, 4, 1, 10, 0, DayPartDay},
		{"a 20-hour day's sunrise is hour 5", long, 4, 1, 5, 0, DayPartDawn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cal.DayPart(1, tt.month, tt.day, tt.h, tt.min); got != tt.want {
				t.Errorf("DayPart = %q, want %q", got, tt.want)
			}
		})
	}
}
