package packages

import (
	"strings"
	"testing"
)

func TestValidVersionString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"semver", "1.2.3", true},
		{"prerelease", "2.0.0-beta.1", true},
		{"v prefix", "v1.0", true},
		{"build metadata", "1.0.0+build.5", true},
		{"50 chars", strings.Repeat("a", 50), true},
		{"51 chars", strings.Repeat("a", 51), false},
		{"empty", "", false},
		{"dot dot", "..", false},
		{"leading traversal", "../x", false},
		{"embedded traversal", "1.0/../2.0", false},
		{"dots inside segment", "1..2", false},
		{"slash", "a/b", false},
		{"backslash", `a\b`, false},
		{"leading dot", ".hidden", false},
		{"leading dash", "-1", false},
		{"space", "1 0", false},
		{"nul", "1\x00", false},
		{"trailing newline", "1.0\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidVersionString(tt.in); got != tt.want {
				t.Errorf("ValidVersionString(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestInstallDirForVersion_RefusesInvalid(t *testing.T) {
	s := &packageService{mediaDir: t.TempDir()}
	for _, v := range []string{"", "..", "../x", "1.0/../2.0", "a/b", `a\b`, strings.Repeat("a", 51)} {
		for _, typ := range []PackageType{PackageTypeFoundryModule, PackageTypeSystem} {
			if got := s.InstallDirForVersion(typ, "slug", v); got != "" {
				t.Errorf("InstallDirForVersion(%q, %q) = %q, want empty", typ, v, got)
			}
			if got := s.installDir(typ, "slug", v); got != "" {
				t.Errorf("installDir(%q, %q) = %q, want empty", typ, v, got)
			}
		}
	}
	if got := s.InstallDirForVersion(PackageTypeFoundryModule, "slug", "1.2.3"); got == "" {
		t.Error("valid version should resolve to a path")
	}
}
