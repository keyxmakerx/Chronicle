// export_timeline_links_roundtrip_integration_test.go round-trips timelines
// against MariaDB with the real calendar and timeline services: a timeline
// keeps the calendar it drew on (not just the campaign's default) and the
// calendar events it showed, with its own label and visibility for each; a
// timeline with no calendar comes back with none.
//
// Skipped under -short. Run with `make test-int-local`.
package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

func TestTimelineCalendarLinks_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTimelineTestDB(t)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	srcCampaignID, srcOwnerID := newCalRoundTripCampaign(t, db, "tl-src")
	dstCampaignID, dstOwnerID := newCalRoundTripCampaign(t, db, "tl-dst")

	calSvc := calendar.NewCalendarService(calendar.NewCalendarRepository(db), calendar.NewEventRepository(db),
		calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	tlSvc := timeline.NewTimelineService(timeline.NewTimelineRepository(db),
		&calendarListerAdapter{svc: calSvc}, &calendarEventListerAdapter{svc: calSvc}, &calendarEraListerAdapter{svc: calSvc})
	tlSvc.(interface {
		SetCalendarEventLinkLister(timeline.CalendarEventLinkLister)
	}).SetCalendarEventLinkLister(&calendarEventLinkListerAdapter{svc: calSvc})
	tlSvc.(interface {
		SetCalendarScope(timeline.CalendarScope)
	}).SetCalendarScope(&calendarScopeAdapter{svc: calSvc})
	viewer := permissions.SystemViewer(3)

	// Two calendars; the timeline draws on the second, not the default.
	mkCal := func(name string) *calendar.Calendar {
		c, err := calSvc.CreateCalendar(ctx, srcCampaignID, calendar.CreateCalendarInput{Name: name, CurrentYear: 1})
		if err != nil {
			t.Fatalf("create calendar %s: %v", name, err)
		}
		if err := calSvc.SetMonths(ctx, c.ID, srcCampaignID, []calendar.MonthInput{{Name: "One", Days: 30}}); err != nil {
			t.Fatalf("set months: %v", err)
		}
		return c
	}
	main := mkCal("Main Calendar")
	if err := calSvc.SetDefaultCalendar(ctx, srcCampaignID, main.ID); err != nil {
		t.Fatalf("set default: %v", err)
	}
	moon := mkCal("Moon Calendar")
	evt, err := calSvc.CreateEvent(ctx, moon.ID, srcCampaignID, calendar.CreateEventInput{
		Name: "Eclipse", Year: 1, Month: 1, Day: 5, Visibility: "everyone", CanAuthorDmOnly: true,
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	linked, err := tlSvc.CreateTimeline(ctx, srcCampaignID, timeline.CreateTimelineInput{
		CampaignID: srcCampaignID, CalendarID: &moon.ID, Name: "Sky Watch", Visibility: "everyone", CreatedBy: srcOwnerID,
	})
	if err != nil {
		t.Fatalf("create linked timeline: %v", err)
	}
	label, dmOnly := "The long dark", "dm_only"
	if _, err := tlSvc.LinkEvent(ctx, linked.ID, evt.ID, timeline.LinkEventInput{Label: &label}); err != nil {
		t.Fatalf("link event: %v", err)
	}
	if err := tlSvc.UpdateEventLinkVisibility(ctx, linked.ID, evt.ID, timeline.UpdateEventVisibilityInput{VisibilityOverride: &dmOnly}); err != nil {
		t.Fatalf("link visibility: %v", err)
	}
	if _, err := tlSvc.CreateTimeline(ctx, srcCampaignID, timeline.CreateTimelineInput{
		CampaignID: srcCampaignID, Name: "Family Tree", Visibility: "everyone", CreatedBy: srcOwnerID,
	}); err != nil {
		t.Fatalf("create calendar-free timeline: %v", err)
	}

	// Export both sections and send them through JSON, as a file would.
	calData, err := (&calendarExportAdapter{svc: calSvc}).ExportCalendar(ctx, srcCampaignID, func(string) string { return "" })
	if err != nil {
		t.Fatalf("export calendar: %v", err)
	}
	tlData, err := (&timelineExportAdapter{svc: tlSvc}).ExportTimelines(ctx, srcCampaignID, func(string) string { return "" })
	if err != nil {
		t.Fatalf("export timelines: %v", err)
	}
	raw, err := json.Marshal(campaigns.CampaignExport{Calendar: calData, Timelines: tlData})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var file campaigns.CampaignExport
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	idMap := campaigns.NewIDMap(dstCampaignID)
	report := campaigns.NewImportReport()
	if err := (&calendarImportAdapter{svc: calSvc}).ImportCalendar(ctx, dstCampaignID, file.Calendar, idMap, report); err != nil {
		t.Fatalf("import calendar: %v", err)
	}
	if err := (&timelineImportAdapter{svc: tlSvc}).ImportTimelines(ctx, dstCampaignID, dstOwnerID, file.Timelines, idMap, report); err != nil {
		t.Fatalf("import timelines: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("import reported losses: %s", report.Summary())
	}

	got, err := tlSvc.ListTimelines(ctx, dstCampaignID, viewer)
	if err != nil {
		t.Fatalf("list imported timelines: %v", err)
	}
	byName := map[string]timeline.Timeline{}
	for _, tl := range got {
		byName[tl.Name] = tl
	}
	sky, ok := byName["Sky Watch"]
	if !ok || !sky.HasCalendar() {
		t.Fatalf("Sky Watch came back without a calendar: %+v", sky)
	}
	cal, err := calSvc.GetCalendarForViewer(ctx, *sky.CalendarID, dstCampaignID, viewer)
	if err != nil || cal.Name != "Moon Calendar" {
		t.Errorf("Sky Watch draws on %+v (err %v), want the restored Moon Calendar", cal, err)
	}
	if fam, ok := byName["Family Tree"]; !ok || fam.HasCalendar() {
		t.Errorf("Family Tree came back bound to a calendar: %+v", fam)
	}

	links, err := tlSvc.ListTimelineEvents(ctx, sky.ID, dstCampaignID, viewer)
	if err != nil {
		t.Fatalf("list Sky Watch events: %v", err)
	}
	var found bool
	for _, l := range links {
		if l.Source == "standalone" || l.EventName != "Eclipse" {
			continue
		}
		found = true
		if l.Label == nil || *l.Label != label {
			t.Errorf("link label = %v, want %q", l.Label, label)
		}
		if l.VisibilityOverride == nil || *l.VisibilityOverride != dmOnly {
			t.Errorf("link visibility override = %v, want dm_only", l.VisibilityOverride)
		}
	}
	if !found {
		t.Errorf("Sky Watch lost its Eclipse link: %+v", links)
	}
}
