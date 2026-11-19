package campaigns

// show_banner_gate_test.go pins that the Foundry update-banner fragment
// loader is owner-only markup: its endpoint sits behind
// RequireAuth+requireOwner, so a non-owner must never receive the fragment
// call (an anonymous visitor on a public campaign would 401 on-load and get
// bounced to /login).

import (
	"context"
	"strings"
	"testing"
)

func renderShowFor(t *testing.T, role Role) string {
	t.Helper()
	cc := &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Name: "Test", IsPublic: true},
		MemberRole: role,
	}
	var sb strings.Builder
	if err := CampaignShowPage(cc, nil, nil, "tok", false).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render show page: %v", err)
	}
	return sb.String()
}

func TestShowBanner_OwnerOnly(t *testing.T) {
	owner := renderShowFor(t, RoleOwner)
	if !strings.Contains(owner, "foundry-vtt/show-banner-fragment") {
		t.Errorf("owner page must lazy-load the VTT banner fragment")
	}
	for _, tc := range []struct {
		name string
		role Role
	}{
		{"anonymous/public-visitor (RoleNone ctx)", RoleNone},
		{"player", RolePlayer},
		{"scribe", RoleScribe},
	} {
		html := renderShowFor(t, tc.role)
		if strings.Contains(html, "show-banner-fragment") {
			t.Errorf("%s must NOT receive the owner-only banner hx-get (it 401/403s and hijacks/toasts)", tc.name)
		}
	}
}
