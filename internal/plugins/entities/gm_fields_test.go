package entities

import (
	"context"
	"encoding/json"
	"testing"
)

func strptr(s string) *string { return &s }

// TestFilterGMOnlyFields is the table-driven contract for the single shared
// filter both egress paths use (audit M-1).
func TestFilterGMOnlyFields(t *testing.T) {
	defs := []FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "gm_notes", Label: "GM Notes", GMOnly: true},
	}
	full := map[string]any{"might": 3, "gm_notes": "the villain is his father"}

	cases := []struct {
		name        string
		data        map[string]any
		defs        []FieldDefinition
		canSeeGM    bool
		wantGMNotes bool // gm_notes present in result?
		wantMight   bool
	}{
		{"player strips gm_notes", full, defs, false, false, true},
		{"GM keeps everything", full, defs, true, true, true},
		{"no defs → untouched", full, nil, false, true, true},
		{"unmarked-only defs → untouched", full, []FieldDefinition{{Key: "might"}}, false, true, true},
		{"gm_only key absent from data → untouched", map[string]any{"might": 1}, defs, false, false, true},
		{"empty data", map[string]any{}, defs, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := FilterGMOnlyFields(tc.data, tc.defs, tc.canSeeGM)
			if _, ok := out["gm_notes"]; ok != tc.wantGMNotes {
				t.Errorf("gm_notes present=%v, want %v (out=%v)", ok, tc.wantGMNotes, out)
			}
			if _, ok := out["might"]; ok != tc.wantMight {
				t.Errorf("might present=%v, want %v (out=%v)", ok, tc.wantMight, out)
			}
		})
	}
}

// TestFilterGMOnlyFields_DoesNotMutateInput pins that a strip returns a fresh
// map and leaves the caller's (DB-backed) map intact.
func TestFilterGMOnlyFields_DoesNotMutateInput(t *testing.T) {
	defs := []FieldDefinition{{Key: "gm_notes", GMOnly: true}}
	in := map[string]any{"might": 3, "gm_notes": "secret"}
	out := FilterGMOnlyFields(in, defs, false)
	if _, ok := in["gm_notes"]; !ok {
		t.Error("input map was mutated: gm_notes removed from the source map")
	}
	if _, ok := out["gm_notes"]; ok {
		t.Error("output must not contain gm_notes")
	}
}

// TestFilterOwnerOnlyFields is the table-driven contract for owner-only field
// stripping (C-FIELDS-OWNER-FILTER) — mirrors TestFilterGMOnlyFields, with
// isOwner as the second way (besides canSeeGM) to keep a value.
func TestFilterOwnerOnlyFields(t *testing.T) {
	defs := []FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "backstory", Label: "Backstory", OwnerOnly: true},
	}
	full := map[string]any{"might": 3, "backstory": "raised by wolves"}

	cases := []struct {
		name          string
		data          map[string]any
		defs          []FieldDefinition
		canSeeGM      bool
		isOwner       bool
		wantBackstory bool
		wantMight     bool
	}{
		{"other player strips backstory", full, defs, false, false, false, true},
		{"owner keeps backstory", full, defs, false, true, true, true},
		{"GM keeps backstory regardless of ownership", full, defs, true, false, true, true},
		{"no defs → untouched", full, nil, false, false, true, true},
		{"unmarked-only defs → untouched", full, []FieldDefinition{{Key: "might"}}, false, false, true, true},
		{"owner_only key absent from data → untouched", map[string]any{"might": 1}, defs, false, false, false, true},
		{"empty data", map[string]any{}, defs, false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := FilterOwnerOnlyFields(tc.data, tc.defs, tc.canSeeGM, tc.isOwner)
			if _, ok := out["backstory"]; ok != tc.wantBackstory {
				t.Errorf("backstory present=%v, want %v (out=%v)", ok, tc.wantBackstory, out)
			}
			if _, ok := out["might"]; ok != tc.wantMight {
				t.Errorf("might present=%v, want %v (out=%v)", ok, tc.wantMight, out)
			}
		})
	}
}

