package calendar

import (
	"context"
	"errors"
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

// fakeDefaultStore records which calendar each campaign's default went to.
type fakeDefaultStore struct {
	missing []string
	cals    map[string][]Calendar
	failOn  string
	set     map[string]string
}

func (f *fakeDefaultStore) ListCampaignsWithoutDefault(context.Context) ([]string, error) {
	return f.missing, nil
}
func (f *fakeDefaultStore) ListByCampaignID(_ context.Context, id string) ([]Calendar, error) {
	if id == f.failOn {
		return nil, errors.New("boom")
	}
	return f.cals[id], nil
}
func (f *fakeDefaultStore) SetDefault(_ context.Context, campaignID, calendarID string) error {
	f.set[campaignID] = calendarID
	return nil
}

func TestReconcileDefaultCalendars(t *testing.T) {
	store := &fakeDefaultStore{
		missing: []string{"camp-a", "camp-broken", "camp-empty"},
		cals: map[string][]Calendar{
			"camp-a": {{ID: "first"}, {ID: "second"}},
		},
		failOn: "camp-broken",
		set:    map[string]string{},
	}
	n, err := ReconcileDefaultCalendars(context.Background(), store)
	if err != nil {
		t.Fatalf("ReconcileDefaultCalendars: %v", err)
	}
	// camp-empty counts: nothing to mark is not a failure.
	if n != 2 {
		t.Errorf("repaired = %d, want 2", n)
	}
	if store.set["camp-a"] != "first" {
		t.Errorf("camp-a default = %q, want the first calendar", store.set["camp-a"])
	}
	if _, ok := store.set["camp-broken"]; ok {
		t.Error("a campaign whose read failed should be skipped")
	}
}

// Deleting the default calendar hands the mark to the next one; deleting
// another calendar leaves the default alone.
func TestDeleteCalendar_HandsOnDefault(t *testing.T) {
	tests := []struct {
		name      string
		deleted   Calendar
		wantNewTo string
	}{
		{"deleting the default", Calendar{ID: "cal-1", CampaignID: testCampaignA, IsDefault: true}, "cal-2"},
		{"deleting another calendar", Calendar{ID: "cal-3", CampaignID: testCampaignA}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var newDefault string
			repo := &fakeCalendarRepo{
				getByIDFn: func(context.Context, string) (*Calendar, error) { c := tt.deleted; return &c, nil },
				listByCampaignFn: func(context.Context, string) ([]Calendar, error) {
					return []Calendar{{ID: "cal-2", CampaignID: testCampaignA}}, nil
				},
				setDefaultFn: func(_ context.Context, _, calendarID string) error { newDefault = calendarID; return nil },
			}
			svc := newTestCalendarService(repo, nil, nil, nil)
			if err := svc.DeleteCalendar(context.Background(), tt.deleted.ID, testCampaignA); err != nil {
				t.Fatalf("DeleteCalendar: %v", err)
			}
			if newDefault != tt.wantNewTo {
				t.Errorf("new default = %q, want %q", newDefault, tt.wantNewTo)
			}
		})
	}
}
