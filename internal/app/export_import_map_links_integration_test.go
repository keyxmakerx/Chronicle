package app

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/maps"
)

// TestCampaignExportImport_MapLinks_DBRoundTrip: a pin that opens another map
// comes back opening that map's copy in the imported campaign, including when
// the target map comes later in the file than the pin.
func TestCampaignExportImport_MapLinks_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	ctx := context.Background()
	owner := h.newUser("owner")
	src, _ := h.sourceWithPlayerCharacter(owner)

	world, err := h.maps.CreateMap(ctx, maps.CreateMapInput{CampaignID: src.ID, Name: "World"})
	if err != nil {
		t.Fatal(err)
	}
	isle, err := h.maps.CreateMap(ctx, maps.CreateMapInput{CampaignID: src.ID, Name: "Isle"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct{ on, to, name string }{{world.ID, isle.ID, "To the isle"}, {isle.ID, world.ID, "Back to the world"}} {
		to := p.to
		if _, err := h.maps.CreateMarker(ctx, maps.CreateMarkerInput{MapID: p.on, Name: p.name, X: 5, Y: 5, LinkedMapID: &to, CreatedBy: owner}); err != nil {
			t.Fatalf("create %s: %v", p.name, err)
		}
	}

	newID, body := h.importBlob(h.newUser("importer"), "campaign.zip", h.exportZip(src, owner))
	if strings.Contains(body, "Imported with losses") {
		t.Errorf("round trip reported losses:\n%s", body)
	}
	rows, err := h.db.Query(`
		SELECT mk.name, src.name, tgt.name, tgt.campaign_id
		  FROM map_markers mk
		  JOIN maps src ON src.id = mk.map_id
		  LEFT JOIN maps tgt ON tgt.id = mk.linked_map_id
		 WHERE src.campaign_id = ?`, newID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var pin, on string
		var to, toCampaign sql.NullString
		if err := rows.Scan(&pin, &on, &to, &toCampaign); err != nil {
			t.Fatal(err)
		}
		if to.Valid && toCampaign.String != newID {
			t.Errorf("pin %q opens a map of campaign %s, want the imported one", pin, toCampaign.String)
		}
		got[pin] = on + "->" + to.String
	}
	want := map[string]string{"To the isle": "World->Isle", "Back to the world": "Isle->World"}
	for pin, w := range want {
		if got[pin] != w {
			t.Errorf("pin %q = %q, want %q", pin, got[pin], w)
		}
	}
}
