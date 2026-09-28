package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
)

// scopedCalendarService is a CalendarService double whose
// GetCalendarForViewer is scoped to one campaign like the real one, and
// hides a dm_only calendar from anyone who doesn't skip per-user rules.
type scopedCalendarService struct {
	calendar.CalendarService
	cal *calendar.Calendar
	err error
}

func (f *scopedCalendarService) GetCalendarForViewer(_ context.Context, calendarID, campaignID string, v permissions.Viewer) (*calendar.Calendar, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.cal == nil || f.cal.ID != calendarID || f.cal.CampaignID != campaignID {
		return nil, apperror.NewNotFound("calendar not found")
	}
	if f.cal.Visibility == "dm_only" && !v.SkipsPerUserRules() {
		return nil, apperror.NewNotFound("calendar not found")
	}
	return f.cal, nil
}

func TestCalendarScopeAdapter_CalendarInCampaign(t *testing.T) {
	infraErr := errors.New("db down")
	own := &calendar.Calendar{ID: "cal-1", CampaignID: "camp-1", Visibility: "everyone"}
	hidden := &calendar.Calendar{ID: "cal-1", CampaignID: "camp-1", Visibility: "dm_only"}

	tests := []struct {
		name     string
		cal      *calendar.Calendar
		svcErr   error
		campaign string
		want     bool
		wantErr  error
	}{
		{name: "same campaign", cal: own, campaign: "camp-1", want: true},
		{name: "dm_only calendar in the same campaign", cal: hidden, campaign: "camp-1", want: true},
		{name: "another campaign's calendar", cal: own, campaign: "camp-2", want: false},
		{name: "no such calendar", cal: nil, campaign: "camp-1", want: false},
		{name: "failed read is an error, not a miss", svcErr: infraErr, campaign: "camp-1", wantErr: infraErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &calendarScopeAdapter{svc: &scopedCalendarService{cal: tc.cal, err: tc.svcErr}}
			got, err := a.CalendarInCampaign(context.Background(), tc.campaign, "cal-1")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("CalendarInCampaign = %v, want %v", got, tc.want)
			}
		})
	}
}
