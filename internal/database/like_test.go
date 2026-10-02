package database

import "testing"

func TestContainsPattern(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "alice", "%alice%"},
		{"empty", "", "%%"},
		{"percent", "50%", "%50!%%"},
		{"underscore", "a_b", "%a!_b%"},
		{"escape char", "hi!", "%hi!!%"},
		{"all three", "!%_", "%!!!%!_%"},
		{"unicode", "Éowyn", "%Éowyn%"},
		{"sql metachars stay literal", "x' OR 1=1 --", "%x' OR 1=1 --%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsPattern(tt.in); got != tt.want {
				t.Errorf("ContainsPattern(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
