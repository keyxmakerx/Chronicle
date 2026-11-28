package campaigns

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// listStubSvc answers List with a fixed page and total.
type listStubSvc struct {
	CampaignService
	page  []Campaign
	total int
}

func (s listStubSvc) List(context.Context, string, ListOptions) ([]Campaign, int, error) {
	return s.page, s.total, nil
}

// The rail lists every campaign on the page and counts the rest instead of
// silently stopping.
func TestSettingsCampaigns(t *testing.T) {
	cases := []struct {
		name     string
		page     []Campaign
		total    int
		wantLen  int
		wantMore int
	}{
		{"none", nil, 0, 0, 0},
		{"all fit", []Campaign{{ID: "a", Name: "Ashen"}, {ID: "b", Name: "Brine"}}, 2, 2, 0},
		{"more than a page", []Campaign{{ID: "a", Name: "Ashen"}}, 130, 1, 129},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AccountCampaignLister{Service: listStubSvc{page: tc.page, total: tc.total}}.SettingsCampaigns(context.Background(), "u1")
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Campaigns) != tc.wantLen || got.More != tc.wantMore {
				t.Errorf("got %d listed, %d more; want %d, %d", len(got.Campaigns), got.More, tc.wantLen, tc.wantMore)
			}
		})
	}
}

// What each viewer gets on a campaign's settings page: a player with parts
// open picks them in place; a player with none is told who decides; the
// owner is sent to Customize and gets the brush and the Store icon choice;
// only a member can leave.
func TestCampaignSettingsPage(t *testing.T) {
	cp := &Campaign{ID: "c1", Name: "Ashen Reach"}
	rail := auth.CampaignSettingsList{Campaigns: []auth.CampaignSettingsRef{{ID: "c1", Name: "Ashen Reach"}}}
	sheet, _ := LookPartByKey(LookPartSheet)
	render := func(v campaignSettingsView) string {
		var buf bytes.Buffer
		if err := CampaignSettingsPage(v).Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	cases := []struct {
		name string
		v    campaignSettingsView
		want []string
		not  []string
	}{
		{
			name: "player with a part open",
			v:    campaignSettingsView{Rail: rail, Campaign: cp, Parts: []yourLookPart{{Part: sheet, URL: "/campaigns/c1/my-look/sheet"}}},
			want: []string{`data-look-part="sheet"`, `data-url="/campaigns/c1/my-look/sheet"`, `aria-current="page"`, `data-widget="account-campaign"`, "Leave Ashen Reach", `data-leave-then="/account"`},
			not:  []string{"data-acct-brush", "data-acct-store", "/customize/look"},
		},
		{
			name: "player with nothing open",
			v:    campaignSettingsView{Rail: rail, Campaign: cp},
			want: []string{"sets the look for everyone here", "Leave Ashen Reach"},
			not:  []string{"data-look-part", "data-acct-brush", "data-acct-store"},
		},
		{
			name: "owner",
			v:    campaignSettingsView{Rail: rail, Campaign: cp, IsOwner: true},
			want: []string{"/campaigns/c1/customize/look", `data-acct-brush="c1"`, `data-acct-store="c1"`, `data-store="shown" aria-pressed="true"`, `data-store="hidden" aria-pressed="false"`},
			not:  []string{"data-look-part", "Leave Ashen Reach", "data-leave-url"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := render(tc.v)
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, n := range tc.not {
				if strings.Contains(out, n) {
					t.Errorf("should not have %q", n)
				}
			}
		})
	}
}
