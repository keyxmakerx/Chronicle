// sidebar_inject_test.go covers the auto-add path: a new category joins a
// customized sidebar, while a never-customized one is left to NormalizeNav,
// which already lists every category (see nav_layout_test.go).
package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

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
