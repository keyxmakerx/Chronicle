package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// fakeEraVersions keeps versions in memory, keyed by page id.
type fakeEraVersions struct {
	calendar.EraVersionService
	byEra map[int][]*calendar.EntityEraVersion
	saved map[string]*calendar.EntityEraVersion
}

func (f *fakeEraVersions) ForEra(_ context.Context, eraID int) ([]*calendar.EntityEraVersion, error) {
	return f.byEra[eraID], nil
}
func (f *fakeEraVersions) Create(_ context.Context, _, entityID string, eraID int, _ string) error {
	f.saved[entityID] = &calendar.EntityEraVersion{EntityID: entityID, EraID: eraID}
	return nil
}
func (f *fakeEraVersions) SetIdentity(_ context.Context, _, entityID string, _ int, name, _ patch.Field[string], _ string) error {
	f.saved[entityID].Name = name.Ptr(nil)
	return nil
}
func (f *fakeEraVersions) SaveEntry(_ context.Context, _, entityID string, _ int, entry, html string, _ *int, _ string) (int, *calendar.EraEntryConflict, error) {
	f.saved[entityID].Entry, f.saved[entityID].EntryHTML = &entry, &html
	return 1, nil, nil
}
func (f *fakeEraVersions) SaveFields(_ context.Context, _, entityID string, _ int, changed map[string]any, _ string) error {
	f.saved[entityID].FieldsData = changed
	return nil
}

func TestEraPageVersionsRoundTrip(t *testing.T) {
	s := func(x string) *string { return &x }
	src := &fakeEraVersions{byEra: map[int][]*calendar.EntityEraVersion{
		7: {
			{EntityID: "old-1", Name: s("Ashgate"), Entry: s(`{"type":"doc"}`), EntryHTML: s("<p>Walls</p>"), FieldsData: map[string]any{"ruler": "Council"}},
			{EntityID: "gone", Name: s("Lost")},
		},
	}}
	slugs := map[string]string{"old-1": "emberfall"}
	out := (&calendarExportAdapter{versions: src}).exportPageVersions(context.Background(), 7, func(id string) string { return slugs[id] })
	if len(out) != 1 || out[0].EntitySlug != "emberfall" {
		t.Fatalf("export = %+v, want only the page that has a slug", out)
	}

	dst := &fakeEraVersions{saved: map[string]*calendar.EntityEraVersion{}}
	idMap := campaigns.NewIDMap("new")
	idMap.EntitySlugToID["emberfall"] = "new-1"
	report := campaigns.NewImportReport()
	(&calendarImportAdapter{versions: dst}).importPageVersions(context.Background(), "new", 9,
		campaigns.ExportCalendarEra{Name: "Ash", PageVersions: append(out, campaigns.ExportEraPageVersion{EntitySlug: "missing"})}, idMap, report)
	got := dst.saved["new-1"]
	if got == nil || got.EraID != 9 || *got.Name != "Ashgate" || *got.EntryHTML != "<p>Walls</p>" || got.FieldsData["ruler"] != "Council" {
		t.Fatalf("restored = %+v", got)
	}
	if len(dst.saved) != 1 {
		t.Errorf("a version whose page was not restored was written: %v", dst.saved)
	}
}
