// structure_limits_test.go pins the limits on whole-list structure writes
// (SetMonths/SetWeekdays/SetMoons/SetSeasons), which the campaign backup
// import reaches without going through the calendar import parsers, and the
// bound on a month's length that every per-day loop relies on.
package calendar

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestMonthDays_BoundedWhateverIsStored(t *testing.T) {
	cal := &Calendar{
		Months:         []Month{{Name: "Long", Days: 2_000_000_000, LeapYearDays: 2_000_000_000}},
		LeapYearEvery:  4,
		LeapYearOffset: 0,
	}
	for _, year := range []int{3, 4} {
		if got := cal.MonthDays(0, year); got != maxCalendarMonthDays {
			t.Errorf("MonthDays(0, %d) = %d, want the %d-day bound", year, got, maxCalendarMonthDays)
		}
	}
}

// structureWriteService returns a service whose calendar lookup succeeds and
// whose bulk setters record whether they were reached.
func structureWriteService(t *testing.T) (CalendarService, *bool, *[]MoonInput, *[]Season) {
	t.Helper()
	reached := false
	var gotMoons []MoonInput
	var gotSeasons []Season
	repo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA}, nil
		},
		setMonthsFn: func(context.Context, string, []MonthInput) error { reached = true; return nil },
		setWeekdaysFn: func(context.Context, string, []WeekdayInput) error {
			reached = true
			return nil
		},
		setMoonsFn: func(_ context.Context, _ string, m []MoonInput) error {
			reached = true
			gotMoons = m
			return nil
		},
		setSeasonsFn: func(_ context.Context, _ string, s []Season) error {
			reached = true
			gotSeasons = s
			return nil
		},
	}
	return newTestCalendarService(repo, nil, nil, nil), &reached, &gotMoons, &gotSeasons
}

func TestStructureWrites_RefuseOversizedInput(t *testing.T) {
	long := strings.Repeat("x", maxCalendarShortNameLength+1)
	longEffect := strings.Repeat("w", maxCalendarWeatherEffectLength+1)
	tooMany := func(n int) []MonthInput {
		out := make([]MonthInput, n)
		for i := range out {
			out[i] = MonthInput{Name: "M", Days: 30}
		}
		return out
	}
	cases := []struct {
		name  string
		write func(CalendarService) error
	}{
		{"too many months", func(s CalendarService) error {
			return s.SetMonths(context.Background(), "cal-1", testCampaignA, tooMany(maxCalendarMonths+1))
		}},
		{"month with no days", func(s CalendarService) error {
			return s.SetMonths(context.Background(), "cal-1", testCampaignA, []MonthInput{{Name: "M", Days: 0}})
		}},
		{"month over the day bound", func(s CalendarService) error {
			return s.SetMonths(context.Background(), "cal-1", testCampaignA, []MonthInput{{Name: "M", Days: maxCalendarMonthDays + 1}})
		}},
		{"leap days over the day bound", func(s CalendarService) error {
			return s.SetMonths(context.Background(), "cal-1", testCampaignA, []MonthInput{{Name: "M", Days: 30, LeapYearDays: maxCalendarMonthDays + 1}})
		}},
		{"month name too long", func(s CalendarService) error {
			return s.SetMonths(context.Background(), "cal-1", testCampaignA, []MonthInput{{Name: long, Days: 30}})
		}},
		{"too many weekdays", func(s CalendarService) error {
			return s.SetWeekdays(context.Background(), "cal-1", testCampaignA, make([]WeekdayInput, maxCalendarWeekdays+1))
		}},
		{"weekday name too long", func(s CalendarService) error {
			return s.SetWeekdays(context.Background(), "cal-1", testCampaignA, []WeekdayInput{{Name: long}})
		}},
		{"too many moons", func(s CalendarService) error {
			return s.SetMoons(context.Background(), "cal-1", testCampaignA, make([]MoonInput, maxCalendarMoons+1))
		}},
		{"moon name too long", func(s CalendarService) error {
			return s.SetMoons(context.Background(), "cal-1", testCampaignA, []MoonInput{{Name: long, CycleDays: 28}})
		}},
		{"too many seasons", func(s CalendarService) error {
			return s.SetSeasons(context.Background(), "cal-1", testCampaignA, make([]Season, maxCalendarSeasons+1))
		}},
		{"season weather effect too long", func(s CalendarService) error {
			return s.SetSeasons(context.Background(), "cal-1", testCampaignA, []Season{{Name: "S", WeatherEffect: &longEffect}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, reached, _, _ := structureWriteService(t)
			var ae *apperror.AppError
			if err := tc.write(svc); !errors.As(err, &ae) || ae.Code != 400 {
				t.Fatalf("got %v, want a 400 app error", err)
			}
			if *reached {
				t.Error("an over-limit write must be refused before it reaches the repository")
			}
		})
	}
}

func TestStructureWrites_NormalizeColors(t *testing.T) {
	svc, _, gotMoons, gotSeasons := structureWriteService(t)
	if err := svc.SetMoons(context.Background(), "cal-1", testCampaignA, []MoonInput{{Name: "Luna", CycleDays: 28, Color: "red;x:y"}}); err != nil {
		t.Fatalf("SetMoons: %v", err)
	}
	if err := svc.SetSeasons(context.Background(), "cal-1", testCampaignA, []Season{{Name: "Spring", Color: "abc"}}); err != nil {
		t.Fatalf("SetSeasons: %v", err)
	}
	if c := (*gotMoons)[0].Color; c != "#808080" {
		t.Errorf("stored moon color = %q, want the #808080 fallback", c)
	}
	if c := (*gotSeasons)[0].Color; c != "#aabbcc" {
		t.Errorf("stored season color = %q, want #aabbcc", c)
	}
}
