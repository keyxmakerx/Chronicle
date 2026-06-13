package campaigns

// show_banner_gate_test.go — cordinator#30 r2: the Foundry update-banner
// fragment loader is OWNER-ONLY MARKUP. It used to render for every viewer
// while its endpoint sits behind RequireAuth+requireOwner — so an anonymous
// visitor on a PUBLIC campaign fired an on-load hx-get, took a 401, and the
// error handler bounced the whole page to /login. Non-owners must never
// receive the fragment call at all.

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
	if err := CampaignShowPage(cc, nil, nil, "tok").Render(context.Background(), &sb); err != nil {
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
