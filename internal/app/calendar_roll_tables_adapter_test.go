package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
)

type memRollTables struct{ data []byte }

func (m *memRollTables) Get(context.Context, string) ([]byte, error) { return m.data, nil }
func (m *memRollTables) Put(_ context.Context, _ string, d []byte, _ string) error {
	m.data = d
	return nil
}

// What the rolling tables page saves (who sees a day's roll, each line's
// conditions) is what the calendar's event roll is handed.
func TestCalendarRollTablesAdapter_CarriesWhenAndVisibility(t *testing.T) {
	svc := rolltables.NewService(&memRollTables{})
	body := `{"tables":[{"id":"omens","name":"Omens","visibility":"dm_only","entries":[
		{"name":"Crows","brief":"b","weight":2,"when":{"moon":{"name":"any","phase":"full","within":1}}},
		{"name":"Fog"}]}]}`
	if _, err := svc.Put(context.Background(), "c1", []byte(body), "u1"); err != nil {
		t.Fatal(err)
	}
	got, err := calendarRollTablesAdapter{svc: svc}.RollTable(context.Background(), "c1", "omens")
	if err != nil || got == nil {
		t.Fatalf("RollTable = %v, %v", got, err)
	}
	if got.Visibility != "dm_only" {
		t.Errorf("visibility = %q, want dm_only", got.Visibility)
	}
	if string(got.Entries[0].When) != `{"moon":{"name":"any","phase":"full","within":1}}` || got.Entries[1].When != nil {
		t.Errorf("when = %q and %q", got.Entries[0].When, got.Entries[1].When)
	}
	if got.Entries[0].Weight != 2 || got.Entries[1].Weight != 1 {
		t.Errorf("weights = %v and %v", got.Entries[0].Weight, got.Entries[1].Weight)
	}
}
