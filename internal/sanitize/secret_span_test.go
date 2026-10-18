package sanitize

import "testing"

// Nested spans must not let the tail of a secret survive the strip.
func TestStripSecretsHTML_Spans(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		want      string
		wantCount int
	}{
		{"flat", `<p>a <span data-secret="true">x</span> b</p>`, `<p>a  b</p>`, 1},
		{"nested span", `<p>pub <span data-secret="true">a <span style="x">b</span> LEAK</span> end</p>`, `<p>pub  end</p>`, 1},
		{"deeply nested", `<span data-secret="true"><span><span>a</span>b</span>c</span>d`, `d`, 1},
		{"two secrets", `<p><span data-secret="true">a</span> mid <span data-secret="true">b</span></p>`, `<p> mid </p>`, 2},
		{"unclosed strips to end", `<p>pub <span data-secret="true">LEAK</p><p>more</p>`, `<p>pub `, 1},
		{"plain spans untouched", `<p><span class="a">x</span> <span>y</span></p>`, `<p><span class="a">x</span> <span>y</span></p>`, 0},
		{"plain span then secret", `<span>a</span><span data-secret>s</span><span>b</span>`, `<span>a</span><span>b</span>`, 1},
		{"multiline", "<span data-secret=\"true\">a\n<span>b</span>\nc</span>z", `z`, 1},
		{"attr value mentions span", `<span title="<span data-secret>">ok</span>`, `<span title="<span data-secret>">ok</span>`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripSecretsHTML(tt.in); got != tt.want {
				t.Errorf("StripSecretsHTML(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if gotN, _, _ := CountSecrets(tt.in); gotN != tt.wantCount {
				t.Errorf("CountSecrets(%q) passages = %d, want %d", tt.in, gotN, tt.wantCount)
			}
		})
	}
}
