// import_festivals_cycles_test.go pins #771: a Chronicle export that
// includes cycles and festivals, re-imported through the same DetectAndParse
// path an upload or a preset goes through, must carry those two
// sub-resources back — parseChronicle's own doc comment already promised
// "round-trips perfectly", which was only true of months/weekdays/moons/
// seasons/eras before this fix. Calendaria's own festivals (a separate
// format, parsed into calData.Festivals and then never read) get their own
// test below.
package calendar

import (
	"encoding/json"
	"testing"
)

func TestChronicleExportImport_CyclesAndFestivalsRoundTrip(t *testing.T) {
	icon := "star"
	after := 12
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
		Cycles: []Cycle{
			{
				Name: "Zodiac", CycleLength: 12, Type: "yearly", SortOrder: 0,
				Entries: []CycleEntry{
					{Name: "Rat", Icon: &icon, YearOffset: 0, SortOrder: 0},
					{Name: "Ox", YearOffset: 1, SortOrder: 1},
				},
			},
		},
		Festivals: []Festival{
			{Name: "Midwinter", Month: intPtr(1), Day: intPtr(1), Color: strPtr("#ff0000"), Icon: &icon, SortOrder: 0},
			{Name: "Yearless Day", AfterMonth: &after, Description: strPtr("Falls between years"), SortOrder: 1},
		},
	}

	export := BuildExport(cal, nil, false)
	if len(export.Calendar.Cycles) != 1 || len(export.Calendar.Cycles[0].Entries) != 2 {
		t.Fatalf("BuildExport lost the cycle or its entries: %+v", export.Calendar.Cycles)
	}
	if len(export.Calendar.Festivals) != 2 {
		t.Fatalf("BuildExport produced %d festivals, want 2", len(export.Calendar.Festivals))
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

	if len(ir.Cycles) != 1 {
		t.Fatalf("imported %d cycles, want 1 — cycles were dropped on import", len(ir.Cycles))
	}
	gotCycle := ir.Cycles[0]
	if gotCycle.Name != "Zodiac" || gotCycle.CycleLength != 12 || gotCycle.Type != "yearly" {
		t.Errorf("cycle = %+v, want Zodiac/12/yearly", gotCycle)
	}
	if len(gotCycle.Entries) != 2 || gotCycle.Entries[0].Name != "Rat" || gotCycle.Entries[0].Icon == nil || *gotCycle.Entries[0].Icon != icon {
		t.Errorf("cycle entries = %+v, want Rat (with icon) then Ox", gotCycle.Entries)
	}
	if gotCycle.Entries[1].Name != "Ox" || gotCycle.Entries[1].YearOffset != 1 {
		t.Errorf("second cycle entry = %+v, want Ox at year_offset 1", gotCycle.Entries[1])
	}

	if len(ir.Festivals) != 2 {
		t.Fatalf("imported %d festivals, want 2 — festivals were dropped on import", len(ir.Festivals))
	}
	var midwinter, yearless *FestivalInput
	for i := range ir.Festivals {
		f := &ir.Festivals[i]
		switch f.Name {
		case "Midwinter":
			midwinter = f
		case "Yearless Day":
			yearless = f
		}
	}
	if midwinter == nil {
		t.Fatal("Midwinter festival missing from import")
	}
	if midwinter.Month == nil || *midwinter.Month != 1 || midwinter.Day == nil || *midwinter.Day != 1 {
		t.Errorf("Midwinter month/day = %+v", midwinter)
	}
	if midwinter.Color == nil || *midwinter.Color != "#ff0000" {
		t.Errorf("Midwinter lost its color: %+v", midwinter.Color)
	}
	if midwinter.Icon == nil || *midwinter.Icon != icon {
		t.Errorf("Midwinter lost its icon: %+v", midwinter.Icon)
	}
	if yearless == nil {
		t.Fatal("Yearless Day festival missing from import")
	}
	if yearless.AfterMonth == nil || *yearless.AfterMonth != after {
		t.Errorf("Yearless Day lost its after_month: %+v", yearless.AfterMonth)
	}
	if yearless.Description == nil || *yearless.Description != "Falls between years" {
		t.Errorf("Yearless Day lost its description: %+v", yearless.Description)
	}

	// The calendar's own current date round-trips too (#772's "world's date"
	// item), belt-and-braces alongside the cycles/festivals this test exists
	// for.
	if ir.Today.Year != 100 || ir.Today.Month == nil || *ir.Today.Month != 3 || ir.Today.Day == nil || *ir.Today.Day != 12 {
		t.Errorf("ir.Today = %+v, want {100, 3, 12}", ir.Today)
	}
}

// TestCalendariaImport_FestivalsSurvive pins the other half of #771:
// Calendaria's own festivals (a distinct format from Chronicle's own) were
// parsed into calData.Festivals and then never read into the result at all.
func TestCalendariaImport_FestivalsSurvive(t *testing.T) {
	raw := []byte(`{
		"name": "Calendaria Test",
		"days": {"hoursPerDay": 24, "minutesPerHour": 60, "secondsPerMinute": 60},
		"months": {"m1": {"name": "Firstmonth", "days": 30, "ordinal": 1}},
		"festivals": {
			"f1": {"name": "Founding Day", "month": 1, "day": 1, "color": "#00ff00", "icon": "flag", "description": "The city's birthday"}
		}
	}`)

	ir, err := DetectAndParse(raw)
	if err != nil {
		t.Fatalf("DetectAndParse: %v", err)
	}
	if ir.Format != FormatCalendaria {
		t.Fatalf("format = %q, want %q", ir.Format, FormatCalendaria)
	}
	if len(ir.Festivals) != 1 {
		t.Fatalf("imported %d festivals, want 1 — Calendaria festivals were dropped on import", len(ir.Festivals))
	}
	f := ir.Festivals[0]
	if f.Name != "Founding Day" || f.Month == nil || *f.Month != 1 || f.Day == nil || *f.Day != 1 {
		t.Errorf("festival = %+v, want Founding Day on 1/1", f)
	}
	if f.Color == nil || *f.Color != "#00ff00" {
		t.Errorf("festival lost its color: %+v", f.Color)
	}
	if f.Description == nil || *f.Description != "The city's birthday" {
		t.Errorf("festival lost its description: %+v", f.Description)
	}
}
