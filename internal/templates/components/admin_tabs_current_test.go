package components

import (
	"context"
	"strings"
	"testing"
)

func TestCurrentTabIndex(t *testing.T) {
	tests := []struct {
		name    string
		current string
		want    string // label of the one tab expected to be current, "" for none
	}{
		{"exact query match", "/admin/database?tab=health", "Database checks"},
		{"each database tab is its own entry", "/admin/database?tab=schema", "Tables"},
		{"bare path picks the first tab on it", "/admin/database", "Database updates"},
		{"other page", "/admin/systems", "Parts of Chronicle"},
		{"unknown query is not guessed", "/admin/database?tab=nope", ""},
		{"unknown path", "/admin/elsewhere", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i := currentTabIndex(AdminTabsHealth, tc.current)
			if tc.want == "" {
				if i != -1 {
					t.Fatalf("got tab %d, want none", i)
				}
				return
			}
			if i < 0 || AdminTabsHealth.Tabs[i].Label != tc.want {
				t.Fatalf("got index %d, want %q", i, tc.want)
			}
		})
	}
}

func TestAdminGroupHeadingDatabaseTabs(t *testing.T) {
	var sb strings.Builder
	if err := AdminGroupHeading(AdminTabsHealth, "/admin/database?tab=health").Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	out := sb.String()
	if n := strings.Count(out, `aria-current="page"`); n != 1 {
		t.Fatalf("aria-current count = %d, want 1", n)
	}
	for _, label := range []string{"Database updates", "Database checks", "Tables"} {
		if !strings.Contains(out, label) {
			t.Errorf("strip missing %q", label)
		}
	}
}
