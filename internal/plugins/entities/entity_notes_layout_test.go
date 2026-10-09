package entities

import (
	"testing"
)

// blockTypeOrder flattens a layout's block types in row/column/block order.
func blockTypeOrder(l EntityTypeLayout) []string {
	var out []string
	for _, r := range l.Rows {
		for _, c := range r.Columns {
			for _, b := range c.Blocks {
				out = append(out, b.Type)
			}
		}
	}
	return out
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

// TestDefaultLayout_LeavesOutEntityNotes pins that new non-character types
// start without the Player Notes block: the owner adds it where they want
// it. Nor does the default auto-append a permissions block (ADR-057
// decision 5).
func TestDefaultLayout_LeavesOutEntityNotes(t *testing.T) {
	order := blockTypeOrder(DefaultLayout())
	if indexOf(order, "entity_notes") >= 0 {
		t.Fatalf("DefaultLayout must not include an entity_notes block, got %v", order)
	}
	if indexOf(order, "permissions") >= 0 {
		t.Errorf("DefaultLayout must not auto-append a permissions block (ADR-057 decision 5), got %v", order)
	}
}

// TestCharacterLayout_IncludesEntityNotes pins that PC types get Player Notes
// too, without disturbing the character_surface-first invariant that
// ensure_pc_type_test.go relies on. Also no longer carries a permissions
// block (ADR-057 decision 5).
func TestCharacterLayout_IncludesEntityNotes(t *testing.T) {
	l := CharacterLayout()
	order := blockTypeOrder(l)
	if indexOf(order, "entity_notes") < 0 {
		t.Fatalf("CharacterLayout must include an entity_notes block, got %v", order)
	}
	if l.Rows[0].Columns[0].Blocks[0].Type != "character_surface" {
		t.Errorf("character_surface must remain the first block, got %v", order)
	}
	if indexOf(order, "permissions") >= 0 {
		t.Errorf("CharacterLayout must not auto-append a permissions block (ADR-057 decision 5), got %v", order)
	}
}
