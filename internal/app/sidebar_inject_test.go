// sidebar_inject_test.go — C-NAV-V3: the render-side default injector that lets
// the single unified items model render the full default sidebar for an empty
// (never-customized or reconciler-skipped) config, plus the auto-add path that
// now reaches converted campaigns.
package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

func catTypes(ids ...int) []layouts.SidebarEntityType {
	out := make([]layouts.SidebarEntityType, 0, len(ids))
	for _, id := range ids {
		out = append(out, layouts.SidebarEntityType{ID: id})
	}
	return out
}

func itemTypes(items []campaigns.SidebarItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Type
	}
	return out
}

// TestInjectDefaultSidebarItems_EmptyRendersFullDefault: an empty config
// synthesizes the full default — Dashboard, the addon shortcuts, every
// top-level category, and All Pages — with sub-category types excluded.
func TestInjectDefaultSidebarItems_EmptyRendersFullDefault(t *testing.T) {
	parent := 10
	types := []layouts.SidebarEntityType{
		{ID: 10}, {ID: 20}, {ID: 30, ParentTypeID: &parent}, // 30 is a sub-type
	}
	got := injectDefaultSidebarItems(nil, types)

	// dashboard, addon(notes), addon(calendar), category(10), category(20), all_pages
	wantTypes := []string{"dashboard", "addon", "addon", "category", "category", "all_pages"}
	if gotTypes := itemTypes(got); !equalStr(gotTypes, wantTypes) {
		t.Fatalf("item types = %v, want %v", gotTypes, wantTypes)
	}
	if got[0].Type != "dashboard" {
		t.Errorf("dashboard must be first, got %v", got[0])
	}
	if got[len(got)-1].Type != "all_pages" {
		t.Errorf("all_pages must be last, got %v", got[len(got)-1])
	}
	// The sub-type (30) must never appear as a category item.
	for _, it := range got {
		if it.Type == "category" && it.TypeID == 30 {
			t.Errorf("sub-category type 30 must not be a sidebar item")
		}
	}
	// Addon shortcuts match the injector's own registration list (the source of
	// truth), derived here rather than hardcoding plugin slugs: internal/app is
	// not the owning plugin directory, so literal plugin names in this test would
	// leak across the plugin boundary and trip check-plugin-isolation.sh
	// (0e / T-B2). defaultSidebarAddons lives in the allowlisted app wiring.
	wantAddons := map[string]bool{}
	for _, a := range defaultSidebarAddons {
		wantAddons[a.Slug] = true
	}
	gotAddons := map[string]bool{}
	for _, it := range got {
		if it.Type == "addon" {
			gotAddons[it.Slug] = true
		}
	}
	if !reflect.DeepEqual(gotAddons, wantAddons) {
		t.Errorf("default addons = %v, want %v (from defaultSidebarAddons)", gotAddons, wantAddons)
	}
}

// TestInjectDefaultSidebarItems_PreservesCustomOrderAppendsNew: an explicit
// (customized) order is preserved; a newly created type is appended (not
// reordered to the front), and the scaffold is added exactly once.
func TestInjectDefaultSidebarItems_PreservesCustomOrderAppendsNew(t *testing.T) {
	// Operator put 20 before 10; 30 is newly created (not in the config).
	existing := []campaigns.SidebarItem{
		{Type: "category", TypeID: 20, Visible: true},
		{Type: "category", TypeID: 10, Visible: true},
	}
	got := injectDefaultSidebarItems(existing, catTypes(10, 20, 30))

	// Extract category ids in order.
	var catOrder []int
	dashboards, allPages := 0, 0
	for _, it := range got {
		switch it.Type {
		case "category":
			catOrder = append(catOrder, it.TypeID)
		case "dashboard":
			dashboards++
		case "all_pages":
			allPages++
		}
	}
	// Custom order preserved (20 before 10), new type appended last.
	want := []int{20, 10, 30}
	if !equalInt(catOrder, want) {
		t.Errorf("category order = %v, want %v (custom order preserved, new appended)", catOrder, want)
	}
	if dashboards != 1 || allPages != 1 {
		t.Errorf("scaffold added more than once: dashboards=%d all_pages=%d", dashboards, allPages)
	}
}

