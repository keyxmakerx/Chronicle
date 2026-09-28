// timeline_calendar_scope_integration_test.go checks the timeline's calendar
// binding against real SQL: calendarScopeAdapter over a real
// calendar.CalendarService lets a timeline store a calendar from its own
// campaign, through the create form's service call and campaign import
// alike, and refuses one from any other campaign before anything is written.
//
// Skipped under -short. Run with `make test-int-local`.
package app

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

func TestTimelineCalendarScope_DB(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTimelineTestDB(t)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	campA, ownerA := newCalRoundTripCampaign(t, db, "scope-a")
	campB, _ := newCalRoundTripCampaign(t, db, "scope-b")

	calSvc := calendar.NewCalendarService(calendar.NewCalendarRepository(db),
		calendar.NewEventRepository(db), calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	newCal := func(campaignID, name, visibility string) string {
		cal, err := calSvc.CreateCalendar(ctx, campaignID, calendar.CreateCalendarInput{
			Name: name, CurrentYear: 1, Visibility: visibility,
		})
		if err != nil {
			t.Fatalf("create calendar %q: %v", name, err)
		}
		return cal.ID
	}
	calA := newCal(campA, "Scope A", "everyone")
	calAHidden := newCal(campA, "Scope A GM", "dm_only")
	calB := newCal(campB, "Scope B", "everyone")

	tlSvc := timeline.NewTimelineService(timeline.NewTimelineRepository(db), nil, nil, nil)
	tlSvc.(interface {
		SetCalendarScope(timeline.CalendarScope)
	}).SetCalendarScope(&calendarScopeAdapter{svc: calSvc})

	storedWith := func(campaignID, calendarID string) int {
		t.Helper()
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM timelines WHERE campaign_id = ? AND calendar_id = ?`,
			campaignID, calendarID).Scan(&n); err != nil {
			t.Fatalf("count timelines: %v", err)
		}
		return n
	}

	for _, tc := range []struct {
		name       string
		calendarID string
		wantStored bool
	}{
		{"own campaign's calendar", calA, true},
		{"own campaign's GM-only calendar", calAHidden, true},
		{"another campaign's calendar", calB, false},
		{"unknown calendar", "00000000-0000-4000-8000-000000000000", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.calendarID
			tl, err := tlSvc.CreateTimeline(ctx, campA, timeline.CreateTimelineInput{
				CampaignID: campA, Name: "Saga: " + tc.name, CalendarID: &id, CreatedBy: ownerA,
			})
			if tc.wantStored {
				if err != nil {
					t.Fatalf("create timeline: %v", err)
				}
				got, err := tlSvc.GetTimeline(ctx, tl.ID)
				if err != nil {
					t.Fatalf("read timeline back: %v", err)
				}
				if got.CalendarID == nil || *got.CalendarID != id {
					t.Errorf("stored calendar_id = %v, want %s", got.CalendarID, id)
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
				t.Fatalf("err = %v, want a 404", err)
			}
			if n := storedWith(campA, id); n != 0 {
				t.Errorf("%d timeline(s) in campaign A stored calendar %s", n, id)
			}
		})
	}

	// Campaign import links its timelines to the calendar it just imported
	// into the same campaign.
	idMap := campaigns.NewIDMap(campA)
	idMap.CalendarID = calA
	report := campaigns.NewImportReport()
	before := storedWith(campA, calA)
	if err := (&timelineImportAdapter{svc: tlSvc}).ImportTimelines(ctx, campA, ownerA,
		[]campaigns.ExportTimeline{{Name: "Imported saga", Visibility: "everyone", ZoomDefault: "year"}},
		idMap, report); err != nil {
		t.Fatalf("import timelines: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("timeline import reported failures: %s", report.Summary())
	}
	if n := storedWith(campA, calA); n != before+1 {
		t.Errorf("imported timeline not linked to the imported calendar: %d linked, want %d", n, before+1)
	}
}
