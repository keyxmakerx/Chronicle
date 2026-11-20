package entities

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A refusal names the offending key so a player's AI can drop it and resend.
func TestAuthorizeFieldsWrite_NamesTheRefusedKey(t *testing.T) {
	uid := "u1"
	e := &Entity{CampaignID: "c1", OwnerUserID: &uid}
	err := authorizeFieldsWrite(campaigns.RolePlayer, e, uid, "c1", false,
		map[string]any{"race": "Elf", "stamina": "40"}, nil, builderPicks{})
	if err == nil || !strings.Contains(err.Error(), `"stamina"`) {
		t.Fatalf("want a refusal naming stamina, got %v", err)
	}
}

// What a hero buys from an ancestry is checked against that ancestry's list
// and points, and changing the ancestry clears what was bought from the old one.
func TestStepOptions(t *testing.T) {
	uid := "u1"
	devil := HeroChoice{Name: "Devil", Properties: map[string]any{
		"ancestry_points": float64(3),
		"purchased_traits": []any{
			map[string]any{"name": "Wings", "cost": float64(2)},
			map[string]any{"name": "Silver Tongue", "cost": float64(1)},
			map[string]any{"name": "Hellfire", "cost": float64(2)},
		}}}
	picks := builderPicks{"ancestry": {names: map[string]HeroChoice{"devil": devil, "dwarf": {Name: "Dwarf"}}}}
	types := []FieldDefinition{{Key: "ancestry"}}
	e := &Entity{CampaignID: "c1", OwnerUserID: &uid, FieldsData: map[string]any{"ancestry": "Devil"}}
	tests := []struct {
		name  string
		patch map[string]any
		ok    bool
	}{
		{"traits within the points", map[string]any{"ancestry_choices_json": `["Wings","Silver Tongue"]`}, true},
		{"over the points", map[string]any{"ancestry_choices_json": `["Wings","Hellfire"]`}, false},
		{"a trait not on the list", map[string]any{"ancestry_choices_json": `["Laser Eyes"]`}, false},
		{"an ancestry with nothing to buy", map[string]any{"ancestry": "Dwarf", "ancestry_choices_json": `["Wings"]`}, false},
		{"cleared", map[string]any{"ancestry_choices_json": nil}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authorizeFieldsWrite(campaigns.RolePlayer, e, uid, "c1", false, tt.patch, types, picks)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
		})
	}

	bought := &Entity{FieldsData: map[string]any{"ancestry": "Devil", "ancestry_choices_json": `["Wings"]`}}
	p := map[string]any{"ancestry": "Dwarf"}
	ClearStaleStepOptions(bought, p)
	if v, ok := p["ancestry_choices_json"]; !ok || v != nil {
		t.Fatalf("changing ancestry kept the old traits: %v", p)
	}
	same := map[string]any{"ancestry": "devil"}
	ClearStaleStepOptions(bought, same)
	if _, ok := same["ancestry_choices_json"]; ok {
		t.Fatal("re-saving the same ancestry cleared its traits")
	}
}

// Each refused key gets its own reason; the rest of the patch is untouched.
func TestClaimedOwnerPatchHeld(t *testing.T) {
	uid := "u1"
	e := &Entity{CampaignID: "c1", OwnerUserID: &uid}
	types := []FieldDefinition{{Key: "class"}, {Key: "might"}, {Key: "complication"}, {Key: "backstory"}}
	plan := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Choices: []HeroChoice{{Name: "Shadow"}}},
		{FieldKey: "complication", Choices: []HeroChoice{{Name: "Lucky"}}}}}
	held, err := ClaimedOwnerPatchHeld(e, uid, "c1", map[string]any{
		"class": "Shadow", "complication": "Lucky", "might": "2", "race": "Elf", "backstory": "Raised in a forge.",
	}, types, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(held) != 2 || !strings.Contains(held["might"], `"might"`) || !strings.Contains(held["race"], `no "race" field`) {
		t.Fatalf("held = %v, want might and race, each with its reason", held)
	}
	if _, err := ClaimedOwnerPatchHeld(e, "u2", "c1", map[string]any{"class": "Shadow"}, types, plan); err == nil {
		t.Fatal("another player's patch was judged key by key")
	}
}
