// import_icons_test.go pins how a campaign import handles a bad icon on
// entity types, timelines, map markers and sidebar items: the icon is
// cleared to "" for the owning service's own default to apply, the object
// itself still imports, and the drop is recorded in the ImportReport. A
// valid icon is left untouched and not reported; an already-empty icon is
// left empty and not reported either.
package campaigns

import (
	"testing"
)

// badImportIconNames are inputs that must fail sanitize.ValidateIcon.
var badImportIconNames = []string{
	`fa-x" onmouseover="y`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func TestNormalizeImportIcons_EntityTypes(t *testing.T) {
	for _, icon := range badImportIconNames {
		t.Run("invalid/"+icon, func(t *testing.T) {
			data := &CampaignExport{
				EntityTypes: []ExportEntityType{{Name: "Beast", Icon: icon}},
			}
			report := NewImportReport()
			normalizeImportIcons(data, report)

			if data.EntityTypes[0].Icon != "" {
				t.Errorf("expected icon to be cleared, got %q", data.EntityTypes[0].Icon)
			}
			if report.Count() != 1 {
				t.Errorf("expected 1 reported failure, got %d", report.Count())
			}
			failures := report.Failures()
			if len(failures) != 1 || failures[0].Kind != kindIcon {
				t.Errorf("expected a failure row with kind %q, got %+v", kindIcon, failures)
			}
		})
	}

	t.Run("valid icon untouched and not reported", func(t *testing.T) {
		data := &CampaignExport{
			EntityTypes: []ExportEntityType{{Name: "Beast", Icon: "fa-dragon"}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.EntityTypes[0].Icon != "fa-dragon" {
			t.Errorf("expected icon to stay fa-dragon, got %q", data.EntityTypes[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})

	t.Run("empty icon stays empty and not reported", func(t *testing.T) {
		data := &CampaignExport{
			EntityTypes: []ExportEntityType{{Name: "Beast", Icon: ""}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.EntityTypes[0].Icon != "" {
			t.Errorf("expected icon to stay empty, got %q", data.EntityTypes[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})
}

func TestNormalizeImportIcons_Timelines(t *testing.T) {
	t.Run("invalid icon cleared and reported", func(t *testing.T) {
		data := &CampaignExport{
			Timelines: []ExportTimeline{{Name: "Main Timeline", Icon: `<b>`}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.Timelines[0].Icon != "" {
			t.Errorf("expected icon to be cleared, got %q", data.Timelines[0].Icon)
		}
		if report.Count() != 1 {
			t.Errorf("expected 1 reported failure, got %d", report.Count())
		}
		failures := report.Failures()
		if len(failures) != 1 || failures[0].Section != SectionTimelines || failures[0].Kind != kindIcon {
			t.Errorf("expected a timelines/icon failure row, got %+v", failures)
		}
	})

	t.Run("valid icon untouched and not reported", func(t *testing.T) {
		data := &CampaignExport{
			Timelines: []ExportTimeline{{Name: "Main Timeline", Icon: "fa-timeline"}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.Timelines[0].Icon != "fa-timeline" {
			t.Errorf("expected icon to stay fa-timeline, got %q", data.Timelines[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})

	t.Run("empty icon stays empty and not reported", func(t *testing.T) {
		data := &CampaignExport{
			Timelines: []ExportTimeline{{Name: "Main Timeline", Icon: ""}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.Timelines[0].Icon != "" {
			t.Errorf("expected icon to stay empty, got %q", data.Timelines[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})
}

func TestNormalizeImportIcons_MapMarkers(t *testing.T) {
	t.Run("invalid icon cleared and reported", func(t *testing.T) {
		data := &CampaignExport{
			Maps: []ExportMap{{
				Name:    "World Map",
				Markers: []ExportMarker{{Name: "Castle", Icon: "FA-BOOK"}},
			}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.Maps[0].Markers[0].Icon != "" {
			t.Errorf("expected icon to be cleared, got %q", data.Maps[0].Markers[0].Icon)
		}
		if report.Count() != 1 {
			t.Errorf("expected 1 reported failure, got %d", report.Count())
		}
		failures := report.Failures()
		if len(failures) != 1 || failures[0].Section != "maps" || failures[0].Kind != kindIcon {
			t.Errorf("expected a maps/icon failure row, got %+v", failures)
		}
	})

	t.Run("valid icon untouched and not reported", func(t *testing.T) {
		data := &CampaignExport{
			Maps: []ExportMap{{
				Name:    "World Map",
				Markers: []ExportMarker{{Name: "Castle", Icon: "fa-dragon"}},
			}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.Maps[0].Markers[0].Icon != "fa-dragon" {
			t.Errorf("expected icon to stay fa-dragon, got %q", data.Maps[0].Markers[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})

	t.Run("empty icon stays empty and not reported", func(t *testing.T) {
		data := &CampaignExport{
			Maps: []ExportMap{{
				Name:    "World Map",
				Markers: []ExportMarker{{Name: "Castle", Icon: ""}},
			}},
		}
		report := NewImportReport()
		normalizeImportIcons(data, report)

		if data.Maps[0].Markers[0].Icon != "" {
			t.Errorf("expected icon to stay empty, got %q", data.Maps[0].Markers[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})
}

func TestNormalizeImportIcons_NilData(t *testing.T) {
	// Must not panic when there is nothing to normalize.
	report := NewImportReport()
	normalizeImportIcons(nil, report)
	if report.Count() != 0 {
		t.Errorf("expected no reported failures, got %d", report.Count())
	}
}

func TestNormalizeSidebarIcons(t *testing.T) {
	t.Run("invalid icon cleared and reported under item label", func(t *testing.T) {
		items := []SidebarItem{{Type: "link", Label: "Quest Board", Icon: `fa-x" onmouseover="y`}}
		report := NewImportReport()
		normalizeSidebarIcons(items, "My Campaign", report)

		if items[0].Icon != "" {
			t.Errorf("expected icon to be cleared, got %q", items[0].Icon)
		}
		if report.Count() != 1 {
			t.Errorf("expected 1 reported failure, got %d", report.Count())
		}
		failures := report.Failures()
		if len(failures) != 1 || failures[0].Name != "Quest Board" || failures[0].Kind != kindIcon {
			t.Errorf("expected a campaign/icon failure row naming the item, got %+v", failures)
		}
	})

	t.Run("invalid icon on unlabeled item falls back to campaign name", func(t *testing.T) {
		items := []SidebarItem{{Type: "link", Icon: "fa-"}}
		report := NewImportReport()
		normalizeSidebarIcons(items, "My Campaign", report)

		failures := report.Failures()
		if len(failures) != 1 || failures[0].Name != "My Campaign" {
			t.Errorf("expected the failure to be named after the campaign, got %+v", failures)
		}
	})

	t.Run("valid icon untouched and not reported", func(t *testing.T) {
		items := []SidebarItem{{Type: "link", Label: "Quest Board", Icon: "fa-dragon"}}
		report := NewImportReport()
		normalizeSidebarIcons(items, "My Campaign", report)

		if items[0].Icon != "fa-dragon" {
			t.Errorf("expected icon to stay fa-dragon, got %q", items[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})

	t.Run("empty icon stays empty and not reported", func(t *testing.T) {
		items := []SidebarItem{{Type: "link", Label: "Quest Board", Icon: ""}}
		report := NewImportReport()
		normalizeSidebarIcons(items, "My Campaign", report)

		if items[0].Icon != "" {
			t.Errorf("expected icon to stay empty, got %q", items[0].Icon)
		}
		if report.Count() != 0 {
			t.Errorf("expected no reported failures, got %d", report.Count())
		}
	})
}
