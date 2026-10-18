package packages

import "testing"

// TestPruneVersionLess pins the ordering that decides which installed version
// is newest: a pre-release must not outrank its release, and a pre-release of
// a newer version must not fall below an older release.
func TestPruneVersionLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"0.0.9", "0.10.0", true},
		{"0.10.0", "0.0.9", false},
		{"1.2.0-beta", "1.2.0", true},
		{"1.2.0", "1.2.0-beta", false},
		{"1.2.3-rc1", "1.2.2", false},
		{"1.2.2", "1.2.3-rc1", true},
		{"v1.2.0", "1.2.0", false},
		{"v1.2.0", "v1.10.0", true},
		{"1.2", "1.2.0", false},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_lt_"+tc.b, func(t *testing.T) {
			if got := pruneVersionLess(tc.a, tc.b); got != tc.want {
				t.Errorf("pruneVersionLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
