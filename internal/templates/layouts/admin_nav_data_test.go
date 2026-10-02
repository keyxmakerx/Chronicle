package layouts

import "testing"

func TestAdminNavCurrent(t *testing.T) {
	tests := []struct {
		path        string
		wantSection string
		wantHref    string
	}{
		{"/admin", "", "/admin"},
		{"/admin/users", "community", "/admin/users"},
		{"/admin/users/abc", "community", "/admin/users"},
		{"/admin/campaigns", "community", "/admin/campaigns"},
		{"/admin/packages", "packages", "/admin/packages"},
		{"/admin/packages/pending", "packages", "/admin/packages"},
		{"/admin/packages/settings", "packages", "/admin/packages/settings"},
		{"/admin/addons", "apps", "/admin/addons"},
		{"/admin/extensions", "apps", "/admin/extensions"},
		{"/admin/extensions/wasm/plugins", "apps", "/admin/extensions"},
		{"/admin/security", "security", "/admin/security"},
		{"/admin/api", "security", "/admin/api"},
		{"/admin/api/cors", "security", "/admin/api"},
		{"/admin/api/security", "security", "/admin/api"},
		{"/admin/activity", "security", "/admin/activity"},
		{"/admin/storage", "site", "/admin/storage"},
		{"/admin/storage/settings", "site", "/admin/storage"},
		{"/admin/data-hygiene", "site", "/admin/data-hygiene"},
		{"/admin/backup", "site", "/admin/backup"},
		{"/admin/restore", "site", "/admin/backup"},
		{"/admin/smtp", "site", "/admin/smtp"},
		{"/admin/systems", "tools", "/admin/systems"},
		{"/admin/database", "tools", "/admin/database"},
		{"/admin/database/schema", "tools", "/admin/database"},
		{"/admin/diagnostics/workspace", "tools", "/admin/diagnostics/workspace"},
		{"/admin/design-lab", "tools", "/admin/design-lab"},
		// A shared leading string is not a path prefix.
		{"/admin/apiary", "", ""},
		{"/admin/packages-old", "", ""},
		{"/campaigns/x", "", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			sec, href := adminNav.Current(tc.path)
			if sec != tc.wantSection || href != tc.wantHref {
				t.Errorf("Current(%q) = (%q, %q), want (%q, %q)", tc.path, sec, href, tc.wantSection, tc.wantHref)
			}
		})
	}
}

// Exactly one item may be current for any item's own link, or two rows would
// light up at once.
func TestAdminNavSingleActive(t *testing.T) {
	var all []AdminNavItem
	all = append(all, adminNav.Home)
	for _, s := range adminNav.Sections {
		all = append(all, s.Items...)
	}
	for _, probe := range all {
		n := 0
		for _, it := range all {
			if it.Active(probe.Href) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%d items active for %q, want 1", n, probe.Href)
		}
	}
}

func TestAdminNavShape(t *testing.T) {
	ids := map[string]bool{}
	for _, s := range adminNav.Sections {
		if s.ID == "" || s.Label == "" {
			t.Errorf("section %+v needs an ID and label", s)
		}
		if ids[s.ID] {
			t.Errorf("duplicate section ID %q", s.ID)
		}
		ids[s.ID] = true
		if len(s.Items) == 0 {
			t.Errorf("section %q has no items", s.ID)
		}
		for _, it := range s.Items {
			if it.Label == "" || it.Href == "" || it.Icon == "" {
				t.Errorf("section %q item %+v needs label, href and icon", s.ID, it)
			}
		}
	}
}
