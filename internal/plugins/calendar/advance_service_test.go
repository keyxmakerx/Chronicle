package calendar

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AdvanceCurrent reads the stored date, rolls it with the calendar's own
// geometry and writes it through SetCurrentDate. The fixture is the 30 and 20
// day calendar from set_current_date_test.go (month one gains a day in leap
// years, every 4th).
func TestAdvanceCurrent(t *testing.T) {
	type at struct{ y, mo, d, h, mi int }
	tests := []struct {
		name        string
		from        at
		hours, days int
		want        at
		wantStatus  int
	}{
		{"plus one hour", at{1001, 1, 5, 10, 15}, 1, 0, at{1001, 1, 5, 11, 15}, 0},
		{"plus eight hours past midnight", at{1001, 1, 5, 20, 0}, 8, 0, at{1001, 1, 6, 4, 0}, 0},
		{"next day keeps the time", at{1001, 1, 5, 20, 30}, 0, 1, at{1001, 1, 6, 20, 30}, 0},
		{"month end", at{1001, 1, 30, 23, 0}, 1, 0, at{1001, 2, 1, 0, 0}, 0},
		{"year end", at{1001, 2, 20, 12, 0}, 0, 1, at{1002, 1, 1, 12, 0}, 0},
		{"leap day exists in a leap year", at{1000, 1, 30, 23, 0}, 1, 0, at{1000, 1, 31, 0, 0}, 0},
		{"leap day skipped in a common year", at{1001, 1, 30, 23, 0}, 1, 0, at{1001, 2, 1, 0, 0}, 0},
		{"backwards refused", at{1001, 1, 5, 0, 0}, -1, 0, at{}, http.StatusBadRequest},
		{"too far refused", at{1001, 1, 5, 0, 0}, 0, 400, at{}, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, updates := setDateFixture(t, func(c *Calendar) {
				c.CurrentYear, c.CurrentMonth, c.CurrentDay, c.CurrentHour, c.CurrentMinute = tc.from.y, tc.from.mo, tc.from.d, tc.from.h, tc.from.mi
			})
			err := svc.AdvanceCurrent(context.Background(), "cal-1", testCampaignA, tc.hours, tc.days)
			if tc.wantStatus != 0 {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code != tc.wantStatus || len(*updates) != 0 {
					t.Fatalf("want %d and no write, got %v (%d writes)", tc.wantStatus, err, len(*updates))
				}
				return
			}
			if err != nil || len(*updates) != 1 {
				t.Fatalf("err %v, %d writes", err, len(*updates))
			}
			g := (*updates)[0]
			if got := (at{g.CurrentYear, g.CurrentMonth, g.CurrentDay, g.CurrentHour, g.CurrentMinute}); got != tc.want {
				t.Errorf("stored %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAdvanceCurrent_Refusals(t *testing.T) {
	realTime := func(c *Calendar) {
		c.Mode = ModeRealLife
		c.TracksRealTime = true
		c.RealTimeZone = strPtr("UTC")
	}
	tests := []struct {
		name       string
		campaign   string
		mut        func(*Calendar)
		wantStatus int
	}{
		{"real-time calendar is 422 like Set today", testCampaignA, realTime, http.StatusUnprocessableEntity},
		{"another campaign's calendar", testCampaignB, nil, http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, updates := setDateFixture(t, tc.mut)
			err := svc.AdvanceCurrent(context.Background(), "cal-1", tc.campaign, 1, 0)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tc.wantStatus || len(*updates) != 0 {
				t.Fatalf("want %d and no write, got %v (%d writes)", tc.wantStatus, err, len(*updates))
			}
		})
	}
}
