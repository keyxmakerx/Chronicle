package entities

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// CheckCategoryDashboardLayout is the rules a category home's save meets, size
// included, without a write.
func TestCheckCategoryDashboardLayout(t *testing.T) {
	svc := newTestService(&mockEntityRepo{}, &mockEntityTypeRepo{})
	block := func(typ string) campaigns.DashboardBlock { return campaigns.DashboardBlock{ID: "b", Type: typ} }
	row := func(w int, blocks ...campaigns.DashboardBlock) campaigns.DashboardRow {
		return campaigns.DashboardRow{ID: "r", Columns: []campaigns.DashboardColumn{{ID: "c", Width: w, Blocks: blocks}}}
	}
	tests := []struct {
		name    string
		layout  campaigns.DashboardLayout
		wantErr string
	}{
		{"a valid home", campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{row(12, block("entity_grid"))}}, ""},
		{"an unknown block", campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{row(12, block("dragon"))}}, "invalid block type"},
		{"a column too wide", campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{row(13, block("entity_grid"))}}, "column width"},
		{"too big", campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{{ID: "r", Columns: []campaigns.DashboardColumn{{ID: "c", Width: 12, Blocks: []campaigns.DashboardBlock{
			{ID: "b", Type: "text_block", Config: map[string]any{"content": strings.Repeat("x", maxLayoutSize+1)}},
		}}}}}}, "too big"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.CheckCategoryDashboardLayout(context.Background(), 1, tt.layout)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
