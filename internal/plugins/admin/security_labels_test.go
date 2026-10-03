package admin

import "testing"

func TestRegistrationModeLabel(t *testing.T) {
	tests := []struct{ in, want string }{
		{"open", "open"},
		{"invite", "invite only"},
		{"closed", "closed"},
		{"other", "other"},
	}
	for _, tc := range tests {
		if got := registrationModeLabel(tc.in); got != tc.want {
			t.Errorf("registrationModeLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTokenHint_NeverFullHash(t *testing.T) {
	tests := []struct{ in, want string }{
		{"abcdef0123456789", "abcdef01"},
		{"abc", "abc"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := tokenHint(tc.in); got != tc.want {
			t.Errorf("tokenHint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
