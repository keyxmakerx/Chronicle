// export_field_switch_roundtrip_integration_test.go round-trips a game
// system field switch through the real export and import handlers: each
// category's answer and the values the switch kept aside come back on the
// restored campaign's own categories and pages, and an older backup without
// them restores with the offer still to answer.
//
// Skipped under -short. Run with `make test-int-local`.
package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fieldSwitchSource is a campaign whose Locations switched to a game system's
// fields (with a page's old Rarity kept aside) and whose Notes answered "Not
// now". Character is left unanswered.
func (h *roundTripHarness) fieldSwitchSource(owner string) (*campaigns.Campaign, []entities.FieldDefinition) {
	h.t.Helper()
	ctx := context.Background()
	src, err := h.campaigns.Create(ctx, owner, campaigns.CreateCampaignInput{Name: "Field Switch Source"})
	if err != nil {
		h.t.Fatalf("create source campaign: %v", err)
	}
	location, note := h.typeID(src.ID, "location"), h.typeID(src.ID, "note")
	old := []entities.FieldDefinition{
		{Key: "rarity", Label: "Rarity", Type: "text"},
		{Key: "weight", Label: "Weight", Type: "text"},
	}
	oldPreset := "custom-old"
	if err := h.switches.Restore(ctx, src.ID, location, &entities.FieldSwitch{
		State: entities.FieldSwitchSwitched, SystemID: "drawsteel", PresetSlug: "drawsteel-item",
		OldFields: old, OldPresetCategory: &oldPreset,
	}, owner); err != nil {
		h.t.Fatalf("seed switched answer: %v", err)
	}
	if err := h.switches.Restore(ctx, src.ID, note, &entities.FieldSwitch{
		State: entities.FieldSwitchDismissed, SystemID: "drawsteel", PresetSlug: "drawsteel-item",
	}, owner); err != nil {
		h.t.Fatalf("seed dismissed answer: %v", err)
	}
	for _, name := range []string{"Old Keep", "Bare Tower"} {
		page, err := h.entities.Create(ctx, src.ID, owner, entities.CreateEntityInput{Name: name, EntityTypeID: location})
		if err != nil {
			h.t.Fatalf("create %s: %v", name, err)
		}
		if name == "Old Keep" {
			if err := h.switches.RestoreSetAside(ctx, src.ID, location, page.ID, map[string]any{"rarity": "Rare", "weight": "3 lb"}); err != nil {
				h.t.Fatalf("seed kept values: %v", err)
			}
		}
	}
	return src, old
}

// switchAnswers is each category's answer by slug, without ids or times.
func (h *roundTripHarness) switchAnswers(campaignID string) map[string]entities.FieldSwitch {
	h.t.Helper()
	ctx := context.Background()
	rows, err := h.switches.ForCampaign(ctx, campaignID)
	if err != nil {
		h.t.Fatalf("list answers: %v", err)
	}
	types, err := h.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		h.t.Fatalf("list categories: %v", err)
	}
	out := map[string]entities.FieldSwitch{}
	for _, et := range types {
		if sw, ok := rows[et.ID]; ok {
			out[et.Slug] = entities.FieldSwitch{
				State: sw.State, SystemID: sw.SystemID, PresetSlug: sw.PresetSlug,
				OldFields: sw.OldFields, OldPresetCategory: sw.OldPresetCategory,
			}
		}
	}
	return out
}

// keptByName is each page's kept-aside values by page name.
func (h *roundTripHarness) keptByName(campaignID string) map[string]map[string]any {
	h.t.Helper()
	kept, err := h.switches.SetAsideForCampaign(context.Background(), campaignID)
	if err != nil {
		h.t.Fatalf("list kept values: %v", err)
	}
	out := map[string]map[string]any{}
	for id, vals := range kept {
		var name, cid string
		if err := h.db.QueryRow(`SELECT name, campaign_id FROM entities WHERE id = ?`, id).Scan(&name, &cid); err != nil {
			h.t.Fatalf("kept values page %s: %v", id, err)
		}
		if cid != campaignID {
			h.t.Fatalf("kept values page %s is in campaign %s, want %s", id, cid, campaignID)
		}
		out[name] = vals
	}
	return out
}

func TestCampaignExportImport_FieldSwitch_DBRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	h := newRoundTripHarness(t)
	owner := h.newUser("owner")
	src, old := h.fieldSwitchSource(owner)
	blob := h.export(src, owner, "", "application/json")

	// Only answered categories and pages with kept values carry the new
	// keys, so a backup of a campaign that never saw a switch is unchanged.
	var file campaigns.CampaignExport
	if err := json.Unmarshal(blob, &file); err != nil {
		t.Fatalf("read export: %v", err)
	}
	carried := map[string]bool{}
	for _, et := range file.EntityTypes {
		if et.FieldSwitch != nil {
			carried[et.Slug] = true
		}
	}
	if !reflect.DeepEqual(carried, map[string]bool{"location": true, "note": true}) {
		t.Errorf("categories carrying an answer = %v, want location and note", carried)
	}
	for _, e := range file.Entities {
		if has := len(e.SetAside) > 0; has != (e.Name == "Old Keep") {
			t.Errorf("page %q carries kept values = %v", e.Name, has)
		}
	}

	t.Run("restores answers and kept values", func(t *testing.T) {
		newID, body := h.importBlob(h.newUser("importer"), "campaign.json", blob)
		if newID == src.ID {
			t.Fatalf("import made no new campaign")
		}
		if strings.Contains(body, "Imported with losses") {
			t.Errorf("round trip reported losses:\n%s", body)
		}
		want, got := h.switchAnswers(src.ID), h.switchAnswers(newID)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("restored answers = %+v\nwant %+v", got, want)
		}
		if loc := got["location"]; !reflect.DeepEqual(loc.OldFields, old) {
			t.Errorf("restored fields to switch back to = %+v, want %+v", loc.OldFields, old)
		}
		if n := got["note"]; n.OldFields != nil {
			t.Errorf("a \"Not now\" answer came back with fields to switch back to: %+v", n.OldFields)
		}
		wantKept := map[string]map[string]any{"Old Keep": {"rarity": "Rare", "weight": "3 lb"}}
		if k := h.keptByName(newID); !reflect.DeepEqual(k, wantKept) {
			t.Errorf("restored kept values = %v, want %v", k, wantKept)
		}
	})

	t.Run("older backup without the keys", func(t *testing.T) {
		var raw map[string]any
		if err := json.Unmarshal(blob, &raw); err != nil {
			t.Fatalf("read export: %v", err)
		}
		for _, list := range []string{"entity_types", "entities"} {
			for _, item := range raw[list].([]any) {
				m := item.(map[string]any)
				delete(m, "field_switch")
				delete(m, "set_aside")
			}
		}
		older, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("write older file: %v", err)
		}
		newID, body := h.importBlob(h.newUser("older-importer"), "campaign.json", older)
		if strings.Contains(body, "Imported with losses") {
			t.Errorf("older file reported losses:\n%s", body)
		}
		if got := h.switchAnswers(newID); len(got) != 0 {
			t.Errorf("older file restored answers %+v, want none (the offer shows)", got)
		}
		if got := h.keptByName(newID); len(got) != 0 {
			t.Errorf("older file restored kept values %v, want none", got)
		}
	})
}
