// export_import_events_test.go pins #779: a Chronicle export that includes
// events, re-imported through the same DetectAndParse path an upload or a
// preset goes through, must carry those events back — parseChronicle's own
// doc comment already promised "round-trips perfectly", which was only true
// of calendar structure before this fix.
package calendar

import (
	"encoding/json"
	"testing"
)

func TestChronicleExportImport_EventsRoundTrip(t *testing.T) {
	festivalSlug := "festival"
	cal := &Calendar{
		Name:             "Round Trip Calendar",
		Mode:             ModeFantasy,
		CurrentYear:      100,
		CurrentMonth:     3,
		CurrentDay:       12,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
		Months:           []Month{{Name: "Firstmonth", Days: 30, SortOrder: 0}},
	}
	events := []Event{
		{Name: "Founding Day", Year: 50, Month: 1, Day: 1, Visibility: "everyone", KindSlug: festivalSlug, AllDay: true},
		{Name: "Secret Meeting", Year: 99, Month: 6, Day: 15, Visibility: "dm_only"},
	}

	export := BuildExport(cal, events, true)
	if len(export.Events) != 2 {
		t.Fatalf("BuildExport produced %d events, want 2", len(export.Events))
	}
	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Format != FormatChronicle {
		t.Fatalf("format = %q, want %q", ir.Format, FormatChronicle)
	}
	if len(ir.Events) != len(events) {
		t.Fatalf("got %d imported events, want %d: %+v", len(ir.Events), len(events), ir.Events)
	}

	got := ir.Events[0]
	if got.Name != "Founding Day" || got.Year != 50 || got.Month != 1 || got.Day != 1 || got.Visibility != "everyone" || !got.AllDay {
		t.Errorf("event 0 = %+v, want the Founding Day fields preserved", got)
	}
	if got.Kind == nil || *got.Kind != festivalSlug {
		t.Errorf("event 0 kind = %v, want %q", got.Kind, festivalSlug)
	}

	got1 := ir.Events[1]
	if got1.Name != "Secret Meeting" || got1.Visibility != "dm_only" {
		t.Errorf("event 1 = %+v, want the Secret Meeting fields preserved", got1)
	}
	if got1.Kind != nil {
		t.Errorf("event 1 kind = %v, want nil (no kind slug on export)", got1.Kind)
	}

	// The calendar's own current date round-trips into ir.Today too, so
	// CreateCalendarFromImport can recreate it without any override.
	if ir.Today.Year != 100 || ir.Today.Month == nil || *ir.Today.Month != 3 || ir.Today.Day == nil || *ir.Today.Day != 12 {
		t.Errorf("ir.Today = %+v, want {100, 3, 12}", ir.Today)
	}
}

// TestChronicleImport_ModeRoundTrips pins a related gap this PR closes
// alongside #779: ImportedSettings previously carried no Mode field at all,
// so CreateCalendarFromImport's CreateCalendar call always defaulted to
// ModeFantasy — a Chronicle export of a REALLIFE calendar would silently
// become a fantasy one on re-import. External formats (Simple Calendar,
// Calendaria, Fantasy-Calendar) have no Gregorian/real-time concept, so
// they correctly leave Settings.Mode empty (defaulting to fantasy); only
// Chronicle's own export carries a real Mode to round-trip.
func TestChronicleImport_ModeRoundTrips(t *testing.T) {
	cal := &Calendar{
		Name: "Real World Calendar", Mode: ModeRealLife,
		CurrentYear: 2024, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	raw, err := json.Marshal(BuildExport(cal, nil, false))
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Settings.Mode != ModeRealLife {
		t.Errorf("Settings.Mode = %q, want %q", ir.Settings.Mode, ModeRealLife)
	}
}

// TestChronicleExportImport_NoEventsRoundTripsToNil confirms an export with
// no events (includeEvents=false, or a calendar with none) leaves
// ImportResult.Events empty rather than a slice of zero-valued events.
func TestChronicleExportImport_NoEventsRoundTripsToNil(t *testing.T) {
	cal := &Calendar{
		Name: "Structure Only", Mode: ModeFantasy,
		CurrentYear: 1, CurrentMonth: 1, CurrentDay: 1,
		HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	export := BuildExport(cal, nil, true)
	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if len(ir.Events) != 0 {
		t.Errorf("got %d events from an export with none, want 0: %+v", len(ir.Events), ir.Events)
	}
}
