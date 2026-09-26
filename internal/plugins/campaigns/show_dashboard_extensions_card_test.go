// show_dashboard_extensions_card_test.go pins #630: the owner
// dashboard's fourth quick-link card must open the live Extensions
// hub, not the retired Settings "Features" tab (which renders blank).

package campaigns

import (
	"context"
	"strings"
	"testing"
)

func TestOwnerDashboard_ExtensionsCard(t *testing.T) {
	cc := &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Name: "Test", IsPublic: true},
		MemberRole: RoleOwner,
	}
	var sb strings.Builder
	if err := OwnerDashboardPage(cc, nil, "tok").Render(context.Background(), &sb); err != nil {
		t.Fatalf("render owner dashboard: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, "settings?tab=features") {
		t.Errorf("dashboard must not link to the retired Features settings tab; got:\n%s", html)
	}
	if !strings.Contains(html, `href="/campaigns/camp-1/extensions"`) {
		t.Errorf("dashboard's fourth card must link to the Extensions hub; got:\n%s", html)
	}
	if !strings.Contains(html, ">Extensions</h3>") {
		t.Errorf("dashboard card must be labeled Extensions; got:\n%s", html)
	}
	if strings.Contains(html, ">Features</h3>") {
		t.Errorf("dashboard must not keep the stale Features label; got:\n%s", html)
	}
}
