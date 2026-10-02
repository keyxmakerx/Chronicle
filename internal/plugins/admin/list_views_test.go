package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestUserListView(t *testing.T) {
	counts := auth.UserFilterCounts{All: 40, Admins: 3, Disabled: 5}
	tests := []struct {
		name       string
		filter     auth.UserFilter
		wantTotal  int
		wantActive string
	}{
		{"all", auth.UserFilterAll, 40, "All"},
		{"admins", auth.UserFilterAdmins, 3, "Admins"},
		{"disabled", auth.UserFilterDisabled, 5, "Disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := userListView(listQuery{Q: "x"}, tt.filter, counts, 1)
			if v.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", v.Total, tt.wantTotal)
			}
			var active []string
			for _, c := range v.Chips {
				if c.Active {
					active = append(active, c.Label)
				}
			}
			if len(active) != 1 || active[0] != tt.wantActive {
				t.Errorf("active chips = %v, want [%s]", active, tt.wantActive)
			}
		})
	}
}

func TestResolveSystemFilter(t *testing.T) {
	counts := []campaigns.SystemCount{{Key: "drawsteel", Count: 2}, {Key: campaigns.SystemFilterNone, Count: 1}}
	tests := []struct{ raw, want string }{
		{"", ""},
		{"drawsteel", "drawsteel"},
		{"none", "none"},
		{"dnd5e", ""},
		{"x' OR 1=1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := resolveSystemFilter(tt.raw, counts); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCampaignListView(t *testing.T) {
	counts := []campaigns.SystemCount{
		{Key: "drawsteel", Count: 4}, {Key: campaigns.SystemFilterNone, Count: 2}, {Key: campaigns.SystemFilterCustom, Count: 1},
	}
	tests := []struct {
		name      string
		filter    string
		counts    []campaigns.SystemCount
		wantTotal int
		wantChips int
	}{
		{"all", "", counts, 7, 4},
		{"one system", "drawsteel", counts, 4, 4},
		{"none bucket", "none", counts, 2, 4},
		{"single system collapses to All only", "", counts[:1], 4, 1},
		{"no campaigns", "", nil, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := campaignListView(listQuery{}, tt.filter, tt.counts, 1)
			if v.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", v.Total, tt.wantTotal)
			}
			if len(v.Chips) != tt.wantChips {
				t.Errorf("chips = %d, want %d", len(v.Chips), tt.wantChips)
			}
		})
	}
}

// TestAdminUsersList_Render pins the toolbar contract: a GET search form that
// works without JS, counts on the chips, and the empty state wording.
func TestAdminUsersList_Render(t *testing.T) {
	counts := auth.UserFilterCounts{All: 0, Admins: 0, Disabled: 0}
	data := UserListData{
		View:      userListView(listQuery{Q: "zed<b>"}, auth.UserFilterAll, counts, 1),
		CSRFToken: "csrf",
	}
	var buf bytes.Buffer
	if err := AdminUsersList(data).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "No people match “zed&lt;b&gt;”.") {
		t.Errorf("empty state must quote the escaped search text; got: %s", html)
	}
	if !strings.Contains(html, "Clear search") {
		t.Errorf("empty state must offer Clear search; got: %s", html)
	}
	if !strings.Contains(html, `id="users-list"`) {
		t.Errorf("list region id missing; got: %s", html)
	}

	buf.Reset()
	if err := AdminUsersPage(data).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	page := buf.String()
	for _, want := range []string{`method="get"`, `name="q"`, `hx-push-url="true"`, `hx-target="#users-list"`, `maxlength="100"`} {
		if !strings.Contains(page, want) {
			t.Errorf("search form missing %s", want)
		}
	}
}
