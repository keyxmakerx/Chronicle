// Tests for the semver-ish comparator used by the admin's "notify
// older campaigns" + "force-update all older" mass actions. Mirrors
// the original foundry_modules/semver_test.go cases plus the
// Foundry-dialect cases (leading "v", pre-release tags) that the
// production catalog actually emits.
package foundry_vtt

import "testing"

func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
		name string
	}{
		{"0.1.0", "0.2.0", true, "minor bump"},
		{"0.2.0", "0.1.0", false, "minor bump reversed"},
		{"0.1.0", "0.1.0", false, "equal"},
		{"v0.1.5", "v0.1.10", true, "double-digit patch handled numerically"},
		{"v0.1.10", "v0.1.5", false, "double-digit patch reversed"},
		{"V1.0.0", "v1.0.1", true, "uppercase V leading also stripped"},
		{"0.2.0-beta", "0.2.0", true, "pre-release sorts before release"},
		{"0.2.0", "0.2.0-beta", false, "release sorts after pre-release"},
		{"0.2.0-alpha", "0.2.0-beta", true, "pre-release lex compare"},
		{"1.0", "1.0.0", false, "missing trailing segment treated as 0"},
		{"1.0", "1.0.1", true, "missing trailing segment vs patch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := semverLess(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("semverLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
