// sidebar_topbar_icon_test.go pins the icon name check on the sidebar config
// and topbar content paths: the editor re-sends every stored item, so an
// invalid icon is dropped (the save still succeeds) and a valid one is kept.
package campaigns

import (
	"context"
	"encoding/json"
	"testing"
)

// badSidebarTopbarIconNames are inputs that must fail the shared icon check.
var badSidebarTopbarIconNames = []string{
	`fa-x" data-y="z`,
	`<b>`,
	`FA-BOOK`,
	`fa-`,
}

func sidebarService(saved *string) *campaignService {
	return &campaignService{repo: &mockCampaignRepo{
		findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
			return &Campaign{ID: id}, nil
		},
		updateSidebarConfigFn: func(_ context.Context, _, cfg string) error {
			*saved = cfg
			return nil
		},
	}}
}

func TestUpdateSidebarConfig_ItemIcon(t *testing.T) {
	cases := map[string]string{"fa-dragon": "fa-dragon", " fa-dragon ": "fa-dragon", "": ""}
	for _, bad := range badSidebarTopbarIconNames {
		cases[bad] = ""
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			var saved string
			items := []SidebarItem{{Type: "link", Label: "Link", URL: "/x", Icon: in, Visible: true}}
			if err := sidebarService(&saved).UpdateSidebarConfig(context.Background(), "camp-1",
				UpdateSidebarConfigRequest{Items: &items}); err != nil {
				t.Fatalf("save must succeed, got %v", err)
			}
			var cfg SidebarConfig
			if err := json.Unmarshal([]byte(saved), &cfg); err != nil || len(cfg.Items) != 1 {
				t.Fatalf("saved config unreadable: %q (%v)", saved, err)
			}
			if got := cfg.Items[0].Icon; got != want {
				t.Errorf("stored icon = %q, want %q", got, want)
			}
		})
	}
}

func TestUpdateTopbarContent_LinkIcon(t *testing.T) {
	cases := map[string]string{"fa-dragon": "fa-dragon", "": ""}
	for _, bad := range badSidebarTopbarIconNames {
		cases[bad] = ""
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			var saved string
			svc := &campaignService{repo: tierTestRepo("{}", &saved)}
			if err := svc.UpdateTopbarContent(context.Background(), "camp-1",
				&TopbarContent{Mode: "links", Links: []TopbarLink{{Label: "Link", URL: "/x", Icon: in}}}); err != nil {
				t.Fatalf("save must succeed, got %v", err)
			}
			var settings CampaignSettings
			if err := json.Unmarshal([]byte(saved), &settings); err != nil || settings.TopbarContent == nil || len(settings.TopbarContent.Links) != 1 {
				t.Fatalf("saved settings unreadable: %q (%v)", saved, err)
			}
			if got := settings.TopbarContent.Links[0].Icon; got != want {
				t.Errorf("stored icon = %q, want %q", got, want)
			}
		})
	}
}
