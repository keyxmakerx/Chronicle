package campaigns

import (
	"reflect"
	"testing"
)

func TestRemapSidebarIDs(t *testing.T) {
	idMap := NewIDMap("new")
	idMap.EntityTypeIDs[10] = 110
	idMap.EntityTypeIDs[20] = 120
	idMap.EntityIDs["old-page"] = "new-page"

	tests := []struct {
		name       string
		in         SidebarConfig
		wantItems  []SidebarItem
		wantHidden []string
	}{
		{
			name: "categories take their new ids and keep order and visibility",
			in: SidebarConfig{Items: []SidebarItem{
				{Type: SidebarTypeCategory, TypeID: 20, Visible: false},
				{Type: SidebarTypeCategory, TypeID: 10, Visible: true},
			}},
			wantItems: []SidebarItem{
				{Type: SidebarTypeCategory, TypeID: 120, Visible: false},
				{Type: SidebarTypeCategory, TypeID: 110, Visible: true},
			},
			wantHidden: []string{},
		},
		{
			name: "a category that did not come back is dropped; other items stay",
			in: SidebarConfig{Items: []SidebarItem{
				{Type: SidebarTypeApp, Slug: "maps", Visible: true},
				{Type: SidebarTypeCategory, TypeID: 99, Visible: true},
				{Type: SidebarTypeSection, ID: "lore", Label: "Lore", Visible: true},
			}},
			wantItems: []SidebarItem{
				{Type: SidebarTypeApp, Slug: "maps", Visible: true},
				{Type: SidebarTypeSection, ID: "lore", Label: "Lore", Visible: true},
			},
			wantHidden: []string{},
		},
		{
			name:       "hidden pages take their new ids; missing ones are dropped",
			in:         SidebarConfig{Items: []SidebarItem{}, HiddenEntityIDs: []string{"gone", "old-page"}},
			wantItems:  []SidebarItem{},
			wantHidden: []string{"new-page"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.in
			remapSidebarIDs(&cfg, idMap)
			if !reflect.DeepEqual(cfg.Items, tt.wantItems) {
				t.Errorf("items = %+v, want %+v", cfg.Items, tt.wantItems)
			}
			if len(cfg.HiddenEntityIDs) != len(tt.wantHidden) || (len(tt.wantHidden) > 0 && !reflect.DeepEqual(cfg.HiddenEntityIDs, tt.wantHidden)) {
				t.Errorf("hidden = %v, want %v", cfg.HiddenEntityIDs, tt.wantHidden)
			}
		})
	}
}
