package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
)

// stubTables is a rolling tables service with a fixed document.
type stubTables struct {
	rolltables.Service
	doc rolltables.Document
	err error
}

func (s stubTables) Get(context.Context, string) (rolltables.Document, error) { return s.doc, s.err }

// The adapter lists every table, Director-only ones too, with their line
// counts, and reads one table's lines with whether a line has day conditions.
func TestDecksTablesAdapterReadsTablesAndLines(t *testing.T) {
	a := &decksTablesAdapter{svc: stubTables{doc: rolltables.Document{Tables: []rolltables.Table{
		{ID: "loot", Name: "Loot", Entries: []rolltables.Entry{{Name: "Gold", Brief: "A purse", Weight: 2}, {Name: "Moonstone", Weight: 1, When: json.RawMessage(`{"moon":["full"]}`)}}},
		{ID: "secret", Name: "Secret", Visibility: rolltables.VisibilityDMOnly, Entries: []rolltables.Entry{{Name: "Twist", Weight: 1}}},
	}}}}
	list, err := a.List(context.Background(), "c1")
	if err != nil || len(list) != 2 || list[0].ID != "loot" || list[0].Lines != 2 || list[1].Name != "Secret" {
		t.Fatalf("list %+v, %v", list, err)
	}
	lines, name, ok, err := a.Lines(context.Background(), "c1", "loot")
	if err != nil || !ok || name != "Loot" || len(lines) != 2 {
		t.Fatalf("lines %+v %q %v %v", lines, name, ok, err)
	}
	if lines[0].Brief != "A purse" || lines[0].Weight != 2 || lines[0].HasDays || !lines[1].HasDays {
		t.Errorf("lines %+v", lines)
	}
	if _, _, ok, err := a.Lines(context.Background(), "c1", "nope"); ok || err != nil {
		t.Errorf("a table that is not there: ok=%v err=%v", ok, err)
	}
	bad := &decksTablesAdapter{svc: stubTables{err: errors.New("down")}}
	if _, err := bad.List(context.Background(), "c1"); err == nil {
		t.Error("a failing read said nothing")
	}
}
