package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// A guest is fenced to one campaign: the sidebar footer must not offer the
// ways out (Campaigns bounces back, Discover shows the signed-out front
// page), and Log out must warn before it ends their only way in.
func TestGuestNavigation(t *testing.T) {
	tests := []struct {
		name      string
		guest     bool
		wantOut   bool // Campaigns and Discover in the footer
		wantGuard bool // Log out asks first
	}{
		{"member", false, true, false},
		{"guest", true, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := SetIsGuest(SetCampaignID(context.Background(), "camp-1"), tc.guest)

			var foot bytes.Buffer
			if err := campaignNavFooter().Render(ctx, &foot); err != nil {
				t.Fatal(err)
			}
			gotOut := strings.Contains(foot.String(), `href="/campaigns"`) && strings.Contains(foot.String(), ">Discover<")
			if gotOut != tc.wantOut {
				t.Errorf("footer ways out shown = %v, want %v:\n%s", gotOut, tc.wantOut, foot.String())
			}

			var side bytes.Buffer
			if err := DefaultSidebarNav().Render(ctx, &side); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(side.String(), "Discover"); got != tc.wantOut {
				t.Errorf("outside sidebar shows Discover = %v, want %v", got, tc.wantOut)
			}

			if tc.wantGuard {
				var out bytes.Buffer
				if err := guestLogOut().Render(ctx, &out); err != nil {
					t.Fatal(err)
				}
				html := out.String()
				if !strings.Contains(html, "data-guest-logout-confirm") || !strings.Contains(html, "Keep my account") {
					t.Errorf("guest Log out has no warning with Keep my account:\n%s", html)
				}
				// The first Log out control must not submit; only the
				// confirm's own button does.
				first := strings.Index(html, "Log out")
				form := strings.Index(html, `action="/logout"`)
				if form < first {
					t.Errorf("the guest's first Log out is inside the logout form")
				}
			}
		})
	}
}
