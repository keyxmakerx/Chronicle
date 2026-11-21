package app

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// TestChecksByCategory pins that two systems disagreeing on one field's
// bounds leave it unchecked rather than letting either refuse the other's
// values.
func TestChecksByCategory(t *testing.T) {
	one, twenty := 1.0, 20.0
	ds := &systems.SystemManifest{EntityPresets: []systems.EntityPresetDef{{Category: "creature", Fields: []systems.FieldDef{
		{Key: "level", Min: &one, Max: &twenty}, {Key: "abilities_json", JSONList: true}, {Key: "ev", Formula: true},
	}}}}
	other := &systems.SystemManifest{EntityPresets: []systems.EntityPresetDef{{Category: "creature", Fields: []systems.FieldDef{
		{Key: "level"},
	}}}}
	cases := []struct {
		name      string
		manifests []*systems.SystemManifest
		wantLevel bool
	}{
		{"one system", []*systems.SystemManifest{ds}, true},
		{"conflict cancels", []*systems.SystemManifest{ds, other}, false},
		{"conflict cancels in either order", []*systems.SystemManifest{other, ds, ds}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checksByCategory(tc.manifests)
			lv, ok := got["creature"]["level"]
			if !ok {
				t.Fatal("level not recorded; a cleared check must still converge")
			}
			if (lv.Max != nil) != tc.wantLevel {
				t.Errorf("level max set=%v, want %v", lv.Max != nil, tc.wantLevel)
			}
			if !got["creature"]["abilities_json"].JSONList {
				t.Error("an uncontested check was dropped")
			}
			if !got["creature"]["ev"].Formula {
				t.Error("a formula figure was dropped")
			}
		})
	}
}
