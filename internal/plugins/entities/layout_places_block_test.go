package entities

import "testing"

func TestLayoutPlacesBlock(t *testing.T) {
	top := EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{
		Blocks: []TemplateBlock{{ID: "b", Type: "shop_inventory"}},
	}}}}}
	nested := EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{
		Blocks: []TemplateBlock{{ID: "t", Type: "tabs", Config: map[string]any{
			"tabs": []any{map[string]any{"blocks": []any{map[string]any{"id": "x", "type": "shop_inventory"}}}},
		}}},
	}}}}}

	tests := []struct {
		name   string
		layout EntityTypeLayout
		want   bool
	}{
		{"placed at the top level", top, true},
		{"placed inside a container", nested, true},
		{"not placed", DefaultLayout(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutPlacesBlock(tt.layout, "shop_inventory"); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
