// event_secret_era_write_test.go: a writer who cannot read back a secret-era
// event must be refused the write with a message that does not reveal why.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestEventWriteInSecretEra(t *testing.T) {
	cal, eras, events := eraFixture()
	newSvc := func() CalendarService {
		calRepo := &fakeCalendarRepo{
			getByIDFn:   func(context.Context, string) (*Calendar, error) { c := cal; return &c, nil },
			getMonthsFn: func(context.Context, string) ([]Month, error) { return eraMonths(), nil },
			getErasFn:   func(context.Context, string) ([]Era, error) { return append([]Era(nil), eras...), nil },
		}
		eventRepo := &fakeEventRepo{
			getEventFn: func(_ context.Context, id string) (*Event, error) {
				for _, e := range events {
					if e.ID == id {
						ev := e
						return &ev, nil
					}
				}
				return nil, nil
			},
		}
		return NewCalendarService(calRepo, eventRepo, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	}
	scribe := permissions.RequestViewer(int(campaigns.RoleScribe), "u-scribe")
	owner := permissions.RequestViewer(int(campaigns.RoleOwner), "u-owner")

	tests := []struct {
		name    string
		v       permissions.Viewer
		year    int
		update  bool
		refused bool
	}{
		{"scribe create inside secret era", scribe, 1040, false, true},
		{"scribe create outside", scribe, 1022, false, false},
		{"owner create inside secret era", owner, 1040, false, false},
		{"scribe update moves into secret era", scribe, 1040, true, true},
		{"scribe update stays outside", scribe, 1023, true, false},
		{"owner update moves into secret era", owner, 1040, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newSvc()
			var err error
			if tt.update {
				err = svc.UpdateEvent(context.Background(), "ev-visible", eraCalendarID, eraCampaignID,
					UpdateEventInput{Year: patch.Of(tt.year)}, tt.v)
			} else {
				_, err = svc.CreateEvent(context.Background(), eraCalendarID, eraCampaignID, CreateEventInput{
					Name: "Meeting", Year: tt.year, Month: 3, Day: 4, CanAuthorDmOnly: tt.v.SkipsPerUserRules(), Author: tt.v,
				})
			}
			if !tt.refused {
				if err != nil {
					t.Fatalf("want ok, got %v", err)
				}
				return
			}
			ae, ok := err.(*apperror.AppError)
			if !ok || ae.Message != "That date can't be used." {
				t.Fatalf("want neutral validation error, got %#v", err)
			}
		})
	}
}
