package importer

import "testing"

// A block with `kind:` is a record: its own keys raise no unknown-key
// warning and it needs no page-only fields, while pages parse as before.
func TestParse_RecordsBesidePages(t *testing.T) {
	in := "---\nname: Grimvale\ntype: location\n---\n# Grimvale\n\nA town.\n\n" +
		"---\nkind: event\nname: Feast\nyear: 1492\nmonth: Hammer\nday: 3\nvisibility: everyone\n---\nA feast.\n\n" +
		"---\nkind: pin\naction: explode\nname: X\n---\n"
	pages := Parse(in)
	if len(pages) != 3 {
		t.Fatalf("got %d blocks", len(pages))
	}
	if pages[0].IsRecord() || pages[0].Status != StatusNew {
		t.Fatalf("page: %+v", pages[0])
	}
	ev := pages[1]
	if !ev.IsRecord() || ev.Status != StatusNew || ev.Name != "Feast" || len(ev.Warnings) != 0 {
		t.Fatalf("record: %+v", ev)
	}
	if ev.Fields["month"] != "Hammer" || ev.Fields["year"] != 1492 {
		t.Fatalf("fields: %#v", ev.Fields)
	}
	if pages[2].Status != StatusParseError {
		t.Fatalf("bad action not refused: %+v", pages[2])
	}
}
