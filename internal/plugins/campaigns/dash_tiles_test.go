package campaigns

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

type tileBuf struct{ b []byte }

func (t *tileBuf) Write(p []byte) (int, error) { t.b = append(t.b, p...); return len(p), nil }

func renderDashTiles(t *testing.T, role Role, tiles ...tilegrid.Tile) string {
	t.Helper()
	cc := &CampaignContext{Campaign: &Campaign{ID: "camp-1"}, MemberRole: role}
	layout := &DashboardLayout{Rows: []DashboardRow{{ID: "r", Kind: "tiles", Tiles: tiles}}}
	buf := &tileBuf{}
	if err := customDashboard(cc, layout, nil).Render(context.Background(), buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return string(buf.b)
}

func TestCustomDashboard_TileSection(t *testing.T) {
	withTileSizes(t, fakeTileSizes{"text_block": {"1x1", "2x2"}})
	text := map[string]any{"content": "<p>hello tile</p>"}

	tests := []struct {
		name    string
		role    Role
		tiles   []tilegrid.Tile
		want    []string
		notWant []string
	}{
		{
			name:  "a tile draws in its cell at its size",
			role:  RolePlayer,
			tiles: []tilegrid.Tile{{ID: "a", Type: "text_block", Size: "2x2", Config: text}},
			want:  []string{`class="tile-grid`, `data-tile-w="2"`, `data-tile-h="2"`, "hello tile"},
		},
		{
			name:    "a stored size the type dropped falls back to its first",
			role:    RolePlayer,
			tiles:   []tilegrid.Tile{{ID: "a", Type: "text_block", Size: "4x4", Config: text}},
			want:    []string{`data-tile-w="1"`, `data-tile-h="1"`, "hello tile"},
			notWant: []string{`data-tile-w="4"`},
		},
		{
			name:    "a type with no size shows the notice and the rest still draws",
			role:    RolePlayer,
			tiles:   []tilegrid.Tile{{ID: "a", Type: "quick_actions", Size: "1x1"}, {ID: "b", Type: "text_block", Size: "1x1", Config: text}},
			want:    []string{"Block not available here", "hello tile"},
			notWant: []string{"All Pages"},
		},
		{
			name:    "a DM-only tile is left out for a player",
			role:    RolePlayer,
			tiles:   []tilegrid.Tile{{ID: "a", Type: "text_block", Size: "1x1", Config: map[string]any{"content": "secret", "visibility": "dm_only"}}},
			notWant: []string{"secret", `data-tile-id="a"`},
		},
		{
			name:  "a DM-only tile shows for the owner",
			role:  RoleOwner,
			tiles: []tilegrid.Tile{{ID: "a", Type: "text_block", Size: "1x1", Config: map[string]any{"content": "secret", "visibility": "dm_only"}}},
			want:  []string{"secret"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderDashTiles(t, tt.role, tt.tiles...)
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
}
