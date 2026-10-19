package entities

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// fakeWidgetLister serves extension widget metadata for tile-size lookups.
type fakeWidgetLister struct{ metas []BlockMeta }

func (f fakeWidgetLister) GetWidgetBlockMetas(context.Context, string) []BlockMeta { return f.metas }

// tileTestRegistry registers a tileable probe block, a block that cannot be a
// tile, and the ext_widget type with one sized and one unsized widget.
func tileTestRegistry() *BlockRegistry {
	r := NewBlockRegistry()
	probe := func(ctx BlockRenderContext) templ.Component {
		return templ.Raw(`<p data-probe="` + ctx.Block.ID + `">probe</p>`)
	}
	r.Register(BlockMeta{Type: "probe", Label: "Probe", TileSizes: []string{"1x1", "2x2"}}, probe)
	r.Register(BlockMeta{Type: "plain", Label: "Plain"}, probe)
	r.Register(BlockMeta{Type: "ext_widget", Label: "Extension Widget"}, probe)
	r.SetWidgetLister(fakeWidgetLister{metas: []BlockMeta{
		{Type: "ext_widget", WidgetSlug: "sky", TileSizes: []string{"1x1", "2x1"}},
		{Type: "ext_widget", WidgetSlug: "fixed"},
	}})
	return r
}

