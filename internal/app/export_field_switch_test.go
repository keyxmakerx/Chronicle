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

// fakeSwitchBackup records what an import restores.
type fakeSwitchBackup struct {
	restored map[int]*entities.FieldSwitch
	cleared  []int
	kept     map[string]map[string]any
}

func (f *fakeSwitchBackup) ForCampaign(context.Context, string) (map[int]*entities.FieldSwitch, error) {
	return nil, nil
}

func (f *fakeSwitchBackup) SetAsideForCampaign(context.Context, string) (map[string]map[string]any, error) {
	return nil, nil
}

func (f *fakeSwitchBackup) Restore(_ context.Context, _ string, typeID int, sw *entities.FieldSwitch, _ string) error {
	if sw == nil {
		f.cleared = append(f.cleared, typeID)
		return nil
	}
	f.restored[typeID] = sw
	return nil
}

func (f *fakeSwitchBackup) RestoreSetAside(_ context.Context, _ string, _ int, entityID string, v map[string]any) error {
	f.kept[entityID] = v
	return nil
}

// The answer and kept values survive the file's JSON, and a category or page
// without them writes no key, so an older reader sees the file it knew.
func TestFieldSwitchBackupRoundTrip(t *testing.T) {
	ctx := context.Background()
	oldCat := "custom"
	old := []entities.FieldDefinition{{Key: "rarity", Label: "Rarity", Type: "text"}}
	file := campaigns.CampaignExport{
		EntityTypes: []campaigns.ExportEntityType{
			{Slug: "item", Name: "Item", FieldSwitch: exportFieldSwitch(&entities.FieldSwitch{
				State: entities.FieldSwitchSwitched, SystemID: "drawsteel", PresetSlug: "drawsteel-item",
				OldFields: old, OldPresetCategory: &oldCat,
			})},
			{Slug: "made", Name: "Made", FieldSwitch: exportFieldSwitch(&entities.FieldSwitch{
				State: entities.FieldSwitchSwitched, SystemID: "drawsteel", PresetSlug: "drawsteel-item",
			})},
			{Slug: "note", Name: "Note", FieldSwitch: exportFieldSwitch(nil)},
		},
		Entities: []campaigns.ExportEntity{
			{Name: "Ring", SetAside: json.RawMessage(`{"rarity":"Rare"}`)},
			{Name: "Rope"},
		},
	}
	blob, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(blob), `"field_switch"`); n != 2 {
		t.Errorf("file carries %d answers, want 2 (none for the unanswered category)", n)
	}
	if n := strings.Count(string(blob), `"set_aside"`); n != 1 {
		t.Errorf("file carries kept values for %d pages, want 1", n)
	}
	if n := strings.Count(string(blob), `"old_fields"`); n != 1 {
		t.Errorf("file carries %d lists to switch back to, want 1", n)
	}

	var back campaigns.CampaignExport
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSwitchBackup{restored: map[int]*entities.FieldSwitch{}, kept: map[string]map[string]any{}}
	imp := &entityImportAdapter{fieldSwitch: fake}
	report := campaigns.NewImportReport()
	for i, et := range back.EntityTypes {
		imp.restoreFieldSwitch(ctx, "c2", "u", 10+i, et, report)
	}
	for i, e := range back.Entities {
		imp.restoreSetAside(ctx, "c2", 10, "page-"+string(rune('a'+i)), e, report)
	}

	want := &entities.FieldSwitch{
		State: entities.FieldSwitchSwitched, SystemID: "drawsteel", PresetSlug: "drawsteel-item",
		OldFields: old, OldPresetCategory: &oldCat,
	}
	if got := fake.restored[10]; !reflect.DeepEqual(got, want) {
		t.Errorf("restored answer = %+v, want %+v", got, want)
	}
	if got := fake.restored[11]; got == nil || got.OldFields != nil {
		t.Errorf("a category made with the system's fields came back as %+v, want nothing to switch back to", got)
	}
	if !reflect.DeepEqual(fake.cleared, []int{12}) {
		t.Errorf("cleared = %v, want the unanswered category so its offer shows", fake.cleared)
	}
	if !reflect.DeepEqual(fake.kept, map[string]map[string]any{"page-a": {"rarity": "Rare"}}) {
		t.Errorf("kept values = %v", fake.kept)
	}
	if report.HasFailures() {
		t.Errorf("round trip reported losses: %s", report.Summary())
	}

	// A server without the switch says what it left out rather than
	// dropping it quietly.
	bare := &entityImportAdapter{}
	lossy := campaigns.NewImportReport()
	bare.restoreFieldSwitch(ctx, "c2", "u", 10, back.EntityTypes[0], lossy)
	bare.restoreSetAside(ctx, "c2", 10, "page-a", back.Entities[0], lossy)
	if lossy.Count() != 2 {
		t.Errorf("a server without the switch dropped the answer and kept values without saying so")
	}
}
