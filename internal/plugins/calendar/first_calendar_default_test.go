package calendar

import (
	"context"
	"testing"
)

// A campaign's first calendar becomes its default, so the readers that follow
// "the campaign's calendar" find it; a later one leaves the default alone.
func TestCreateCalendar_FirstCalendarIsDefault(t *testing.T) {
	tests := []struct {
		name        string
		existing    *Calendar
		wantDefault bool
	}{
		{"no default yet", nil, true},
		{"campaign already has a default", &Calendar{ID: "cal-old", CampaignID: testCampaignA, IsDefault: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created *Calendar
			repo := &fakeCalendarRepo{
				getDefaultFn: func(context.Context, string) (*Calendar, error) { return tt.existing, nil },
				createFn: func(_ context.Context, cal *Calendar) error {
					created = cal
					return nil
				},
			}
			svc := newTestCalendarService(repo, nil, nil, nil)
			if _, err := svc.CreateCalendar(context.Background(), testCampaignA, CreateCalendarInput{Name: "Harptos"}); err != nil {
				t.Fatalf("CreateCalendar: %v", err)
			}
			if created == nil || created.IsDefault != tt.wantDefault {
				t.Errorf("created IsDefault = %v, want %v", created != nil && created.IsDefault, tt.wantDefault)
			}
		})
	}
}
