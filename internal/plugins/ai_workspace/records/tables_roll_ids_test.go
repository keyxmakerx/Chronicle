package records

import (
	"context"
	"reflect"
	"testing"
)

func TestRegistry_RollTableIDs(t *testing.T) {
	tables := &fakeTables{doc: TableDoc{Tables: []Table{{ID: "tavern-names", Name: "Tavern names"}, {ID: "inns", Name: "Old inns"}}}}
	reg := NewRegistry(TableKind{Svc: tables})
	recs := []Record{
		{Kind: "table", Action: ActionCreate, Name: "Tavern Names!"}, // slug taken: gets -2
		{Kind: "table", Action: ActionCreate, Name: "Old inns"},      // exists: Apply refuses it
		{Kind: "table", Action: ActionUpdate, Name: "Old inns", Fields: map[string]any{"rename_to": "Taverns of old"}},
		{Kind: "event", Action: ActionCreate, Name: "Not a table"},
	}
	got := reg.RollTableIDs(context.Background(), "c", recs)
	want := map[string]string{
		"tavern-names": "tavern-names", "tavern names": "tavern-names",
		"inns": "inns", "old inns": "inns", "taverns of old": "inns",
		"tavern names!": "tavern-names-2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if got := NewRegistry().RollTableIDs(context.Background(), "c", recs); got != nil {
		t.Errorf("no table kind: %v", got)
	}
}
