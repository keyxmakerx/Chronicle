package packages

import "testing"

// TestLatestStableVersion pins that "latest" is the highest version number,
// not the release GitHub happens to list first, and that a pre-release wins
// only when nothing else exists.
func TestLatestStableVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []PackageVersion
		want     string
	}{
		{"maintenance release listed first", []PackageVersion{{Version: "1.9.5"}, {Version: "2.0.0"}, {Version: "1.9.4"}}, "2.0.0"},
		{"numeric, not text, order", []PackageVersion{{Version: "1.9.0"}, {Version: "1.10.0"}}, "1.10.0"},
		{"pre-release above the release is skipped", []PackageVersion{{Version: "3.0.0-rc1", Prerelease: true}, {Version: "2.1.0"}}, "2.1.0"},
		{"only pre-releases", []PackageVersion{{Version: "1.0.0-rc1", Prerelease: true}, {Version: "1.0.0-rc2", Prerelease: true}}, "1.0.0-rc2"},
		{"none", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := latestStableVersion(tt.versions)
			if (got == nil) != (tt.want == "") || (got != nil && got.Version != tt.want) {
				t.Fatalf("latestStableVersion = %+v, want %q", got, tt.want)
			}
		})
	}
}
