package entities

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// TestPageEditorState_DropsRemovedBlocks pins that the Customizer never
// shows a block type Chronicle removed: a stored layout's old permissions
// block is left out, a row it alone filled goes with it, and everything
// else is kept as stored.
func TestPageEditorState_DropsRemovedBlocks(t *testing.T) {
	blk := func(id, typ string) TemplateBlock { return TemplateBlock{ID: id, Type: typ} }
	tests := []struct {
		name     string
		layout   EntityTypeLayout
		wantRows []string
		wantIDs  []string
	}{
		{
			name: "auto-added permissions row is dropped whole",
			layout: EntityTypeLayout{Rows: []TemplateRow{
				{ID: "row-1", Columns: []TemplateColumn{{ID: "c1", Width: 12, Blocks: []TemplateBlock{blk("b-entry", "entry")}}}},
				{ID: "row-perm", Columns: []TemplateColumn{{ID: "col-perm", Width: 12, Blocks: []TemplateBlock{blk("blk-perm", "permissions")}}}},
			}},
			wantRows: []string{"row-1"},
			wantIDs:  []string{"b-entry"},
		},
		{
			name: "permissions beside other blocks leaves the row and its columns",
			layout: EntityTypeLayout{Rows: []TemplateRow{
				{ID: "row-1", Columns: []TemplateColumn{
					{ID: "c1", Width: 8, Blocks: []TemplateBlock{blk("b-title", "title"), blk("b-perm", "permissions")}},
					{ID: "c2", Width: 4, Blocks: []TemplateBlock{blk("b-perm2", "permissions")}},
				}},
			}},
			wantRows: []string{"row-1"},
			wantIDs:  []string{"b-title"},
		},
		{
			name: "permissions tile is dropped from a tile section",
			layout: EntityTypeLayout{Rows: []TemplateRow{
				{ID: "row-t", Kind: tilegrid.KindTiles, Tiles: []tilegrid.Tile{{ID: "t-tags", Type: "tags", Size: "1x1"}, {ID: "t-perm", Type: "permissions", Size: "1x1"}}},
			}},
			wantRows: []string{"row-t"},
			wantIDs:  []string{"t-tags"},
		},
		{
			name: "a row that was already empty stays",
			layout: EntityTypeLayout{Rows: []TemplateRow{
				{ID: "row-empty", Columns: []TemplateColumn{{ID: "c1", Width: 12}}},
			}},
			wantRows: []string{"row-empty"},
		},
	}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := pageEditorState(cc, &EntityType{ID: 7, Layout: tt.layout})
			if strings.Contains(raw, `"permissions"`) {
				t.Fatalf("Customizer state still carries a permissions block: %s", raw)
			}
			var st struct {
				Layout EntityTypeLayout `json:"layout"`
			}
			if err := json.Unmarshal([]byte(raw), &st); err != nil {
				t.Fatal(err)
			}
			var rows, ids []string
			for _, r := range st.Layout.Rows {
				rows = append(rows, r.ID)
				for _, tl := range r.Tiles {
					ids = append(ids, tl.ID)
				}
				for _, c := range r.Columns {
					for _, b := range c.Blocks {
						ids = append(ids, b.ID)
					}
				}
			}
			if strings.Join(rows, ",") != strings.Join(tt.wantRows, ",") {
				t.Errorf("rows = %v, want %v", rows, tt.wantRows)
			}
			if strings.Join(ids, ",") != strings.Join(tt.wantIDs, ",") {
				t.Errorf("blocks = %v, want %v", ids, tt.wantIDs)
			}
		})
	}
}
