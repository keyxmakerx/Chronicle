package sessions

// A recap has no visibility setting of its own, so the session page shows it
// to members only; a public campaign's visitors see the session, not the
// write-up.

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestSessionDetailPage_RecapIsMembersOnly(t *testing.T) {
	recap := "<p>The vault door was a mimic</p>"
	tests := []struct {
		name     string
		isMember bool
		want     bool
	}{
		{"member sees the recap", true, true},
		{"public visitor does not", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{
				Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Camp", IsPublic: true},
				IsMember: tc.isMember,
			}
			if tc.isMember {
				cc.MemberRole = campaigns.RolePlayer
			}
			s := &Session{ID: "s1", CampaignID: "camp-1", Name: "Night one", Status: StatusCompleted, RecapHTML: &recap}
			var sb strings.Builder
			if err := SessionDetailPage(cc, s, "csrf", false, false, "u1", RecapView{Open: true}, "").Render(context.Background(), &sb); err != nil {
				t.Fatalf("render: %v", err)
			}
			got := strings.Contains(sb.String(), "The vault door was a mimic")
			if got != tc.want {
				t.Errorf("recap shown = %v, want %v", got, tc.want)
			}
		})
	}
}
