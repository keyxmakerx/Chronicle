package systems

import "testing"

// TestVersionLess pins the semver-aware comparison used to pick the latest
// installed package version. The cases that matter are the ones a plain string
// sort gets wrong (double-digit components).
func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.0.9", "0.10.0", true},   // the bug: string sort says "0.0.9" > "0.10.0"
		{"0.10.0", "0.0.9", false},  // ...and the reverse
		{"0.0.9", "0.0.10", true},   // double-digit patch
		{"0.2.0", "0.10.0", true},   // string sort says "0.2.0" > "0.10.0"
		{"1.0.0", "0.9.9", false},   // major beats minor/patch
		{"0.0.7", "0.0.9", true},    // simple ascending
		{"0.10.0", "0.10.0", false}, // equal is not less
		{"1.2", "1.2.3", true},      // shorter, missing component treated as 0
		{"1.2.0", "1.2", false},     // ...and the reverse is not less
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestLatestVersionSelection mirrors the selection loop in ScanPackageDir: the
// highest semver wins regardless of os.ReadDir's alphabetical name order.
func TestLatestVersionSelection(t *testing.T) {
	// As os.ReadDir would return them: sorted as strings.
	dirs := []string{"0.0.7", "0.0.9", "0.10.0"}
	var latest string
	for _, d := range dirs {
		if latest == "" || versionLess(latest, d) {
			latest = d
		}
	}
	if latest != "0.10.0" {
		t.Fatalf("latest = %q, want 0.10.0 (string sort would wrongly pick 0.0.9)", latest)
	}
}