// TestInjectDefaultSidebarItems_HiddenCategoryNotReAdded: a hidden category is
// present-but-invisible; the injector must not re-add it as a visible duplicate.
func TestInjectDefaultSidebarItems_HiddenCategoryNotReAdded(t *testing.T) {
	existing := []campaigns.SidebarItem{{Type: "category", TypeID: 10, Visible: false}}
	got := injectDefaultSidebarItems(existing, catTypes(10))

	count, hidden := 0, false
	for _, it := range got {
		if it.Type == "category" && it.TypeID == 10 {
			count++
			if !it.Visible {
				hidden = true
			}
		}
	}
	if count != 1 {
		t.Errorf("category 10 appears %d times, want 1 (no re-add)", count)
	}
	if !hidden {
		t.Errorf("category 10 must stay hidden")
	}
}

// TestInjectDefaultSidebarItems_Idempotent: running the injector on its own
// output is a fixed point.
func TestInjectDefaultSidebarItems_Idempotent(t *testing.T) {
	types := catTypes(10, 20)
	once := injectDefaultSidebarItems(nil, types)
	twice := injectDefaultSidebarItems(once, types)
	if !equalStr(itemTypes(once), itemTypes(twice)) {
		t.Errorf("not idempotent:\n once=%v\n twice=%v", itemTypes(once), itemTypes(twice))
	}
}

// --- Auto-add reaches converted campaigns ---

type fakeSidebarStore struct {
	cfg     *campaigns.SidebarConfig
	written *campaigns.UpdateSidebarConfigRequest
}

func (f *fakeSidebarStore) GetSidebarConfig(_ context.Context, _ string) (*campaigns.SidebarConfig, error) {
	return f.cfg, nil
}
func (f *fakeSidebarStore) UpdateSidebarConfig(_ context.Context, _ string, req campaigns.UpdateSidebarConfigRequest) error {
	f.written = &req
	return nil
}

// TestAddEntityTypeToSidebar_ReachesConvertedCampaign: a campaign on the items
// model (as the reconciler leaves it) gains a newly created type, appended
// before All Pages in its customized order.
func TestAddEntityTypeToSidebar_ReachesConvertedCampaign(t *testing.T) {
	store := &fakeSidebarStore{cfg: &campaigns.SidebarConfig{Items: []campaigns.SidebarItem{
		{Type: "category", TypeID: 5, Visible: true},
		{Type: "all_pages", Visible: true},
	}}}
	adder := &sidebarAutoAdderAdapter{campaignService: store}

	if err := adder.AddEntityTypeToSidebar(context.Background(), "c1", 9); err != nil {
		t.Fatalf("AddEntityTypeToSidebar: %v", err)
	}
	if store.written == nil || store.written.Items == nil {
		t.Fatalf("expected a write appending the new type")
	}
	items := *store.written.Items
	// New category present, inserted before all_pages.
	var order []string
	for _, it := range items {
		switch it.Type {
		case "category":
			order = append(order, "cat")
		case "all_pages":
			order = append(order, "all")
		}
	}
	if len(order) != 3 || order[2] != "all" {
		t.Errorf("expected [cat cat all], got %v", order)
	}
	found := false
	for _, it := range items {
		if it.Type == "category" && it.TypeID == 9 {
			found = true
		}
	}
	if !found {
		t.Errorf("new type 9 not added: %+v", items)
	}
}

// TestAddEntityTypeToSidebar_SkipsEmptyConfig: a never-customized campaign
// (empty Items) is left alone — the render injector shows the new type in its
// natural position, so persisting a lone item here (which would snap it to the
// front) is avoided.
func TestAddEntityTypeToSidebar_SkipsEmptyConfig(t *testing.T) {
	store := &fakeSidebarStore{cfg: &campaigns.SidebarConfig{}}
	adder := &sidebarAutoAdderAdapter{campaignService: store}

	if err := adder.AddEntityTypeToSidebar(context.Background(), "c1", 9); err != nil {
		t.Fatalf("AddEntityTypeToSidebar: %v", err)
	}
	if store.written != nil {
		t.Errorf("empty config must not be written to, got %+v", store.written)
	}
}

func equalStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
