// sidebar_topbar_icon_test.go pins the icon name check on the sidebar
// config and topbar content partial-update paths: an item/link carrying an
// invalid icon is refused before the repository is written, and a valid one
// is accepted.
package campaigns

import (
	"context"
	"testing"
)

// badSidebarTopbarIconNames are inputs that must fail sanitize.ValidateIcon.
var badSidebarTopbarIconNames = []string{
	`fa-x" onmouseover="y`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func TestUpdateSidebarConfig_InvalidItemIcon(t *testing.T) {
	for _, icon := range badSidebarTopbarIconNames {
		t.Run(icon, func(t *testing.T) {
			var saved string
			svc := &campaignService{repo: &mockCampaignRepo{
				findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
					return &Campaign{ID: id}, nil
				},
				updateSidebarConfigFn: func(_ context.Context, _, cfg string) error {
					saved = cfg
					return nil
				},
			}}
			items := []SidebarItem{{Type: "link", Label: "Link", URL: "/x", Icon: icon}}
			err := svc.UpdateSidebarConfig(context.Background(), "camp-1", UpdateSidebarConfigRequest{Items: &items})
			if err == nil {
				t.Fatalf("expected an error for icon %q", icon)
			}
			if saved != "" {
				t.Errorf("invalid icon %q must short-circuit before repo write", icon)
			}
		})
	}
}

func TestUpdateSidebarConfig_ValidItemIcon(t *testing.T) {
	var saved string
	svc := &campaignService{repo: &mockCampaignRepo{
		findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
			return &Campaign{ID: id}, nil
		},
		updateSidebarConfigFn: func(_ context.Context, _, cfg string) error {
			saved = cfg
			return nil
		},
	}}
	items := []SidebarItem{{Type: "link", Label: "Link", URL: "/x", Icon: "fa-dragon", Visible: true}}
	if err := svc.UpdateSidebarConfig(context.Background(), "camp-1", UpdateSidebarConfigRequest{Items: &items}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == "" {
		t.Error("expected the sidebar config to be persisted")
	}
}

func TestUpdateTopbarContent_InvalidLinkIcon(t *testing.T) {
	for _, icon := range badSidebarTopbarIconNames {
		t.Run(icon, func(t *testing.T) {
			var saved string
			svc := &campaignService{repo: tierTestRepo("{}", &saved)}
			err := svc.UpdateTopbarContent(context.Background(), "camp-1",
				&TopbarContent{Mode: "links", Links: []TopbarLink{{Label: "Link", URL: "/x", Icon: icon}}})
			if err == nil {
				t.Fatalf("expected an error for icon %q", icon)
			}
			if saved != "" {
				t.Errorf("invalid icon %q must short-circuit before repo write", icon)
			}
		})
	}
}

func TestUpdateTopbarContent_ValidLinkIcon(t *testing.T) {
	var saved string
	svc := &campaignService{repo: tierTestRepo("{}", &saved)}
	err := svc.UpdateTopbarContent(context.Background(), "camp-1",
		&TopbarContent{Mode: "links", Links: []TopbarLink{{Label: "Link", URL: "/x", Icon: "fa-dragon"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved == "" {
		t.Error("expected the topbar content to be persisted")
	}
}
