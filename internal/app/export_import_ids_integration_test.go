// export_import_ids_integration_test.go round-trips what a campaign names by
// id (its sidebar and each category's pinned pages) and a timeline's own
// events against MariaDB, through the real services.
//
// Skipped under -short. Run with `make test-int-local`.
package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/timeline"
)

// TestCampaignExportImport_SidebarAndPins_DBRoundTrip: an owner's sidebar
// comes back in their order, with the category they hid still hidden and
// the page they hid still hidden, all pointing at the new campaign's
// categories and pages; a category's pinned page is the restored copy.
func TestCampaignExportImport_SidebarAndPins_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()
	owner := h.newUser("owner")
	src, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Sidebar Source"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	types, err := h.entities.GetEntityTypes(ctx, src.ID)
	if err != nil || len(types) < 3 {
		t.Fatalf("source categories = %d (err %v), want at least 3", len(types), err)
	}
	page, err := h.entities.Create(ctx, src.ID, owner, entities.CreateEntityInput{Name: "Secret Cave", EntityTypeID: types[0].ID})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	if err := h.entities.UpdateEntityTypeDashboard(ctx, types[0].ID, nil, []string{page.ID}); err != nil {
		t.Fatalf("pin page: %v", err)
	}

	// Reverse the category order and hide the second-to-last one.
	var items []campaigns.SidebarItem
	var wantSlugs []string
	for i := len(types) - 1; i >= 0; i-- {
		items = append(items, campaigns.SidebarItem{Type: campaigns.SidebarTypeCategory, TypeID: types[i].ID, Visible: i != 1})
		wantSlugs = append(wantSlugs, types[i].Slug)
	}
	hidden := []string{page.ID}
	if err := h.campaigns.UpdateSidebarConfig(ctx, src.ID, campaigns.UpdateSidebarConfigRequest{Items: &items, HiddenEntityIDs: &hidden}); err != nil {
		t.Fatalf("save sidebar: %v", err)
	}
	src, err = h.campaigns.GetByID(ctx, src.ID)
	if err != nil {
		t.Fatalf("reload source: %v", err)
	}

	newID, body := h.importBlob(h.newUser("importer"), "campaign.json", h.export(src, owner, "", "application/json"))
	if strings.Contains(body, "Imported with losses") {
		t.Fatalf("round trip reported losses:\n%s", body)
	}

	cfg, err := h.campaigns.GetSidebarConfig(ctx, newID)
	if err != nil {
		t.Fatalf("imported sidebar: %v", err)
	}
	var gotSlugs []string
	for _, it := range cfg.Items {
		if it.Type != campaigns.SidebarTypeCategory {
			continue
		}
		var slug, campaignID string
		if err := h.db.QueryRow(`SELECT slug, campaign_id FROM entity_types WHERE id = ?`, it.TypeID).Scan(&slug, &campaignID); err != nil {
			t.Fatalf("sidebar category %d names no category: %v", it.TypeID, err)
		}
		if campaignID != newID {
			t.Errorf("sidebar category %q belongs to campaign %s, want the imported %s", slug, campaignID, newID)
		}
		if want := slug != types[1].Slug; it.Visible != want {
			t.Errorf("sidebar category %q visible = %v, want %v", slug, it.Visible, want)
		}
		gotSlugs = append(gotSlugs, slug)
	}
	if strings.Join(gotSlugs, ",") != strings.Join(wantSlugs, ",") {
		t.Errorf("sidebar category order = %v, want %v", gotSlugs, wantSlugs)
	}

	var newPage string
	if err := h.db.QueryRow(`SELECT id FROM entities WHERE campaign_id = ? AND name = 'Secret Cave'`, newID).Scan(&newPage); err != nil {
		t.Fatalf("imported page: %v", err)
	}
	if len(cfg.HiddenEntityIDs) != 1 || cfg.HiddenEntityIDs[0] != newPage {
		t.Errorf("hidden pages = %v, want [%s]", cfg.HiddenEntityIDs, newPage)
	}
	newTypes, err := h.entities.GetEntityTypes(ctx, newID)
	if err != nil {
		t.Fatalf("imported categories: %v", err)
	}
	for _, nt := range newTypes {
		if nt.Slug != types[0].Slug {
			continue
		}
		if len(nt.PinnedEntityIDs) != 1 || nt.PinnedEntityIDs[0] != newPage {
			t.Errorf("%s pinned pages = %v, want [%s]", nt.Slug, nt.PinnedEntityIDs, newPage)
		}
	}
}

