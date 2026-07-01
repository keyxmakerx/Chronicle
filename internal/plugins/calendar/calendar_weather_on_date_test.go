// calendar_weather_on_date_test.go — the additive weather-on-date path
// (C-CAL-EDITOR-EXPANSION PR2). With WeatherDate set, the weather override lands
// on that day; absent, it lands on the calendar's current date — the prior
// behavior, regression-pinned so the (additive) wire contract stays unchanged.
package calendar

import (
	"context"
	"testing"
)

func TestSetWorldState_WeatherDate(t *testing.T) {
	cal := gmTestCalendar() // current = 1492-6-15
	var gotY, gotMo, gotD int
	newSvc := func() CalendarService {
		repo := &mockCalendarRepo{
			// Mirror the PRODUCTION repo shape: GetByID scans only the
			// calendars row (Months always empty); months come from the
			// separate GetMonths read. A fully-populated GetByID here
			// masked the live bug where SetWorldState validated
			// weatherDate against the never-loaded cal.Months and
			// rejected every dated write.
			getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
				c := *cal
				c.Months = nil
				return &c, nil
			},
			getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return cal.Months, nil },
			setDayWeatherFn: func(_ context.Context, _ string, year, month, day int, _ string) error {
				gotY, gotMo, gotD = year, month, day
				return nil
			},
		}
		return NewCalendarService(repo)
	}
	rain := "rain"

	// A named WeatherDate retargets the override to that day.
	gotY, gotMo, gotD = 0, 0, 0
	if err := newSvc().SetWorldState(context.Background(), "cal-1",
		WorldStateUpdateInput{Weather: &rain, WeatherDate: &WorldStateWeatherDate{Year: 1500, Month: 2, Day: 3}}); err != nil {
		t.Fatalf("SetWorldState (dated): %v", err)
	}
	if gotY != 1500 || gotMo != 2 || gotD != 3 {
		t.Errorf("weatherDate override wrote %d-%d-%d; want 1500-2-3", gotY, gotMo, gotD)
	}

	// Absent WeatherDate behaves exactly as before — current date.
	gotY, gotMo, gotD = 0, 0, 0
	if err := newSvc().SetWorldState(context.Background(), "cal-1",
		WorldStateUpdateInput{Weather: &rain}); err != nil {
		t.Fatalf("SetWorldState (current): %v", err)
	}
	if gotY != 1492 || gotMo != 6 || gotD != 15 {
		t.Errorf("absent weatherDate wrote %d-%d-%d; want current 1492-6-15", gotY, gotMo, gotD)
	}

	// A WeatherDate that isn't a date on this calendar is rejected — nothing
	// is written. (12 months × 30 days; the year is deliberately unbounded.)
	for _, bad := range []WorldStateWeatherDate{
		{Year: 1492, Month: 0, Day: 10},  // month below range
		{Year: 1492, Month: 13, Day: 10}, // month past the calendar
		{Year: 1492, Month: 6, Day: 0},   // day below range
		{Year: 1492, Month: 6, Day: 31},  // day past the month
	} {
		gotY, gotMo, gotD = 0, 0, 0
		bad := bad
		if err := newSvc().SetWorldState(context.Background(), "cal-1",
			WorldStateUpdateInput{Weather: &rain, WeatherDate: &bad}); err == nil {
			t.Errorf("weatherDate %d-%d-%d: want validation error, got nil", bad.Year, bad.Month, bad.Day)
		}
		if gotY != 0 || gotMo != 0 || gotD != 0 {
			t.Errorf("weatherDate %d-%d-%d: rejected date must not write (wrote %d-%d-%d)",
				bad.Year, bad.Month, bad.Day, gotY, gotMo, gotD)
		}
	}

	// Negative-era years and the month's exact last day stay valid.
	gotY, gotMo, gotD = 0, 0, 0
	if err := newSvc().SetWorldState(context.Background(), "cal-1",
		WorldStateUpdateInput{Weather: &rain, WeatherDate: &WorldStateWeatherDate{Year: -32, Month: 12, Day: 30}}); err != nil {
		t.Fatalf("SetWorldState (negative year, last day): %v", err)
	}
	if gotY != -32 || gotMo != 12 || gotD != 30 {
		t.Errorf("edge weatherDate wrote %d-%d-%d; want -32-12-30", gotY, gotMo, gotD)
	}

	// A calendar with no months configured skips the upper-bound check (the
	// Time-branch tolerance) — the write proceeds; below-range stays rejected.
	gotY, gotMo, gotD = 0, 0, 0
	noMonths := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			c := *cal
			c.Months = nil
			return &c, nil
		},
		getMonthsFn: func(_ context.Context, _ string) ([]Month, error) { return nil, nil },
		setDayWeatherFn: func(_ context.Context, _ string, year, month, day int, _ string) error {
			gotY, gotMo, gotD = year, month, day
			return nil
		},
	}
	if err := NewCalendarService(noMonths).SetWorldState(context.Background(), "cal-1",
		WorldStateUpdateInput{Weather: &rain, WeatherDate: &WorldStateWeatherDate{Year: 1500, Month: 2, Day: 3}}); err != nil {
		t.Fatalf("SetWorldState (no months configured): %v", err)
	}
	if gotY != 1500 || gotMo != 2 || gotD != 3 {
		t.Errorf("no-months weatherDate wrote %d-%d-%d; want 1500-2-3", gotY, gotMo, gotD)
	}
	if err := NewCalendarService(noMonths).SetWorldState(context.Background(), "cal-1",
		WorldStateUpdateInput{Weather: &rain, WeatherDate: &WorldStateWeatherDate{Year: 1500, Month: 0, Day: 3}}); err == nil {
		t.Error("no-months weatherDate month 0: want validation error, got nil")
	}
}
