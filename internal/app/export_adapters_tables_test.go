package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/rolltables"
)

type memTableRepo map[string][]byte

func (m memTableRepo) Get(_ context.Context, id string) ([]byte, error) { return m[id], nil }
func (m memTableRepo) Put(_ context.Context, id string, d []byte, _ string) error {
	m[id] = d
	return nil
}

func TestRollTablesRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := memTableRepo{}
	svc := rolltables.NewService(repo)
	exp, imp := &rollTableExportAdapter{svc: svc}, &rollTableImportAdapter{svc: svc}

	// A campaign with no tables adds nothing to the file.
	if raw, err := exp.ExportRollTables(ctx, "src"); err != nil || raw != nil {
		t.Fatalf("empty export = %s, %v", raw, err)
	}

	doc := `{"tables":[{"id":"loot","name":"Loot","credit":{"source":"SRD","licence":"CC-BY-4.0"},"entries":[{"name":"Coin","brief":"a purse","weight":3},{"name":"Gem","brief":"","weight":1}]}]}`
	if _, err := svc.Put(ctx, "src", []byte(doc), "u"); err != nil {
		t.Fatal(err)
	}
	raw, err := exp.ExportRollTables(ctx, "src")
	if err != nil || raw == nil {
		t.Fatalf("export = %s, %v", raw, err)
	}

	report := campaigns.NewImportReport()
	if err := imp.ImportRollTables(ctx, "dst", "u", raw, report); err != nil || report.Count() != 0 {
		t.Fatalf("import err=%v lost=%d", err, report.Count())
	}
	got, _ := svc.Get(ctx, "dst")
	if len(got.Tables) != 1 || got.Tables[0].Credit == nil || got.Tables[0].Entries[0].Weight != 3 {
		t.Fatalf("restored = %+v", got)
	}

	// A document the plugin refuses is reported, not stored.
	report = campaigns.NewImportReport()
	if err := imp.ImportRollTables(ctx, "bad", "u", json.RawMessage(`{"tables":[{"id":"X","name":"","entries":[]}]}`), report); err != nil {
		t.Fatal(err)
	}
	if report.Count() != 1 || repo["bad"] != nil {
		t.Fatalf("lost=%d stored=%v", report.Count(), repo["bad"])
	}
}
