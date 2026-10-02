// set_current_date_test.go: table-driven tests for SetCurrentDate's
// validation against the calendar's own geometry and its write-through.
package calendar

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// setDateFixture builds a 2-month fantasy calendar (30 and 20 days) whose
// first month gains one day in leap years (every 4th year), and records the
// calendars passed to Update.
func setDateFixture(t *testing.T, mut func(*Calendar)) (CalendarService, *[]Calendar) {
	t.Helper()
	cal := Calendar{
		ID: "cal-1", CampaignID: testCampaignA, Mode: ModeFantasy,
		Name: "Date Test Calendar", Visibility: "everyone",
		CurrentYear: 1000, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
		LeapYearEvery: 4,
	}
	if mut != nil {
		mut(&cal)
	}
	months := []Month{
		{Name: "First", Days: 30, LeapYearDays: 1, SortOrder: 0},
		{Name: "Second", Days: 20, SortOrder: 1},
	}
	var updates []Calendar
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			c := cal
			return &c, nil
		},
		getMonthsFn:   func(_ context.Context, _ string) ([]Month, error) { return months, nil },
		getWeekdaysFn: func(_ context.Context, _ string) ([]Weekday, error) { return nil, nil },
		updateFn: func(_ context.Context, c *Calendar) error {
			updates = append(updates, *c)
			return nil
		},
	}
	return newTestCalendarService(calRepo, nil, nil, nil), &updates
}

func TestSetCurrentDate_Validation(t *testing.T) {
	tests := []struct {
		name       string
		campaign   string
		mut        func(*Calendar)
		y, mo, d   int
		h, mi      int
		wantStatus int
	}{
		{name: "wrong campaign", campaign: testCampaignB, y: 1000, mo: 1, d: 1, wantStatus: http.StatusNotFound},
		{name: "real-time calendar", campaign: testCampaignA, y: 1000, mo: 1, d: 1,
			mut: func(c *Calendar) {
				c.Mode = ModeRealLife
				c.TracksRealTime = true
				c.RealTimeZone = strPtr("UTC")
			},
			wantStatus: http.StatusUnprocessableEntity},
		{name: "month zero", campaign: testCampaignA, y: 1000, mo: 0, d: 1, wantStatus: http.StatusBadRequest},
		{name: "month past last", campaign: testCampaignA, y: 1000, mo: 3, d: 1, wantStatus: http.StatusBadRequest},
		{name: "day zero", campaign: testCampaignA, y: 1000, mo: 1, d: 0, wantStatus: http.StatusBadRequest},
		{name: "day past month length", campaign: testCampaignA, y: 1000, mo: 2, d: 21, wantStatus: http.StatusBadRequest},
		{name: "leap day in non-leap year", campaign: testCampaignA, y: 1001, mo: 1, d: 31, wantStatus: http.StatusBadRequest},
		{name: "hour at HoursPerDay", campaign: testCampaignA, y: 1000, mo: 1, d: 1, h: 24, wantStatus: http.StatusBadRequest},
		{name: "negative hour", campaign: testCampaignA, y: 1000, mo: 1, d: 1, h: -1, wantStatus: http.StatusBadRequest},
		{name: "minute at MinutesPerHour", campaign: testCampaignA, y: 1000, mo: 1, d: 1, mi: 60, wantStatus: http.StatusBadRequest},
		{name: "negative minute", campaign: testCampaignA, y: 1000, mo: 1, d: 1, mi: -1, wantStatus: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, updates := setDateFixture(t, tc.mut)
			err := svc.SetCurrentDate(context.Background(), "cal-1", tc.campaign, tc.y, tc.mo, tc.d, tc.h, tc.mi)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tc.wantStatus {
				t.Fatalf("want AppError %d, got %v", tc.wantStatus, err)
			}
			if len(*updates) != 0 {
				t.Errorf("a rejected date must not write; got %d updates", len(*updates))
			}
		})
	}
}

func TestSetCurrentDate_ValidDateWrites(t *testing.T) {
	tests := []struct {
		name     string
		y, mo, d int
		h, mi    int
	}{
		{"ordinary date", 1005, 2, 20, 13, 45},
		{"leap day in leap year", 1000, 1, 31, 0, 0},
		{"last hour and minute of the day", 1002, 1, 1, 23, 59},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, updates := setDateFixture(t, nil)
			if err := svc.SetCurrentDate(context.Background(), "cal-1", testCampaignA, tc.y, tc.mo, tc.d, tc.h, tc.mi); err != nil {
				t.Fatalf("SetCurrentDate: %v", err)
			}
			if len(*updates) != 1 {
				t.Fatalf("want exactly one Update, got %d", len(*updates))
			}
			got := (*updates)[0]
			if got.CurrentYear != tc.y || got.CurrentMonth != tc.mo || got.CurrentDay != tc.d ||
				got.CurrentHour != tc.h || got.CurrentMinute != tc.mi {
				t.Errorf("stored %d-%d-%d %d:%d, want %d-%d-%d %d:%d",
					got.CurrentYear, got.CurrentMonth, got.CurrentDay, got.CurrentHour, got.CurrentMinute,
					tc.y, tc.mo, tc.d, tc.h, tc.mi)
			}
			if got.Name != "Date Test Calendar" {
				t.Errorf("Name = %q, want it preserved", got.Name)
			}
		})
	}
}
