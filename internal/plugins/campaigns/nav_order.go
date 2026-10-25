package campaigns

import (
	"context"
	"sort"
	"strconv"
)

// NavCategoryPlace is one top-level category where the sidebar draws it: its
// section's id and title.
type NavCategoryPlace struct {
	TypeID       int
	Section      string
	SectionTitle string
}

// NavCategoryOrder lists every known category in the order the sidebar draws
// them, section by section, with where each sits.
func NavCategoryOrder(items []SidebarItem, apps []NavApp, cats []NavCategory) []NavCategoryPlace {
	var out []NavCategoryPlace
	for _, s := range NormalizeNav(items, apps, cats).Sections {
		for _, it := range s.Items {
			if id, ok := navCategoryID(it.Key); ok {
				out = append(out, NavCategoryPlace{TypeID: id, Section: s.ID, SectionTitle: navSectionTitle(s)})
			}
		}
	}
	return out
}

func navCategoryID(key string) (int, bool) {
	kind, ref, ok := NavKeyKind(key)
	if !ok || kind != NavRowCategory {
		return 0, false
	}
	id, err := strconv.Atoi(ref)
	return id, err == nil
}

// OrderNavCategories returns items with the categories in order (first wanted
// first) rearranged among themselves inside the section each sits in. Every
// other row, and every category order leaves out, keeps its place, so the
// owner's sections, apps and links are untouched and no category changes
// section.
//
// A category the owner never arranged has no row, so one is written where the
// sidebar already draws it (the end of Categories) before the order applies.
// A sidebar never arranged at all also gets its default-pinned apps written:
// once any row is stored the sidebar treats the layout as arranged and would
// otherwise move those apps out of Pinned.
func OrderNavCategories(items []SidebarItem, apps []NavApp, cats []NavCategory, order []int) []SidebarItem {
	out := append([]SidebarItem(nil), items...)
	if len(out) == 0 {
		for _, a := range apps {
			if a.DefaultPinned {
				out = append(out, SidebarItem{Type: SidebarTypeApp, Slug: a.Slug, Visible: true, Section: NavSectionPinned})
			}
		}
	}
	has := map[int]bool{}
	for _, it := range out {
		if it.Type == SidebarTypeCategory {
			has[it.TypeID] = true
		}
	}
	for _, c := range cats {
		if !has[c.TypeID] {
			has[c.TypeID] = true
			out = append(out, SidebarItem{Type: SidebarTypeCategory, TypeID: c.TypeID, Visible: true, Section: NavSectionCategories})
		}
	}

	section := map[int]string{}
	for _, p := range NavCategoryOrder(out, apps, cats) {
		section[p.TypeID] = p.Section
	}
	rank := map[int]int{}
	for i, id := range order {
		if _, dup := rank[id]; !dup {
			rank[id] = i
		}
	}
	// Each section's slots are the rows of the categories being ordered, in
	// row order; only a category's first row counts, as in NormalizeNav.
	slots := map[string][]int{}
	first := map[int]bool{}
	for i, it := range out {
		if it.Type != SidebarTypeCategory || first[it.TypeID] {
			continue
		}
		first[it.TypeID] = true
		if _, ok := rank[it.TypeID]; ok {
			if s, drawn := section[it.TypeID]; drawn {
				slots[s] = append(slots[s], i)
			}
		}
	}
	result := append([]SidebarItem(nil), out...)
	for _, rows := range slots {
		moved := make([]SidebarItem, len(rows))
		for j, r := range rows {
			moved[j] = out[r]
		}
		sort.SliceStable(moved, func(a, b int) bool { return rank[moved[a].TypeID] < rank[moved[b].TypeID] })
		for j, r := range rows {
			result[r] = moved[j]
		}
	}
	return result
}

// OrderNavCategories writes a new order for the sidebar's categories (see the
// function of the same name). apps and cats are what the sidebar is built
// from, as NormalizeNav takes them.
func (s *campaignService) OrderNavCategories(ctx context.Context, campaignID string, apps []NavApp, cats []NavCategory, order []int) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	items := OrderNavCategories(sidebarConfigForWrite(campaign).Items, apps, cats, order)
	return s.UpdateSidebarConfig(ctx, campaignID, UpdateSidebarConfigRequest{Items: &items})
}
