package sanitize

import "testing"

// HasSecrets must say yes for exactly what the strip functions remove, or a
// writer served the stripped copy could save over GM-only content.
func TestHasSecrets(t *testing.T) {
	tests := []struct {
		name string
		json string
		html string
		want bool
	}{
		{"empty", "", "", false},
		{"plain html", "", `<p>The harbor at dawn</p>`, false},
		{"secret span", "", `<p>a <span data-secret="true">the mayor lies</span></p>`, true},
		{"gm picture", "", `<figure class="ce-img ce-img--gm"><img src="/x.png"></figure>`, true},
		{"roller", "", `<div class="ce-roll" data-table="t1">Roll</div>`, true},
		{"plain json", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hi"}]}]}`, "", false},
		{"secret mark", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"secret"}]}]}]}`, "", true},
		{"gm picture node", `{"type":"doc","content":[{"type":"chronicleImage","attrs":{"gmOnly":true}}]}`, "", true},
		{"open picture node", `{"type":"doc","content":[{"type":"chronicleImage","attrs":{"gmOnly":false}}]}`, "", false},
		{"gm diagram node", `{"type":"doc","content":[{"type":"diagram","attrs":{"gmOnly":true}}]}`, "", true},
		{"roll table node", `{"type":"doc","content":[{"type":"rollTable"}]}`, "", true},
		{"not json", `not json`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasSecrets(tt.json, tt.html); got != tt.want {
				t.Errorf("HasSecrets = %v, want %v", got, tt.want)
			}
			// Whatever HasSecrets flags, the strip must change; whatever it
			// clears, the strip must leave alone.
			if tt.html != "" && (StripSecretsHTML(tt.html) != tt.html) != tt.want {
				t.Errorf("StripSecretsHTML disagrees with HasSecrets for %q", tt.html)
			}
		})
	}
}