// TestFilterOwnerOnlyFields_DoesNotMutateInput mirrors the GM-only version.
func TestFilterOwnerOnlyFields_DoesNotMutateInput(t *testing.T) {
	defs := []FieldDefinition{{Key: "backstory", OwnerOnly: true}}
	in := map[string]any{"might": 3, "backstory": "secret"}
	out := FilterOwnerOnlyFields(in, defs, false, false)
	if _, ok := in["backstory"]; !ok {
		t.Error("input map was mutated: backstory removed from the source map")
	}
	if _, ok := out["backstory"]; ok {
		t.Error("output must not contain backstory")
	}
}

// TestFilterRestrictedFields pins that the composed helper applies BOTH
// tiers in one pass: a GM-only field stays hidden from the entity's own
// owner (it's a true GM secret), while an owner-only field is shown to the
// owner but not to another player.
func TestFilterRestrictedFields(t *testing.T) {
	defs := []FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "gm_notes", Label: "GM Notes", GMOnly: true},
		{Key: "backstory", Label: "Backstory", OwnerOnly: true},
	}
	full := map[string]any{"might": 3, "gm_notes": "the villain is his father", "backstory": "raised by wolves"}

	// The owner: sees their own backstory, but not the GM-only secret.
	owner := FilterRestrictedFields(full, defs, false, true)
	if _, ok := owner["backstory"]; !ok {
		t.Error("owner must see their own owner_only backstory")
	}
	if _, ok := owner["gm_notes"]; ok {
		t.Error("owner must NOT see a GM-only field even on their own entity")
	}

	// Another player: sees neither.
	other := FilterRestrictedFields(full, defs, false, false)
	if _, ok := other["backstory"]; ok {
		t.Error("another player must not see owner_only backstory")
	}
	if _, ok := other["gm_notes"]; ok {
		t.Error("another player must not see gm_only notes")
	}

	// GM: sees everything.
	gm := FilterRestrictedFields(full, defs, true, false)
	if _, ok := gm["backstory"]; !ok {
		t.Error("GM must see owner_only backstory")
	}
	if _, ok := gm["gm_notes"]; !ok {
		t.Error("GM must see gm_only notes")
	}
}

// TestSyncFieldGMFlags_ConvergesAndIsIdempotent pins the reconciler: it stamps
// gm_only from the (category → key → gm_only) map onto matching types, ignores
// non-matching categories, is idempotent, and can also UN-mark a field.
func TestSyncFieldGMFlags_ConvergesAndIsIdempotent(t *testing.T) {
	// Type 1 (category "character") has an unmarked gm_notes that must become
	// gm_only; type 2 (category "location") is untouched (category not in map).
	t1 := EntityType{ID: 1, PresetCategory: strptr("character"), Fields: []FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "gm_notes", Label: "GM Notes"}, // currently NOT gm_only
	}}
	t2 := EntityType{ID: 2, PresetCategory: strptr("location"), Fields: []FieldDefinition{
		{Key: "region", Label: "Region"},
	}}

	saved := map[int]string{}
	typeRepo := &mockEntityTypeRepo{
		listAllFn: func(_ context.Context) ([]EntityType, error) {
			return []EntityType{t1, t2}, nil
		},
		updateFieldsSchemaFn: func(_ context.Context, id int, fieldsJSON string) error {
			saved[id] = fieldsJSON
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo).(*entityService)

	gmByCategory := map[string]map[string]bool{
		"character": {"might": false, "gm_notes": true},
	}
	n, err := svc.SyncFieldGMFlags(context.Background(), gmByCategory)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n != 1 {
		t.Fatalf("want exactly 1 type updated (type 1), got %d", n)
	}
	if _, touched := saved[2]; touched {
		t.Error("type 2 (category not in map) must not be written")
	}
	var got []FieldDefinition
	if err := json.Unmarshal([]byte(saved[1]), &got); err != nil {
		t.Fatalf("unmarshal saved fields: %v", err)
	}
	var gmNotes *FieldDefinition
	for i := range got {
		if got[i].Key == "gm_notes" {
			gmNotes = &got[i]
		}
	}
	if gmNotes == nil || !gmNotes.GMOnly {
		t.Fatalf("gm_notes must be stamped gm_only=true, got %+v", got)
	}

	// Idempotent: feed the healed type back; no further write.
	typeRepo.listAllFn = func(_ context.Context) ([]EntityType, error) {
		return []EntityType{{ID: 1, PresetCategory: strptr("character"), Fields: got}}, nil
	}
	n2, err := svc.SyncFieldGMFlags(context.Background(), gmByCategory)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second run must be a no-op, got %d", n2)
	}

	// Un-mark: manifest flips gm_notes back to player-visible → converges.
	typeRepo.listAllFn = func(_ context.Context) ([]EntityType, error) {
		return []EntityType{{ID: 1, PresetCategory: strptr("character"), Fields: got}}, nil
	}
	n3, err := svc.SyncFieldGMFlags(context.Background(), map[string]map[string]bool{
		"character": {"gm_notes": false},
	})
	if err != nil {
		t.Fatalf("unmark sync: %v", err)
	}
	if n3 != 1 {
		t.Errorf("un-marking gm_notes should update 1 type, got %d", n3)
	}
}

