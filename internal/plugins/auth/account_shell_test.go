package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

// The rail jumps within the account page when on it and leads back to it from
// a campaign's page; it marks the campaign whose page is open, says when it
// has none, and counts what it leaves out.
func TestAccountShell(t *testing.T) {
	list := CampaignSettingsList{
		Campaigns: []CampaignSettingsRef{{ID: "c1", Name: "Ashen"}, {ID: "c2", Name: "Brine", Archived: true}},
		More:      3,
	}
	render := func(l CampaignSettingsList, active string) string {
		var buf bytes.Buffer
		ctx := templ.WithChildren(context.Background(), templ.Raw("<p>body</p>"))
		if err := AccountShell(l, active).Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	cases := []struct {
		name   string
		list   CampaignSettingsList
		active string
		want   []string
		not    []string
	}{
		{"account page", list, "", []string{`href="#profile"`, `href="/account/campaigns/c1"`, "Archived", "3 more campaigns", "<p>body</p>"}, []string{`aria-current`}},
		{"a campaign's page", list, "c2", []string{`href="/account#profile"`, `aria-current="page"`}, nil},
		{"no campaigns", CampaignSettingsList{}, "", []string{"Campaigns you join show here"}, []string{"/account/campaigns/"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := render(tc.list, tc.active)
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in\n%s", w, out)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(out, n) {
					t.Errorf("should not have %q", n)
				}
			}
		})
	}
	if got := moreCampaignsText(1); got != "1 more campaign" {
		t.Errorf("moreCampaignsText(1) = %q", got)
	}
}
