package sessions

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestDumpNightPageForBrowser writes a date-less night's real rendered page to
// $NIGHT_PAGE_OUT so test/js/night_page_layout_browser.test.mjs can measure its
// layout in a browser. It does nothing unless that variable is set.
func TestDumpNightPageForBrowser(t *testing.T) {
	out := os.Getenv("NIGHT_PAGE_OUT")
	if out == "" {
		t.Skip("NIGHT_PAGE_OUT not set")
	}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp1", Name: "Audit"}, MemberRole: campaigns.RoleOwner}
	s := &Session{ID: "s1", CampaignID: "camp1", Name: "Session 7", Status: StatusPlanned, CreatorName: "Mara Owner"}
	note := NightDateNote{NoRealDate: true, Words: "No real date yet · 22 Hammer, 1523",
		EventName: "Session 7", EventHref: "/campaigns/camp1/calendars/c/view?event=e&date=1523-1-22"}
	var sb strings.Builder
	if err := SessionDetailPage(cc, s, "tok", true, true, "u1", RecapView{}, "UTC", note).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}