// TestSyncFieldOwnerOnlyFlags_ConvergesAndIsIdempotent mirrors
// TestSyncFieldGMFlags_ConvergesAndIsIdempotent for the owner_only flag —
// same shared syncFieldFlag walk, different flag.
func TestSyncFieldOwnerOnlyFlags_ConvergesAndIsIdempotent(t *testing.T) {
	t1 := EntityType{ID: 1, PresetCategory: strptr("character"), Fields: []FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "backstory", Label: "Backstory"}, // currently NOT owner_only
	}}
	t2 := EntityType{ID: 2, PresetCategory: strptr("location"), Fields: []FieldDefinition{
		{Key: "region", Label: "Region"},
	}}

	saved := map[int]string{}
	typeRepo := &mockEntityTypeRepo{
		listAllFn: func(_ context.Context) ([]EntityType, error) {
			return []EntityType{t1, t2}, nil
		},
		updateFieldsSchemaFn: func(_ context.Context, id int, fieldsJSON string) error {
			saved[id] = fieldsJSON
			return nil
		},
	}
	svc := newTestService(&mockEntityRepo{}, typeRepo).(*entityService)

	ownerOnlyByCategory := map[string]map[string]bool{
		"character": {"might": false, "backstory": true},
	}
	n, err := svc.SyncFieldOwnerOnlyFlags(context.Background(), ownerOnlyByCategory)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n != 1 {
		t.Fatalf("want exactly 1 type updated (type 1), got %d", n)
	}
	if _, touched := saved[2]; touched {
		t.Error("type 2 (category not in map) must not be written")
	}
	var got []FieldDefinition
	if err := json.Unmarshal([]byte(saved[1]), &got); err != nil {
		t.Fatalf("unmarshal saved fields: %v", err)
	}
	var backstory *FieldDefinition
	for i := range got {
		if got[i].Key == "backstory" {
			backstory = &got[i]
		}
	}
	if backstory == nil || !backstory.OwnerOnly {
		t.Fatalf("backstory must be stamped owner_only=true, got %+v", got)
	}

	// Idempotent: feed the healed type back; no further write.
	typeRepo.listAllFn = func(_ context.Context) ([]EntityType, error) {
		return []EntityType{{ID: 1, PresetCategory: strptr("character"), Fields: got}}, nil
	}
	n2, err := svc.SyncFieldOwnerOnlyFlags(context.Background(), ownerOnlyByCategory)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second run must be a no-op, got %d", n2)
	}

	// Un-mark: manifest flips backstory back to shared → converges.
	typeRepo.listAllFn = func(_ context.Context) ([]EntityType, error) {
		return []EntityType{{ID: 1, PresetCategory: strptr("character"), Fields: got}}, nil
	}
	n3, err := svc.SyncFieldOwnerOnlyFlags(context.Background(), map[string]map[string]bool{
		"character": {"backstory": false},
	})
	if err != nil {
		t.Fatalf("unmark sync: %v", err)
	}
	if n3 != 1 {
		t.Errorf("un-marking backstory should update 1 type, got %d", n3)
	}
}
