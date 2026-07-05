package entities

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCharacterSurfaceSchemaJSON pins the seeded schema: provider key, the seed
// (name/image/editor), and non-empty fields grouped by section in order.
func TestCharacterSurfaceSchemaJSON(t *testing.T) {
	img := "portraits/tyne.png"
	label := "Tactician"
	et := &EntityType{
		Fields: []FieldDefinition{
			{Key: "might", Label: "Might", Section: "Characteristics"},
			{Key: "agility", Label: "Agility", Section: "Characteristics"},
			{Key: "bond", Label: "Bond", Section: "Story"},
			{Key: "blank", Label: "Blank", Section: "Story"},
		},
	}
	ent := &Entity{
		ID: "e1", Name: "Tyne", TypeName: "Character",
		ImagePath: &img, TypeLabel: &label,
		FieldsData: map[string]any{"might": 2, "agility": 1, "bond": "The company", "blank": ""},
	}

	out := CharacterSurfaceSchemaJSON(ent, et, "c1", "csrf-tok", true, true)

	var schema map[string]any
	if err := json.Unmarshal([]byte(out), &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v\n%s", err, out)
	}
	prov := schema["provider"].(map[string]any)
	if prov["key"] != "entity:e1" {
		t.Errorf("provider key = %v, want entity:e1", prov["key"])
	}
	seed := prov["seed"].(map[string]any)
	if seed["name"] != "Tyne" {
		t.Errorf("seed name = %v", seed["name"])
	}
	if seed["image"] != "/media/portraits/tyne.png" {
		t.Errorf("seed image = %v, want /media/portraits/tyne.png", seed["image"])
	}

	ed := seed["editor"].(map[string]any)
	if ed["endpoint"] != "/campaigns/c1/entities/e1/entry" {
		t.Errorf("editor endpoint = %v", ed["endpoint"])
	}
	if ed["canEdit"] != true {
		t.Errorf("editor canEdit = %v, want true", ed["canEdit"])
	}

	// Characteristics (2 fields) then Story (1 — the blank value is dropped).
	secs := seed["sections"].([]any)
	if len(secs) != 2 {
		t.Fatalf("sections = %d, want 2", len(secs))
	}
	s0 := secs[0].(map[string]any)
	if s0["title"] != "Characteristics" || len(s0["fields"].([]any)) != 2 {
		t.Errorf("section 0 = %+v, want Characteristics with 2 fields", s0)
	}
	s1 := secs[1].(map[string]any)
	if s1["title"] != "Story" || len(s1["fields"].([]any)) != 1 {
		t.Errorf("section 1 = %+v, want Story with 1 field (blank dropped)", s1)
	}
}

// TestCharacterSurfaceSchemaJSON_ScriptSafe ensures the JSON is safe to embed in
// a <script> tag — json.Marshal escapes '<', so no literal </script> survives.
func TestCharacterSurfaceSchemaJSON_ScriptSafe(t *testing.T) {
	ent := &Entity{ID: "e1", Name: "</script><script>alert(1)</script>", FieldsData: map[string]any{}}
	out := CharacterSurfaceSchemaJSON(ent, &EntityType{}, "c1", "", false, false)
	if strings.Contains(out, "</script>") {
		t.Fatalf("schema must not contain a literal </script>; got: %s", out)
	}
}

// TestCharacterSurfaceSchemaJSON_StripsGMFieldsForNonGM pins that the
// server-rendered character sheet omits gm_only field values for a non-GM
// viewer (audit M-1) and keeps them for a GM.
func TestCharacterSurfaceSchemaJSON_StripsGMFieldsForNonGM(t *testing.T) {
	et := &EntityType{Fields: []FieldDefinition{
		{Key: "might", Label: "Might", Section: "Stats"},
		{Key: "gm_notes", Label: "GM Notes", Section: "Director", GMOnly: true},
	}}
	ent := &Entity{ID: "e1", Name: "Hero", FieldsData: map[string]any{
		"might": 3, "gm_notes": "the-villain-is-his-father",
	}}

	// Player (canSeeGM=false): the gm_only field and its value must be absent.
	player := CharacterSurfaceSchemaJSON(ent, et, "c1", "", false, false)
	if strings.Contains(player, "the-villain-is-his-father") || strings.Contains(player, "GM Notes") {
		t.Errorf("player seed must omit the gm_only field + value; got %s", player)
	}
	if !strings.Contains(player, "Might") {
		t.Errorf("player seed must still include normal fields; got %s", player)
	}

	// GM (canSeeGM=true): the gm_only value is present.
	gm := CharacterSurfaceSchemaJSON(ent, et, "c1", "", true, true)
	if !strings.Contains(gm, "the-villain-is-his-father") {
		t.Errorf("GM seed must include the gm_only value; got %s", gm)
	}
}

// TestCharacterSurfaceSchemaJSON_NilEntity degrades to an empty object.
func TestCharacterSurfaceSchemaJSON_NilEntity(t *testing.T) {
	if got := CharacterSurfaceSchemaJSON(nil, nil, "c1", "", false, false); got != "{}" {
		t.Errorf("nil entity = %q, want {}", got)
	}
}
