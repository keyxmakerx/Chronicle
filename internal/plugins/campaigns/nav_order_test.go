package campaigns

import (
	"reflect"
	"testing"
)

func TestOrderNavCategories(t *testing.T) {
	apps := []NavApp{{Slug: "pinned-app", DefaultPinned: true, Enabled: true}, {Slug: "other-app", Enabled: true}}
	cats := []NavCategory{{TypeID: 1}, {TypeID: 2}, {TypeID: 3}, {TypeID: 4}}
	cat := func(id int, section string) SidebarItem {
		return SidebarItem{Type: SidebarTypeCategory, TypeID: id, Visible: true, Section: section}
	}
	link := SidebarItem{Type: SidebarTypeLink, ID: "wiki", Label: "Wiki", URL: "https://example.com", Visible: true}
	mine := SidebarItem{Type: SidebarTypeSection, ID: "lore", Label: "Lore", Visible: true}
	tests := []struct {
		name  string
		items []SidebarItem
		order []int
		want  []int // categories as the sidebar draws them afterwards
	}{
		{"never arranged", nil, []int{3, 1}, []int{3, 2, 1, 4}},
		{"whole order", []SidebarItem{cat(1, ""), cat(2, ""), cat(3, ""), cat(4, "")}, []int{4, 3, 2, 1}, []int{4, 3, 2, 1}},
		{"unlisted keep their place", []SidebarItem{cat(1, ""), cat(2, ""), cat(3, ""), cat(4, "")}, []int{4, 1}, []int{4, 2, 3, 1}},
		{"categories without a row come in at the end", []SidebarItem{cat(2, "")}, []int{4, 2}, []int{4, 1, 3, 2}},
		{"each stays in its own section",
			[]SidebarItem{mine, cat(1, "lore"), cat(2, "lore"), cat(3, NavSectionCategories), cat(4, NavSectionCategories)},
			[]int{4, 2, 3, 1}, []int{4, 3, 2, 1}},
		{"repeats and unknown ids ignored", []SidebarItem{cat(1, ""), cat(2, ""), cat(3, ""), cat(4, "")}, []int{9, 2, 2, 1}, []int{2, 1, 3, 4}},
		{"other rows untouched", []SidebarItem{link, cat(1, ""), cat(2, "")}, []int{2, 1}, []int{2, 1, 3, 4}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := OrderNavCategories(tc.items, apps, cats, tc.order)
			var drawn []int
			for _, p := range NavCategoryOrder(got, apps, cats) {
				drawn = append(drawn, p.TypeID)
			}
			if !reflect.DeepEqual(drawn, tc.want) {
				t.Errorf("drawn %v, want %v", drawn, tc.want)
			}
			before := NormalizeNav(tc.items, apps, cats)
			after := NormalizeNav(got, apps, cats)
			for i, s := range before.Sections {
				if len(after.Sections) <= i || after.Sections[i].ID != s.ID {
					t.Fatalf("sections changed: %+v", after.Sections)
				}
				var b, a []string
				for _, it := range s.Items {
					if _, ok := navCategoryID(it.Key); !ok {
						b = append(b, it.Key)
					}
				}
				for _, it := range after.Sections[i].Items {
					if _, ok := navCategoryID(it.Key); !ok {
						a = append(a, it.Key)
					}
				}
				if !reflect.DeepEqual(a, b) {
					t.Errorf("section %s rows other than categories changed: %v, was %v", s.ID, a, b)
				}
			}
		})
	}
}
