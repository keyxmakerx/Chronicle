package packages

import "testing"

// TestActionsFragmentURLFor pins the per-type hook lookup the page builder
// uses to fill each row's lazy-load slot: an unregistered type, or a hook
// with no URL func, must yield "" rather than panic.
func TestActionsFragmentURLFor(t *testing.T) {
	hooks := typeUIHooks{
		PackageTypeFoundryModule: {ActionsFragmentURL: func(p Package) string {
			return "/admin/foundry-vtt/packages/" + p.ID + "/actions-fragment"
		}},
		PackageTypeSystem: {},
	}
	cases := []struct {
		name  string
		hooks typeUIHooks
		pkg   Package
		want  string
	}{
		{"registered type returns its URL", hooks, Package{ID: "pkg-1", Type: PackageTypeFoundryModule}, "/admin/foundry-vtt/packages/pkg-1/actions-fragment"},
		{"hook without URL func returns empty", hooks, Package{ID: "sys-1", Type: PackageTypeSystem}, ""},
		{"unregistered type returns empty", hooks, Package{ID: "unk-1", Type: PackageType("unknown-type")}, ""},
		{"nil registry returns empty", nil, Package{ID: "p", Type: PackageTypeFoundryModule}, ""},
		{"empty ID still produces a URL (validation is the fragment handler's job)", hooks, Package{Type: PackageTypeFoundryModule}, "/admin/foundry-vtt/packages//actions-fragment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.hooks.actionsFragmentURLFor(tc.pkg); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