func TestTileSizesFor(t *testing.T) {
	r := tileTestRegistry()
	tests := []struct {
		name   string
		typ    string
		config map[string]any
		want   []string
	}{
		{"block declaring sizes", "probe", nil, []string{"1x1", "2x2"}},
		{"block declaring none", "plain", nil, nil},
		{"unregistered type", "gone", nil, nil},
		{"extension widget with sizes", "ext_widget", map[string]any{"widget_slug": "sky"}, []string{"1x1", "2x1"}},
		{"extension widget without sizes", "ext_widget", map[string]any{"widget_slug": "fixed"}, nil},
		{"extension widget no longer installed", "ext_widget", map[string]any{"widget_slug": "removed"}, nil},
		{"extension widget with no slug", "ext_widget", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.TileSizesFor(context.Background(), "camp-1", tt.typ, tt.config)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func tileRow(kind string, cols []TemplateColumn, tiles ...tilegrid.Tile) EntityTypeLayout {
	return EntityTypeLayout{Rows: []TemplateRow{{ID: "r1", Kind: kind, Columns: cols, Tiles: tiles}}}
}

func TestValidateLayout_TileSections(t *testing.T) {
	r := tileTestRegistry()
	sizes := func(typ string, cfg map[string]any) []string {
		return r.TileSizesFor(context.Background(), "camp-1", typ, cfg)
	}
	valid := r.IsValid
	oneCol := []TemplateColumn{{ID: "c1", Width: 12}}

	tests := []struct {
		name    string
		layout  EntityTypeLayout
		wantErr string
	}{
		{"ordinary row is unchanged", EntityTypeLayout{Rows: []TemplateRow{{ID: "r1", Columns: oneCol}}}, ""},
		{"valid tile section", tileRow("tiles", nil, tilegrid.Tile{ID: "t1", Type: "probe", Size: "2x2"}), ""},
		{"empty tile section", tileRow("tiles", nil), ""},
		{"extension widget tile in its declared size", tileRow("tiles", nil,
			tilegrid.Tile{ID: "t1", Type: "ext_widget", Size: "2x1", Config: map[string]any{"widget_slug": "sky"}}), ""},
		{"tile section with columns", tileRow("tiles", oneCol, tilegrid.Tile{ID: "t1", Type: "probe", Size: "1x1"}), "both columns and tiles"},
		{"tiles on an ordinary row", tileRow("", oneCol, tilegrid.Tile{ID: "t1", Type: "probe", Size: "1x1"}), "not a tile section"},
		{"tiles with no kind and no columns", tileRow("", nil, tilegrid.Tile{ID: "t1", Type: "probe", Size: "1x1"}), "not a tile section"},
		{"unknown kind", tileRow("masonry", nil), "unknown kind"},
		{"type that cannot be a tile", tileRow("tiles", nil, tilegrid.Tile{ID: "t1", Type: "plain", Size: "1x1"}), "can't be shown as a tile"},
		{"size outside the type's list", tileRow("tiles", nil, tilegrid.Tile{ID: "t1", Type: "probe", Size: "2x1"}), "Allowed sizes: 1x1, 2x2"},
		{"malformed size", tileRow("tiles", nil, tilegrid.Tile{ID: "t1", Type: "probe", Size: "big"}), "invalid size"},
		{"missing size", tileRow("tiles", nil, tilegrid.Tile{ID: "t1", Type: "probe"}), "invalid size"},
		{"unknown block type", tileRow("tiles", nil, tilegrid.Tile{ID: "t1", Type: "gone", Size: "1x1"}), "unknown block type"},
		{"extension widget in a size it does not declare", tileRow("tiles", nil,
			tilegrid.Tile{ID: "t1", Type: "ext_widget", Size: "2x2", Config: map[string]any{"widget_slug": "sky"}}), "Allowed sizes: 1x1, 2x1"},
		{"extension widget with no tile sizes", tileRow("tiles", nil,
			tilegrid.Tile{ID: "t1", Type: "ext_widget", Size: "1x1", Config: map[string]any{"widget_slug": "fixed"}}), "can't be shown as a tile"},
		{"tile id repeats a block id", EntityTypeLayout{Rows: []TemplateRow{
			{ID: "r1", Columns: []TemplateColumn{{ID: "c1", Width: 12, Blocks: []TemplateBlock{{ID: "x", Type: "probe"}}}}},
			{ID: "r2", Kind: "tiles", Tiles: []tilegrid.Tile{{ID: "x", Type: "probe", Size: "1x1"}}},
		}}, "same block twice"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLayout(tt.layout, valid, nil, sizes)
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

// A layout saved before tile sections existed has neither field, and one with
// tiles round-trips through JSON unchanged.
func TestTemplateRow_JSONCompatibility(t *testing.T) {
	var old TemplateRow
	if err := json.Unmarshal([]byte(`{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[]}]}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.Kind != "" || old.Tiles != nil {
		t.Errorf("old row picked up tile fields: %+v", old)
	}
	raw, _ := json.Marshal(old)
	if strings.Contains(string(raw), `"kind"`) || strings.Contains(string(raw), `"tiles"`) {
		t.Errorf("an ordinary row must not serialize tile fields: %s", raw)
	}

	in := `{"id":"r2","kind":"tiles","columns":null,"tiles":[{"id":"t1","type":"probe","size":"2x2","config":{"visibility":"dm_only"}}]}`
	var row TemplateRow
	if err := json.Unmarshal([]byte(in), &row); err != nil {
		t.Fatal(err)
	}
	if row.Kind != "tiles" || len(row.Tiles) != 1 || row.Tiles[0].Size != "2x2" || row.Tiles[0].Config["visibility"] != "dm_only" {
		t.Errorf("tile row decoded wrong: %+v", row)
	}
}

func renderTileSection(t *testing.T, row TemplateRow, role campaigns.Role) string {
	t.Helper()
	old := GetGlobalBlockRegistry()
	SetGlobalBlockRegistry(tileTestRegistry())
	t.Cleanup(func() { SetGlobalBlockRegistry(old) })

	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: role}
	buf := &bytesBufferLike{}
	err := entityTileSection(row, cc, &Entity{ID: "e1"}, &EntityType{}, true, "csrf").Render(context.Background(), buf)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestEntityTileSection_Render(t *testing.T) {
	tests := []struct {
		name    string
		tiles   []tilegrid.Tile
		role    campaigns.Role
		want    []string
		notWant []string
	}{
		{
			name:  "a tile gets its size attributes and its block",
			tiles: []tilegrid.Tile{{ID: "t1", Type: "probe", Size: "2x2"}},
			role:  campaigns.RolePlayer,
			want:  []string{`class="tile-grid`, `data-tile-id="t1"`, `data-tile-w="2"`, `data-tile-h="2"`, `data-probe="t1"`},
		},
		{
			name:    "a stored size the type dropped falls back to its first size",
			tiles:   []tilegrid.Tile{{ID: "t1", Type: "probe", Size: "4x3"}},
			role:    campaigns.RolePlayer,
			want:    []string{`data-tile-w="1"`, `data-tile-h="1"`, `data-probe="t1"`},
			notWant: []string{`data-tile-w="4"`},
		},
		{
			name:    "a type that now allows no size shows the notice and the page still renders",
			tiles:   []tilegrid.Tile{{ID: "t1", Type: "plain", Size: "1x1"}, {ID: "t2", Type: "probe", Size: "1x1"}},
			role:    campaigns.RolePlayer,
			want:    []string{"Block not available here", `data-probe="t2"`},
			notWant: []string{`data-probe="t1"`},
		},
		{
			name:    "a removed block type shows the notice",
			tiles:   []tilegrid.Tile{{ID: "t1", Type: "gone", Size: "1x1"}},
			role:    campaigns.RolePlayer,
			want:    []string{"Block not available here"},
			notWant: []string{`data-probe`},
		},
		{
			name:    "a DM-only tile is left out for a player, cell and all",
			tiles:   []tilegrid.Tile{{ID: "t1", Type: "probe", Size: "1x1", Config: map[string]any{"visibility": "dm_only"}}},
			role:    campaigns.RolePlayer,
			notWant: []string{`data-tile-id="t1"`, `data-probe`},
		},
		{
			name:  "a DM-only tile shows for the owner with its badge",
			tiles: []tilegrid.Tile{{ID: "t1", Type: "probe", Size: "1x1", Config: map[string]any{"visibility": "dm_only"}}},
			role:  campaigns.RoleOwner,
			want:  []string{`data-tile-id="t1"`, "GM Only", `data-probe="t1"`},
		},
		{
			name: "an extension widget tile takes the size its manifest declares",
			tiles: []tilegrid.Tile{{ID: "t1", Type: "ext_widget", Size: "2x1",
				Config: map[string]any{"widget_slug": "sky"}}},
			role: campaigns.RolePlayer,
			want: []string{`data-tile-w="2"`, `data-tile-h="1"`},
		},
		{
			name: "tiles keep their stored order",
			tiles: []tilegrid.Tile{
				{ID: "first", Type: "probe", Size: "1x1"},
				{ID: "second", Type: "probe", Size: "1x1"},
			},
			role: campaigns.RolePlayer,
			want: []string{`data-tile-id="first"`, `data-tile-id="second"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderTileSection(t, TemplateRow{ID: "r1", Kind: "tiles", Tiles: tt.tiles}, tt.role)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in:\n%s", w, got)
				}
			}
		})
	}

	t.Run("core block sizes are well formed", func(t *testing.T) {
		// Every size a core block declares must be one the grid can draw, and a
		// container can never be a tile (its children live in free-form config).
		r := NewBlockRegistry()
		RegisterCoreBlocks(r)
		tileable := 0
		for _, m := range r.Types() {
			if err := tilegrid.ValidateSizeList(m.TileSizes); err != nil {
				t.Errorf("%s: %v", m.Type, err)
			}
			if m.Container && len(m.TileSizes) > 0 {
				t.Errorf("%s is a container and cannot be a tile", m.Type)
			}
			if len(m.TileSizes) > 0 {
				tileable++
			}
		}
		if tileable == 0 {
			t.Error("no core block can be a tile")
		}
	})

	t.Run("order", func(t *testing.T) {
		got := renderTileSection(t, TemplateRow{ID: "r1", Kind: "tiles", Tiles: []tilegrid.Tile{
			{ID: "first", Type: "probe", Size: "1x1"}, {ID: "second", Type: "probe", Size: "1x1"},
		}}, campaigns.RolePlayer)
		if strings.Index(got, `"first"`) > strings.Index(got, `"second"`) {
			t.Errorf("tiles out of order:\n%s", got)
		}
	})
}
