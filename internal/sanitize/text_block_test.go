package sanitize

import "testing"

func TestTextBlock(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"empty", "  \n ", ""},
		{"one line", "Welcome, travellers.", "<p>Welcome, travellers.</p>"},
		{"paragraphs and line breaks", "First line\nsame paragraph\n\n  \nSecond", "<p>First line<br>same paragraph</p><p>Second</p>"},
		{"plain text is escaped", "a < b & c > d", "<p>a &lt; b &amp; c &gt; d</p>"},
		{"markup goes through the HTML policy", `<p>Hi <a href="/x">there</a></p><script>alert(1)</script>`, `<p>Hi <a href="/x" rel="nofollow">there</a></p>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TextBlock(tt.in); got != tt.want {
				t.Errorf("TextBlock(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
