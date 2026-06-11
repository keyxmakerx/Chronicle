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
			getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { c := *cal; return &c, nil },
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
}
