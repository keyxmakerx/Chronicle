package campaigns

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// fakeTileSizes stands in for the block registry.
type fakeTileSizes map[string][]string

func (f fakeTileSizes) TileSizesFor(_ context.Context, _, blockType string, _ map[string]any) []string {
	return f[blockType]
}

func withTileSizes(t *testing.T, sizes fakeTileSizes) {
	t.Helper()
	SetTileSizeResolver(sizes)
	t.Cleanup(func() { SetTileSizeResolver(nil) })
}

func TestValidateDashboardLayout_TileSections(t *testing.T) {
	withTileSizes(t, fakeTileSizes{
		"text_block":    {"1x1", "2x2"},
		"coming_up":     {"2x2"},
		"quick_actions": nil,
	})
	col := []DashboardColumn{{ID: "c1", Width: 12}}
	tile := func(id, typ, size string) tilegrid.Tile { return tilegrid.Tile{ID: id, Type: typ, Size: size} }

	tests := []struct {
		name    string
		rows    []DashboardRow
		wantErr string
	}{
		{"ordinary row is unchanged", []DashboardRow{{ID: "r", Columns: col}}, ""},
		{"valid tile section", []DashboardRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{tile("a", "text_block", "2x2"), tile("b", "coming_up", "2x2")}}}, ""},
		{"empty tile section", []DashboardRow{{ID: "r", Kind: "tiles"}}, ""},
		{"columns and tiles", []DashboardRow{{ID: "r", Kind: "tiles", Columns: col, Tiles: []tilegrid.Tile{tile("a", "text_block", "1x1")}}}, "both columns and tiles"},
		{"tiles on an ordinary row", []DashboardRow{{ID: "r", Tiles: []tilegrid.Tile{tile("a", "text_block", "1x1")}}}, "tile section"},
		{"unknown kind", []DashboardRow{{ID: "r", Kind: "masonry"}}, "unknown kind"},
		{"type with no tile sizes", []DashboardRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{tile("a", "quick_actions", "1x1")}}}, "can't be shown as a tile"},
		{"size outside the type's list", []DashboardRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{tile("a", "text_block", "2x1")}}}, "Allowed sizes: 1x1, 2x2"},
		{"malformed size", []DashboardRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{tile("a", "text_block", "2by2")}}}, "invalid size"},
		{"unsupported block type", []DashboardRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{tile("a", "not_a_block", "1x1")}}}, "can't be shown as a tile"},
		{"repeated tile id across rows", []DashboardRow{
			{ID: "r1", Kind: "tiles", Tiles: []tilegrid.Tile{tile("a", "text_block", "1x1")}},
			{ID: "r2", Kind: "tiles", Tiles: []tilegrid.Tile{tile("a", "text_block", "1x1")}},
		}, "same block twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDashboardLayout(context.Background(), "camp-1", &DashboardLayout{Rows: tt.rows})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// A text block is sanitized on save whether it sits in a column or a tile.
func TestValidateDashboardLayout_SanitizesTileText(t *testing.T) {
	withTileSizes(t, fakeTileSizes{"text_block": {"1x1"}})
	layout := &DashboardLayout{Rows: []DashboardRow{{ID: "r", Kind: "tiles", Tiles: []tilegrid.Tile{{
		ID: "a", Type: "text_block", Size: "1x1",
		Config: map[string]any{"content": `<p>hi</p><script>alert(1)</script>`},
	}}}}}
	if err := validateDashboardLayout(context.Background(), "camp-1", layout); err != nil {
		t.Fatal(err)
	}
	got, _ := layout.Rows[0].Tiles[0].Config["content"].(string)
	if strings.Contains(got, "<script") || !strings.Contains(got, "hi") {
		t.Errorf("content = %q", got)
	}
}

func TestDashboardRow_JSONCompatibility(t *testing.T) {
	var old DashboardRow
	if err := json.Unmarshal([]byte(`{"id":"r","columns":[{"id":"c","width":12,"blocks":[]}]}`), &old); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(old)
	if strings.Contains(string(raw), `"kind"`) || strings.Contains(string(raw), `"tiles"`) {
		t.Errorf("an ordinary row must not serialize tile fields: %s", raw)
	}
}

func TestResolveDashboardTile(t *testing.T) {
	withTileSizes(t, fakeTileSizes{"text_block": {"1x1", "2x2"}})
	tests := []struct {
		name string
		tile tilegrid.Tile
		w, h int
		ok   bool
	}{
		{"stored size allowed", tilegrid.Tile{Type: "text_block", Size: "2x2"}, 2, 2, true},
		{"stored size dropped falls back to first", tilegrid.Tile{Type: "text_block", Size: "4x1"}, 1, 1, true},
		{"type allows none", tilegrid.Tile{Type: "quick_actions", Size: "1x1"}, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, ok := ResolveDashboardTile(context.Background(), "camp-1", tt.tile)
			if w != tt.w || h != tt.h || ok != tt.ok {
				t.Errorf("got %d,%d,%v want %d,%d,%v", w, h, ok, tt.w, tt.h, tt.ok)
			}
		})
	}
}

func TestComingUpRepeats_CountsTiles(t *testing.T) {
	l := &DashboardLayout{Rows: []DashboardRow{
		{ID: "r0", Columns: []DashboardColumn{{ID: "c", Width: 12, Blocks: []DashboardBlock{{ID: "b", Type: BlockComingUp}}}}},
		{ID: "r1", Kind: "tiles", Tiles: []tilegrid.Tile{{ID: "t", Type: BlockComingUp, Size: "2x2"}}},
	}}
	got := l.ComingUpRepeats()
	if !got[[3]int{1, -1, 0}] || len(got) != 1 {
		t.Errorf("repeats = %v, want only the tile marked", got)
	}
}
