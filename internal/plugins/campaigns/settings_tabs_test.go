// settings_tabs_test.go pins the registry other plugins add their parts of
// the Data tab through (role filter, order, ties) and where each old Details
// and data tab address leads now.
package campaigns

import (
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func ctxWithRole(role Role) *CampaignContext {
	return &CampaignContext{
		Campaign:   &Campaign{ID: "camp-1", Name: "Test"},
		MemberRole: role,
	}
}

// tabIDs extracts the IDs from a tab slice for cleaner failure messages.
func tabIDs(tabs []SettingsTab) []string {
	out := make([]string, len(tabs))
	for i, t := range tabs {
		out[i] = t.ID
	}
	return out
}

// The registry holds only other plugins' parts: none registered is none
// shown, whatever the role, since the Campaign, Connections and Data tabs draw
// their own parts.
func TestSettingsTabs_NoneRegisteredIsNone(t *testing.T) {
	h := &Handler{}
	for _, role := range []Role{RolePlayer, RoleScribe, RoleOwner} {
		if got := h.visibleSettingsTabs(ctxWithRole(role)); len(got) != 0 {
			t.Errorf("role %d: %v", role, tabIDs(got))
		}
	}
}

// Parts land in SortOrder, keep their registration order on a tie, are
// dropped below their role, and a part with no id is never drawn (it would
// have no anchor to land on).
func TestRegisterSettingsTab_OrderRoleAndTies(t *testing.T) {
	h := &Handler{}
	part := func(id string, role Role, order int) func(*CampaignContext) SettingsTab {
		return func(*CampaignContext) SettingsTab {
			return SettingsTab{ID: id, Label: id, MinRole: role, SortOrder: order}
		}
	}
	h.RegisterSettingsTab(part("late", RolePlayer, 90))
	h.RegisterSettingsTab(part("import", RoleOwner, 55))
	h.RegisterSettingsTab(part("a", RolePlayer, 70))
	h.RegisterSettingsTab(part("b", RolePlayer, 70))
	h.RegisterSettingsTab(part("", RolePlayer, 10))
	tests := []struct {
		name string
		role Role
		want string
	}{
		{"owner", RoleOwner, "import,a,b,late"},
		{"player", RolePlayer, "a,b,late"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tabIDs(h.visibleSettingsTabs(ctxWithRole(tt.role)))
			if s := strings.Join(got, ","); s != tt.want {
				t.Errorf("parts = %s, want %s", s, tt.want)
			}
		})
	}
	ids := h.extraSettingsTabIDs(ctxWithRole(RoleOwner))
	if !ids["import"] || !ids["late"] || ids[""] || len(ids) != 4 {
		t.Errorf("extra ids = %v", ids)
	}
}

// Every old Details and data tab address leads to where that part lives now:
// Keys on Connections, the export, the activity log and each plugin's part on
// Data under its own anchor, People, Foundry and the AI tools on their pages.
// The Campaign tab itself and an unknown tab stay put.
func TestSettingsTabRedirects(t *testing.T) {
	extra := map[string]bool{"import": true}
	for tab, want := range map[string]string{
		"":             "",
		"general":      "",
		"people":       "/campaigns/c1/members",
		"integrations": "/campaigns/c1/foundry",
		"ai-workspace": "/campaigns/c1/ai",
		"api-keys":     "/campaigns/c1/connections#keys",
		"data":         "/campaigns/c1/data#export",
		"activity":     "/campaigns/c1/data#activity",
		"import":       "/campaigns/c1/data#import",
		"features":     "",
		"nope":         "",
	} {
		if got := settingsTabRedirect("c1", tab, extra); got != want {
			t.Errorf("settingsTabRedirect(%q) = %q, want %q", tab, got, want)
		}
	}
	if got := settingsTabRedirect("c1", "import", nil); got != "" {
		t.Errorf("a part this viewer cannot see is not redirected to: %q", got)
	}
}

// The registry's role gates depend on the role hierarchy staying 1=Player,
// 2=Scribe, 3=Owner, the same as the permissions package.
func TestSettingsTabs_RoleConstantsMatchPermissionsPackage(t *testing.T) {
	cases := []struct {
		got, want int
		label     string
	}{
		{int(RolePlayer), permissions.RolePlayer, "Player"},
		{int(RoleScribe), permissions.RoleScribe, "Scribe"},
		{int(RoleOwner), permissions.RoleOwner, "Owner"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: campaigns.Role=%d, permissions.Role=%d — must match",
				c.label, c.got, c.want)
		}
	}
}
