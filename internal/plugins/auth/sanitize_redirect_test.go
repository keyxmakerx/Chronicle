package auth

import "testing"

func TestSanitizeRedirect(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain path", "/campaigns/abc", "/campaigns/abc"},
		{"path with query", "/invites/accept?token=x&y=1", "/invites/accept?token=x&y=1"},
		{"path with fragment", "/a#b", "/a#b"},
		{"relative", "campaigns", ""},
		{"absolute url", "https://evil.example/", ""},
		{"protocol relative", "//evil.example", ""},
		{"slash backslash", "/\\evil.example", ""},
		{"slash tab slash", "/\t/evil.example", ""},
		{"slash newline slash", "/\n/evil.example", ""},
		{"slash CR slash", "/\r/evil.example", ""},
		{"nul byte", "/a\x00b", ""},
		{"del byte", "/a\x7fb", ""},
		{"backslash later", "/a\\b", ""},
		{"unparseable", "/%zz", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeRedirect(tt.in); got != tt.want {
				t.Errorf("sanitizeRedirect(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
