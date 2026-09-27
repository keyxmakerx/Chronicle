package campaigns

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateSidebarItems(t *testing.T) {
	long := strings.Repeat("x", maxNavLabelLen+1)
	tests := []struct {
		name    string
		items   []SidebarItem
		wantErr bool
	}{
		{name: "an arranged sidebar with the owner's own section", items: []SidebarItem{
			{Type: "app", Slug: "notes", Section: "pinned", Visible: true},
			{Type: "category", TypeID: 4, Section: "categories", Visible: false},
			{Type: "section", ID: "sec_a", Label: "At the table", Visible: true},
			{Type: "link", ID: "lnk_1", Label: "Wiki", URL: "https://example.test", Section: "sec_a", Visible: true},
			{Type: "addon", Slug: "maps", Section: "sec_a", Visible: true},
		}},
		{name: "older configs with dashboard and all pages rows", items: []SidebarItem{
			{Type: "dashboard", Visible: true}, {Type: "all_pages", Visible: true},
		}},
		{name: "an unknown item type", items: []SidebarItem{{Type: "script"}}, wantErr: true},
		{name: "a section claiming a built-in id", items: []SidebarItem{{Type: "section", ID: "pinned"}}, wantErr: true},
		{name: "two sections with one id", items: []SidebarItem{{Type: "section", ID: "a"}, {Type: "section", ID: "a"}}, wantErr: true},
		{name: "a section id with spaces", items: []SidebarItem{{Type: "section", ID: "a b"}}, wantErr: true},
		{name: "an item in a section that does not exist", items: []SidebarItem{{Type: "app", Slug: "notes", Section: "ghost"}}, wantErr: true},
		{name: "a label that is too long", items: []SidebarItem{{Type: "section", ID: "a", Label: long}}, wantErr: true},
		{name: "an app slug that is not a slug", items: []SidebarItem{{Type: "app", Slug: "<b>x</b>"}}, wantErr: true},
		{name: "a category without an id", items: []SidebarItem{{Type: "category"}}, wantErr: true},
		{name: "a link id with markup", items: []SidebarItem{{Type: "link", ID: `x"><`, Label: "L", URL: "/x"}}, wantErr: true},
		{name: "a link to a script URL", items: []SidebarItem{{Type: "link", ID: "l", Label: "L", URL: "javascript:alert(1)"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateSidebarItems(tt.items)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSidebarItems_Tidies(t *testing.T) {
	got, err := validateSidebarItems([]SidebarItem{
		{Type: "section", ID: " sec_a ", Label: "  Lore  "},
		{Type: "link", ID: "lnk", Label: " Wiki ", URL: "/wiki", Icon: "w-screen h-screen fixed", Section: " sec_a "},
		{Type: "link", ID: "lnk2", Label: "Map", URL: "/map", Icon: "fa-map"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ID != "sec_a" || got[0].Label != "Lore" {
		t.Errorf("section not trimmed: %+v", got[0])
	}
	if got[1].Label != "Wiki" || got[1].Section != "sec_a" {
		t.Errorf("link not trimmed: %+v", got[1])
	}
	if got[1].Icon != "" {
		t.Errorf("an icon that is not a Font Awesome name must be dropped, got %q", got[1].Icon)
	}
	if got[2].Icon != "fa-map" {
		t.Errorf("icon = %q, want fa-map", got[2].Icon)
	}
}

// TestUpdateSidebarConfig_StoresSections pins that the section each item sits
// in survives the write, and that a bad item stops the write entirely.
func TestUpdateSidebarConfig_StoresSections(t *testing.T) {
	var saved string
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
			return &Campaign{ID: id}, nil
		},
		updateSidebarConfigFn: func(_ context.Context, _, cfg string) error { saved = cfg; return nil },
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})

	items := []SidebarItem{
		{Type: "app", Slug: "notes", Section: "pinned", Visible: true},
		{Type: "section", ID: "sec_a", Label: "At the table", Visible: true},
		{Type: "category", TypeID: 7, Section: "sec_a", Visible: false},
	}
	if err := svc.UpdateSidebarConfig(context.Background(), "camp-1", UpdateSidebarConfigRequest{Items: &items}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got SidebarConfig
	if err := json.Unmarshal([]byte(saved), &got); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}
	if len(got.Items) != 3 || got.Items[0].Section != "pinned" || got.Items[2].Section != "sec_a" || got.Items[2].Visible {
		t.Errorf("saved items = %+v", got.Items)
	}

	saved = ""
	bad := []SidebarItem{{Type: "app", Slug: "notes", Section: "nowhere", Visible: true}}
	err := svc.UpdateSidebarConfig(context.Background(), "camp-1", UpdateSidebarConfigRequest{Items: &bad})
	assertAppError(t, err, 400)
	if saved != "" {
		t.Errorf("a refused config must not be written")
	}
}
