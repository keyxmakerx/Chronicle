package app

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// A campaign's Adventures category comes back from export and import still
// DM-only, with its renamed title and its layout, and exactly once. Losing the
// marker would turn the owner's prep into pages every player can open.
func TestCampaignExportImport_AdventuresStaysDMOnly_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()
	owner := h.newUser("owner")
	src, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Adventures Source"})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	types, err := h.entities.GetEntityTypes(ctx, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	var adv *entities.EntityType
	for i := range types {
		if types[i].DMOnly() {
			adv = &types[i]
		}
	}
	if adv == nil {
		t.Fatal("a new campaign must be seeded with Adventures")
	}
	if _, err := h.entities.Create(ctx, src.ID, owner, entities.CreateEntityInput{Name: "The Sunken Crypt", EntityTypeID: adv.ID}); err != nil {
		t.Fatalf("create adventure: %v", err)
	}

	newID, body := h.importBlob(h.newUser("importer"), "campaign.json", h.export(src, owner, "", "application/json"))
	if strings.Contains(body, "Imported with losses") {
		t.Fatalf("round trip reported losses:\n%s", body)
	}
	got, err := h.entities.GetEntityTypes(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, et := range got {
		if !et.DMOnly() {
			continue
		}
		n++
		if len(et.Layout.Rows) != 2 {
			t.Errorf("imported Adventures layout has %d rows, want the trial page's 2", len(et.Layout.Rows))
		}
	}
	if n != 1 {
		t.Fatalf("imported campaign has %d DM-only categories, want 1", n)
	}
}
