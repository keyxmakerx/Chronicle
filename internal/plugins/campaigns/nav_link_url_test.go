// nav_link_url_test.go — ingress scheme allowlisting for owner-supplied
// topbar/sidebar link URLs (audit-R2 Finding 1, stored XSS / open redirect).
package campaigns

import (
	"context"
	"testing"
)

var dangerousURLs = []string{
	"javascript:alert(1)",
	" javascript:alert(1)", // leading whitespace
	"JAVASCRIPT:alert(1)",  // casing
	"data:text/html,<script>alert(1)</script>",
	"vbscript:msgbox(1)",
	"//evil.com", // protocol-relative open redirect
}

func TestUpdateSidebarConfig_RejectsDangerousURLs(t *testing.T) {
	newSvc := func(saved *string) *campaignService {
		return &campaignService{repo: &mockCampaignRepo{
			findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
				return &Campaign{ID: id}, nil
			},
			updateSidebarConfigFn: func(_ context.Context, _, cfg string) error { *saved = cfg; return nil },
		}}
	}
	// Sidebar link items (Items[type=link]) are guarded — the single
	// unified model, replacing the legacy CustomLinks field.
	for _, u := range dangerousURLs {
		var saved string
		items := []SidebarItem{{Type: "link", Label: "Evil", URL: u}}
		if err := newSvc(&saved).UpdateSidebarConfig(context.Background(), "camp-1",
			UpdateSidebarConfigRequest{Items: &items}); err == nil {
			t.Errorf("Items[link] should reject %q", u)
		} else if saved != "" {
			t.Errorf("rejected link %q must short-circuit before write", u)
		}
	}
	// Valid links persist.
	var saved string
	okItems := []SidebarItem{{Type: "link", Label: "OK", URL: "/campaigns/x", Visible: true}}
	if err := newSvc(&saved).UpdateSidebarConfig(context.Background(), "camp-1",
		UpdateSidebarConfigRequest{Items: &okItems}); err != nil {
		t.Errorf("valid sidebar link should be accepted: %v", err)
	}
	if saved == "" {
		t.Errorf("valid sidebar config should persist")
	}
}
