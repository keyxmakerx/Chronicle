package semver

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.0.9", "0.10.0", -1},
		{"0.10.0", "0.0.9", 1},
		{"1.2", "1.2.0", 0},
		{"1.2", "1.2.3", -1},
		{"v1.2.0", "1.2.0", 0},
		{"v1.10.0", "1.9.0", 1},
		{"1.2.0-beta", "1.2.0", -1},
		{"1.2.0", "1.2.0-beta", 1},
		{"1.2.3-rc1", "1.2.2", 1},
		{"1.2.2", "1.2.3-rc1", -1},
		{"1.2.3-rc1", "1.2.3-rc2", -1},
		{"1.2.3-rc.2", "1.2.3-rc.10", -1},
		{"1.2.3-beta", "1.2.3-rc", -1},
		{"1.2.3-1", "1.2.3-beta", -1},
		{"1.2.3-rc1", "1.2.3-rc1", 0},
		{"1.2.3+build5", "1.2.3", 0},
		{"1.2.x", "1.2.0", 0},
		{"2.0.0-alpha", "1.9.9", 1},
	}
	for _, tc := range tests {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			if got := Compare(tc.a, tc.b); got != tc.want {
				t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
			if got := Compare(tc.b, tc.a); got != -tc.want {
				t.Errorf("Compare(%q, %q) = %d, want %d (antisymmetry)", tc.b, tc.a, got, -tc.want)
			}
		})
	}
}
