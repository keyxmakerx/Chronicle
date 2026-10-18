package systems

import "testing"

func TestPropText(t *testing.T) {
	props := map[string]any{
		"plain":    "Speed 5",
		"marked":   "Target is {@condition taunted} until {@duration EoT|the end of its turn}.",
		"number":   3,
		"brokenAt": "{@condition",
	}
	tests := []struct{ key, want string }{
		{"plain", "Speed 5"},
		{"marked", "Target is taunted until the end of its turn."},
		{"number", "3"},
		{"brokenAt", "{@condition"},
		{"missing", ""},
	}
	for _, tt := range tests {
		if got := propText(props, tt.key); got != tt.want {
			t.Errorf("propText(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}
