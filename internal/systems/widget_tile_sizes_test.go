package systems

import (
	"encoding/json"
	"strings"
	"testing"
)

// A package declares the tile sizes of a widget as "tile_sizes" on its
// manifest widget entry, and the palette entry passes them through so a tile
// section can validate and draw the widget.
func TestWidgetTileSizes_PassThrough(t *testing.T) {
	var m SystemManifest
	raw := `{"widgets":[{"slug":"stat-block","name":"Stat block","script_file":"widgets/stat-block.js","tile_sizes":["2x2","4x2"]},{"slug":"plain","name":"Plain","script_file":"widgets/plain.js"}]}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	metas := placeableWidgetMetas(&m)
	if len(metas) != 2 {
		t.Fatalf("metas = %+v", metas)
	}
	if got := strings.Join(metas[0].TileSizes, ","); got != "2x2,4x2" {
		t.Errorf("stat-block tile sizes = %q", got)
	}
	if metas[1].TileSizes != nil {
		t.Errorf("a widget with no tile_sizes must not be a tile: %v", metas[1].TileSizes)
	}
}

func TestWidgetTileSizes_ManifestValidation(t *testing.T) {
	tests := []struct {
		name  string
		sizes []string
		bad   bool
	}{
		{"none", nil, false},
		{"valid", []string{"1x1", "2x1"}, false},
		{"malformed", []string{"wide"}, true},
		{"too wide", []string{"5x1"}, true},
		{"repeated", []string{"1x1", "1x1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &SystemManifest{
				ID: "sys", Name: "Sys", Version: "1.0.0", APIVersion: "1",
				Widgets: []WidgetDef{{Slug: "w", Name: "W", ScriptFile: "widgets/w.js", TileSizes: tt.sizes}},
			}
			err := ValidateManifest(m)
			if tt.bad {
				if err == nil || !strings.Contains(err.Error(), "tile_sizes") {
					t.Fatalf("error = %v, want a tile_sizes error", err)
				}
				return
			}
			if err != nil && strings.Contains(err.Error(), "tile_sizes") {
				t.Fatalf("unexpected tile_sizes error: %v", err)
			}
		})
	}
}
