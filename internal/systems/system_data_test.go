package systems

import "testing"

// TestSystemDataFilePattern pins the filename allow-list for SystemDataAPI.
// The pattern is the security-critical surface (the serving logic mirrors the
// already-covered RulesGlossaryAPI): it must accept plain JSON basenames and
// reject anything that could escape the system's data/ dir.
func TestSystemDataFilePattern(t *testing.T) {
	cases := []struct {
		name string
		file string
		want bool
	}{
		{"plain json", "skills.json", true},
		{"hyphenated", "rules-glossary.json", true},
		{"underscored", "creature_keywords.json", true},
		{"alphanumeric", "skills2.json", true},
		{"empty", "", false},
		{"no extension", "skills", false},
		{"wrong extension", "skills.yaml", false},
		{"just extension", ".json", false},
		{"leading dot", ".env.json", false},
		{"double extension", "skills.json.json", false},
		{"parent traversal", "../secret.json", false},
		{"nested path", "sub/skills.json", false},
		{"backslash traversal", "..\\secret.json", false},
		{"absolute path", "/etc/passwd.json", false},
		{"embedded dot", "a.b.json", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := systemDataFilePattern.MatchString(tc.file); got != tc.want {
				t.Errorf("systemDataFilePattern.MatchString(%q) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}
