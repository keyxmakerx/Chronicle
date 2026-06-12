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

func TestUpdateTopbarContent_RejectsDangerousURLs(t *testing.T) {
	for _, u := range dangerousURLs {
		var saved string
		svc := &campaignService{repo: tierTestRepo("{}", &saved)}
		err := svc.UpdateTopbarContent(context.Background(), "camp-1",
			&TopbarContent{Mode: "links", Links: []TopbarLink{{Label: "Evil", URL: u}}})
		if err == nil {
			t.Errorf("UpdateTopbarContent should reject %q", u)
		}
		if saved != "" {
			t.Errorf("rejected URL %q must short-circuit before repo write", u)
		}
	}
	// Valid URLs are accepted + persisted.
	for _, u := range []string{"https://example.com/x", "http://example.com", "/campaigns/abc"} {
		var saved string
		svc := &campaignService{repo: tierTestRepo("{}", &saved)}
		if err := svc.UpdateTopbarContent(context.Background(), "camp-1",
			&TopbarContent{Mode: "links", Links: []TopbarLink{{Label: "OK", URL: u}}}); err != nil {
			t.Errorf("UpdateTopbarContent should accept %q: %v", u, err)
		}
		if saved == "" {
			t.Errorf("accepted URL %q should have been persisted", u)
		}
	}
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
	// Both the legacy CustomLinks and the newer Items[type=link] are guarded.
	// (Request form per the #473 merge semantics: pointer fields.)
	for _, u := range dangerousURLs {
		var saved string
		links := []NavLink{{Label: "Evil", URL: u}}
		if err := newSvc(&saved).UpdateSidebarConfig(context.Background(), "camp-1",
			UpdateSidebarConfigRequest{CustomLinks: &links}); err == nil {
			t.Errorf("CustomLinks should reject %q", u)
		} else if saved != "" {
			t.Errorf("rejected CustomLink %q must short-circuit before write", u)
		}

		saved = ""
		items := []SidebarItem{{Type: "link", Label: "Evil", URL: u}}
		if err := newSvc(&saved).UpdateSidebarConfig(context.Background(), "camp-1",
			UpdateSidebarConfigRequest{Items: &items}); err == nil {
			t.Errorf("Items[link] should reject %q", u)
		}
	}
	// Valid links persist.
	var saved string
	okLinks := []NavLink{{Label: "OK", URL: "/campaigns/x"}}
	if err := newSvc(&saved).UpdateSidebarConfig(context.Background(), "camp-1",
		UpdateSidebarConfigRequest{CustomLinks: &okLinks}); err != nil {
		t.Errorf("valid sidebar link should be accepted: %v", err)
	}
	if saved == "" {
		t.Errorf("valid sidebar config should persist")
	}
}
