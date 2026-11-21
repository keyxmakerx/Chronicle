package sanitize

import "testing"

// A marker stands where each GM-only part was, so a caller can say a part is
// not shown; with an empty marker the result is exactly StripSecretsHTML's.
func TestRedactSecretsHTML(t *testing.T) {
	tests := []struct {
		name, in, marker, want string
	}{
		{"passage", `<p>a <span data-secret="true">b</span> c</p>`, "#", `<p>a # c</p>`},
		{"nested passage is one part", `<p><span data-secret><span>x</span>y</span></p>`, "#", `<p>#</p>`},
		{"two passages", `<p><span data-secret>1</span><span data-secret>2</span></p>`, "#", `<p>##</p>`},
		{"nothing hidden", `<p>plain</p>`, "#", `<p>plain</p>`},
		{"empty marker strips", `<p>a <span data-secret>b</span>c</p>`, "", `<p>a c</p>`},
		{"empty html", "", "#", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactSecretsHTML(tt.in, tt.marker); got != tt.want {
				t.Errorf("RedactSecretsHTML() = %q, want %q", got, tt.want)
			}
			if tt.marker == "" && StripSecretsHTML(tt.in) != tt.want {
				t.Errorf("StripSecretsHTML() differs from the empty-marker redaction")
			}
		})
	}
}
