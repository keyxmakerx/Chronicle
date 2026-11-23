package syncapi

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// TestItemPresetFor pins that a switch preset reaches item sync only once the
// campaign's Items have switched, while an ordinary item preset always does.
func TestItemPresetFor(t *testing.T) {
	plain := &systems.SystemManifest{ID: "dnd", EntityPresets: []systems.EntityPresetDef{{Slug: "dnd-item", Category: "item"}}}
	switched := &systems.SystemManifest{ID: "drawsteel", EntityPresets: []systems.EntityPresetDef{
		{Slug: "drawsteel-item", Category: "item", Switch: &systems.PresetSwitchDef{Label: "treasure fields"}},
	}}
	none := &systems.SystemManifest{ID: "x"}
	yes := func(context.Context, string, *systems.EntityPresetDef) bool { return true }
	no := func(context.Context, string, *systems.EntityPresetDef) bool { return false }
	tests := []struct {
		name string
		m    *systems.SystemManifest
		gate ItemFieldsGate
		want bool
	}{
		{"an ordinary item preset needs no switch", plain, nil, true},
		{"no item preset", none, yes, false},
		{"a switch preset before the switch", switched, no, false},
		{"a switch preset with the gate unwired", switched, nil, false},
		{"a switch preset after the switch", switched, yes, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &APIHandler{}
			h.SetItemFieldsGate(tc.gate)
			if got := h.itemPresetFor(context.Background(), "c1", tc.m) != nil; got != tc.want {
				t.Errorf("itemPresetFor = %v, want %v", got, tc.want)
			}
		})
	}
}