// TestTimelineStandaloneEvent_DBRoundTrip: a timeline's own event keeps its
// text, times of day and repeat through an export and import.
func TestTimelineStandaloneEvent_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTimelineTestDB(t)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	srcCampaignID, srcOwnerID := newCalRoundTripCampaign(t, db, "tle-src")
	dstCampaignID, dstOwnerID := newCalRoundTripCampaign(t, db, "tle-dst")
	calSvc := calendar.NewCalendarService(calendar.NewCalendarRepository(db), calendar.NewEventRepository(db),
		calendar.NewEventKindRepository(db), calendar.NewWeatherRepository(db))
	tlSvc := timeline.NewTimelineService(timeline.NewTimelineRepository(db),
		&calendarListerAdapter{svc: calSvc}, &calendarEventListerAdapter{svc: calSvc}, &calendarEraListerAdapter{svc: calSvc})

	tl, err := tlSvc.CreateTimeline(ctx, srcCampaignID, timeline.CreateTimelineInput{
		CampaignID: srcCampaignID, Name: "Festival Days", Visibility: "everyone", CreatedBy: srcOwnerID,
	})
	if err != nil {
		t.Fatalf("create timeline: %v", err)
	}
	ip := func(n int) *int { return &n }
	sp := func(s string) *string { return &s }
	want := timeline.CreateTimelineEventInput{
		Name: "Lantern Night", Description: sp("Lanterns float down the river."),
		DescriptionHTML: sp("<p>Lanterns float down the <strong>river</strong>.</p>"),
		Year: 12, Month: 3, Day: 4, StartHour: ip(19), StartMinute: ip(30),
		EndYear: ip(12), EndMonth: ip(3), EndDay: ip(4), EndHour: ip(23), EndMinute: ip(15),
		IsRecurring: true, RecurrenceType: sp("yearly"), Visibility: "everyone", CreatedBy: srcOwnerID,
	}
	if _, err := tlSvc.CreateStandaloneEvent(ctx, tl.ID, want); err != nil {
		t.Fatalf("create event: %v", err)
	}

	tlData, err := (&timelineExportAdapter{svc: tlSvc}).ExportTimelines(ctx, srcCampaignID, func(string) string { return "" })
	if err != nil {
		t.Fatalf("export timelines: %v", err)
	}
	raw, err := json.Marshal(campaigns.CampaignExport{Timelines: tlData})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var file campaigns.CampaignExport
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	report := campaigns.NewImportReport()
	if err := (&timelineImportAdapter{svc: tlSvc}).ImportTimelines(ctx, dstCampaignID, dstOwnerID, file.Timelines, campaigns.NewIDMap(dstCampaignID), report); err != nil {
		t.Fatalf("import timelines: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("import reported losses: %s", report.Summary())
	}

	got, err := tlSvc.ListTimelines(ctx, dstCampaignID, permissions.SystemViewer(3))
	if err != nil || len(got) != 1 {
		t.Fatalf("imported timelines = %d (err %v), want 1", len(got), err)
	}
	links, err := tlSvc.ListTimelineEvents(ctx, got[0].ID, dstCampaignID, permissions.SystemViewer(3))
	if err != nil || len(links) != 1 {
		t.Fatalf("imported events = %d (err %v), want 1", len(links), err)
	}
	evt, err := tlSvc.GetStandaloneEvent(ctx, links[0].EventID)
	if err != nil {
		t.Fatalf("imported event: %v", err)
	}
	str := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}
	num := func(p *int) int {
		if p == nil {
			return -1
		}
		return *p
	}
	checks := []struct {
		field     string
		got, want any
	}{
		{"description", str(evt.Description), str(want.Description)},
		{"description_html", str(evt.DescriptionHTML), str(want.DescriptionHTML)},
		{"start_hour", num(evt.StartHour), 19},
		{"start_minute", num(evt.StartMinute), 30},
		{"end_hour", num(evt.EndHour), 23},
		{"end_minute", num(evt.EndMinute), 15},
		{"is_recurring", evt.IsRecurring, true},
		{"recurrence_type", str(evt.RecurrenceType), "yearly"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
		}
	}
}
