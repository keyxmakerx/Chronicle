package packages

import "testing"

// TestActionsFragmentURLFor pins the per-type URL dispatch packages.templ
// uses to render the lazy-load slot. Future plugin types added to the
// PackageType enum need an explicit case here; missing the case is the
// failure mode this test guards against.
//
// Per cordinator/decisions/2026-05-23-packages-treatment.md (NW-2.2 Chunk G).
func TestActionsFragmentURLFor(t *testing.T) {
	cases := []struct {
		name string
		pkg  Package
		want string
	}{
		{
			name: "foundry-module returns foundry_vtt fragment URL",
			pkg:  Package{ID: "pkg-1", Type: PackageTypeFoundryModule},
			want: "/admin/foundry-vtt/packages/pkg-1/actions-fragment",
		},
		{
			name: "system returns empty (no type-specific fragment)",
			pkg:  Package{ID: "sys-1", Type: PackageTypeSystem},
			want: "",
		},
		{
			name: "unknown type returns empty (defensive default)",
			pkg:  Package{ID: "unk-1", Type: PackageType("unknown-type")},
			want: "",
		},
		{
			name: "empty ID still produces a URL for foundry-module (validation is the handler's job)",
			pkg:  Package{ID: "", Type: PackageTypeFoundryModule},
			want: "/admin/foundry-vtt/packages//actions-fragment",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := actionsFragmentURLFor(tc.pkg)
			if got != tc.want {
				t.Errorf("actionsFragmentURLFor(%+v) = %q, want %q", tc.pkg, got, tc.want)
			}
		})
	}
}
