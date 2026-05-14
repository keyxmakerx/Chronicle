package foundry_modules

import "testing"

// TestSemverLess pins the ordering relation used by the
// "notify older-version campaigns" repository query. Test cases
// span the variants that show up in Foundry module versioning:
// vanilla SemVer, leading "v", pre-release tags, segment-count
// mismatch.
func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.1.0", "0.1.1", true},
		{"0.1.1", "0.1.0", false},
		{"0.1.5", "0.2.0", true},
		{"1.0.0", "0.9.9", false},
		{"0.1", "0.1.0", false},          // missing segment defaults to 0
		{"v0.1.0", "0.1.1", true},        // leading v stripped
		{"0.2.0-beta.1", "0.2.0", true},  // pre-release < release
		{"0.2.0", "0.2.0-beta.1", false}, // release > pre-release
		{"0.2.0-alpha", "0.2.0-beta", true},
		{"0.2.0", "0.2.0", false}, // equal
	}
	for _, tc := range cases {
		got := semverLess(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("semverLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
