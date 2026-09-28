package timeline

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// stubCalendarScope answers CalendarInCampaign from a fixed calendar -> campaign
// map and records every question it was asked.
type stubCalendarScope struct {
	owner map[string]string
	err   error
	asked []string
}

func (s *stubCalendarScope) CalendarInCampaign(_ context.Context, campaignID, calendarID string) (bool, error) {
	s.asked = append(s.asked, campaignID+"/"+calendarID)
	if s.err != nil {
		return false, s.err
	}
	return s.owner[calendarID] == campaignID, nil
}

// A timeline stores a calendar only once it is confirmed to be in the
// timeline's own campaign; a foreign or unknown calendar is refused with the
// same NotFound and nothing is written.
func TestCreateTimeline_CalendarMustBeInCampaign(t *testing.T) {
	infraErr := errors.New("db down")

	tests := []struct {
		name      string
		calendar  *string
		noScope   bool
		scopeErr  error
		wantCode  int // 0 = success
		wantErr   error
		wantCal   *string
		wantAsked []string
	}{
		{name: "no calendar", calendar: nil},
		{name: "empty calendar id", calendar: strPtr("")},
		{name: "calendar in the same campaign", calendar: strPtr("cal-own"),
			wantCal: strPtr("cal-own"), wantAsked: []string{"camp-1/cal-own"}},
		{name: "calendar in another campaign", calendar: strPtr("cal-other"),
			wantCode: 404, wantAsked: []string{"camp-1/cal-other"}},
		{name: "unknown calendar", calendar: strPtr("cal-missing"),
			wantCode: 404, wantAsked: []string{"camp-1/cal-missing"}},
		{name: "no scope check wired", calendar: strPtr("cal-own"), noScope: true,
			wantCode: 404},
		{name: "no scope check wired, no calendar", calendar: nil, noScope: true},
		{name: "scope read fails", calendar: strPtr("cal-own"), scopeErr: infraErr,
			wantErr: infraErr, wantAsked: []string{"camp-1/cal-own"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var created *Timeline
			repo := &mockTimelineRepo{createFn: func(_ context.Context, tl *Timeline) error {
				created = tl
				return nil
			}}
			svc := newTestTimelineService(repo)
			scope := &stubCalendarScope{
				owner: map[string]string{"cal-own": "camp-1", "cal-other": "camp-2"},
				err:   tc.scopeErr,
			}
			if !tc.noScope {
				svc.(*timelineService).SetCalendarScope(scope)
			}

			tl, err := svc.CreateTimeline(context.Background(), "camp-1", CreateTimelineInput{
				CampaignID: "camp-1", Name: "Saga", CalendarID: tc.calendar,
			})

			switch {
			case tc.wantCode != 0:
				assertAppError(t, err, tc.wantCode)
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want it to wrap %v", err, tc.wantErr)
				}
				var ae *apperror.AppError
				if errors.As(err, &ae) && ae.Code == 404 {
					t.Error("a failed read must not be reported as a missing calendar")
				}
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantCode != 0 || tc.wantErr != nil {
				if created != nil {
					t.Errorf("timeline was written with calendar %v despite the refusal", created.CalendarID)
				}
			} else {
				got := tl.CalendarID
				if (got == nil) != (tc.wantCal == nil) || (got != nil && *got != *tc.wantCal) {
					t.Errorf("CalendarID = %v, want %v", deref(got), deref(tc.wantCal))
				}
				if created == nil {
					t.Error("timeline was not written")
				}
			}

			if len(scope.asked) != len(tc.wantAsked) {
				t.Fatalf("scope asked %v, want %v", scope.asked, tc.wantAsked)
			}
			for i := range tc.wantAsked {
				if scope.asked[i] != tc.wantAsked[i] {
					t.Errorf("scope asked %v, want %v", scope.asked, tc.wantAsked)
				}
			}
		})
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
