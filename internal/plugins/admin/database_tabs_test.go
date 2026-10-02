package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/components"
)

func TestNormalizeDatabaseTab(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", DatabaseTabMigrations},
		{"migrations", DatabaseTabMigrations},
		{"health", DatabaseTabHealth},
		{"schema", DatabaseTabSchema},
		{"backups", DatabaseTabMigrations},
		{"'><script>", DatabaseTabMigrations},
	}
	for _, tc := range tests {
		if got := normalizeDatabaseTab(tc.in); got != tc.want {
			t.Errorf("normalizeDatabaseTab(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Each tab href the page reports must be one the shared strip declares, or no
// entry would be marked current.
func TestDatabaseTabHrefsMatchStrip(t *testing.T) {
	for _, tab := range []string{DatabaseTabMigrations, DatabaseTabHealth, DatabaseTabSchema} {
		found := false
		for _, st := range components.AdminTabsHealth.Tabs {
			if st.Href == databaseTabHref(tab) {
				found = true
			}
		}
		if !found {
			t.Errorf("tab %q href %q is not in AdminTabsHealth", tab, databaseTabHref(tab))
		}
	}
}

func TestDatabaseTabPanelRendersOnlyOpenTab(t *testing.T) {
	tests := []struct {
		tab     string
		want    string
		notWant string
	}{
		{DatabaseTabMigrations, "Core schema", "Schema diagram"},
		{DatabaseTabHealth, "Health checks are not available", "Schema diagram"},
		{DatabaseTabSchema, "Schema diagram", "Core schema"},
		{"bogus", "Core schema", "Schema diagram"},
	}
	for _, tc := range tests {
		t.Run(tc.tab, func(t *testing.T) {
			var sb strings.Builder
			if err := dbTabPanel(tc.tab, CoreMigrationStatus{}, nil, nil, "tok").Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if !strings.Contains(out, tc.want) {
				t.Errorf("missing %q", tc.want)
			}
			if strings.Contains(out, tc.notWant) {
				t.Errorf("unexpected %q", tc.notWant)
			}
		})
	}
}
