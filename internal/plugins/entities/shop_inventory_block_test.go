package entities

import "testing"

func TestNeedsImplicitShopInventory(t *testing.T) {
	withBlock := EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{
		Blocks: []TemplateBlock{{ID: "b", Type: "shop_inventory"}},
	}}}}}
	nested := EntityTypeLayout{Rows: []TemplateRow{{Columns: []TemplateColumn{{
		Blocks: []TemplateBlock{{ID: "t", Type: "tabs", Config: map[string]any{
			"tabs": []any{map[string]any{"blocks": []any{map[string]any{"id": "x", "type": "shop_inventory"}}}},
		}}},
	}}}}}

	tests := []struct {
		name   string
		entity *Entity
		et     *EntityType
		want   bool
	}{
		{"shop with default layout", &Entity{TypeSlug: "shop"}, &EntityType{Layout: DefaultLayout()}, true},
		{"shop placing the block", &Entity{TypeSlug: "shop"}, &EntityType{Layout: withBlock}, false},
		{"shop placing the block in a container", &Entity{TypeSlug: "shop"}, &EntityType{Layout: nested}, false},
		{"shop with no type loaded", &Entity{TypeSlug: "shop"}, nil, true},
		{"non-shop", &Entity{TypeSlug: "npc"}, &EntityType{Layout: DefaultLayout()}, false},
		{"nil entity", nil, &EntityType{Layout: DefaultLayout()}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsImplicitShopInventory(tt.entity, tt.et); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
