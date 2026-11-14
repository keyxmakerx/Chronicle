package sessions

import "testing"

// A recap made only of GM-only parts has nothing a player can read, so it
// must not announce itself to one; the DM team still reads it.
func TestRecapReadable(t *testing.T) {
	secret := `<p><span data-secret="true">The duke is the cults leader.</span></p>`
	mixed := `<p>The party reached Harrowgate.</p>` + secret
	tests := []struct {
		name   string
		html   string
		viewer SinceViewer
		want   bool
	}{
		{"plain recap, player", `<p>The party reached Harrowgate.</p>`, SinceViewer{}, true},
		{"only secrets, player", secret, SinceViewer{}, false},
		{"only secrets, DM team", secret, SinceViewer{DMTeam: true}, true},
		{"secrets and text, player", mixed, SinceViewer{}, true},
		{"empty markup", `<p> </p><br>`, SinceViewer{}, false},
		{"empty markup, DM team", `<p> </p>`, SinceViewer{DMTeam: true}, false},
		{"a picture alone", `<img src="/m/a.png">`, SinceViewer{}, true},
	}
	for _, tt := range tests {
		if got := recapReadable(tt.html, tt.viewer); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
