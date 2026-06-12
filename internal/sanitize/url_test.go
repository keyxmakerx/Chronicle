package sanitize

import "testing"

func TestSafeLinkURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		// Accepted.
		{"http", "http://example.com/x", true},
		{"https", "https://example.com/x?q=1#h", true},
		{"relative path", "/campaigns/abc/entities", true},
		{"relative root", "/", true},
		{"https mixed case scheme", "HtTpS://example.com", true},

		// Rejected — dangerous schemes.
		{"javascript", "javascript:alert(1)", false},
		{"javascript uppercase", "JAVASCRIPT:alert(1)", false},
		{"javascript leading space", " javascript:alert(1)", false},
		{"javascript embedded tab", "java\tscript:alert(1)", false},
		{"javascript newline", "java\nscript:alert(1)", false},
		{"data", "data:text/html,<script>alert(1)</script>", false},
		{"vbscript", "vbscript:msgbox(1)", false},

		// Rejected — open-redirect shapes.
		{"protocol-relative", "//evil.com", false},
		{"backslash protocol-relative", "/\\evil.com", false},
		{"double backslash", "\\\\evil.com", false},

		// Rejected — misc.
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"bare scheme no slashes", "https:evil", false},
		{"mailto", "mailto:x@y.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SafeLinkURL(tt.in)
			if ok != tt.ok {
				t.Fatalf("SafeLinkURL(%q) ok=%v, want %v (got %q)", tt.in, ok, tt.ok, got)
			}
			if !ok && got != "" {
				t.Errorf("rejected URL must return empty string, got %q", got)
			}
		})
	}
}
