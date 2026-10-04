package sanitize

import (
	"strings"
	"testing"
)

// The rolling-table roller is a DM tool: players get the text a DM put in
// the page, never the roller itself.
func TestStripSecretsHTML_DropsRoller(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "roller between paragraphs",
			in:   `<p>a</p><div class="ce-roll ce-roll--t-rumours ce-roll--n-3"></div><p>b</p>`,
			want: `<p>a</p><p>b</p>`,
		},
		{
			name: "two rollers",
			in:   `<div class="ce-roll ce-roll--t-omens ce-roll--n-1"></div><p>x</p><div class="ce-roll ce-roll--t-market ce-roll--n-5"></div>`,
			want: `<p>x</p>`,
		},
		{
			name: "look-alike class kept",
			in:   `<div class="ce-rollx"></div>`,
			want: `<div class="ce-rollx"></div>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripSecretsHTML(tt.in); got != tt.want {
				t.Errorf("StripSecretsHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripSecretsHTML_RollerSurvivesSanitizeThenStrips(t *testing.T) {
	stored := HTML(`<p>x</p><div class="ce-roll ce-roll--t-rumours ce-roll--n-3"></div>`)
	if !strings.Contains(stored, "ce-roll--t-rumours") {
		t.Fatalf("sanitizer dropped the roller's classes: %q", stored)
	}
	if got := StripSecretsHTML(stored); strings.Contains(got, "ce-roll") {
		t.Errorf("roller reached players: %q", got)
	}
}

func TestStripSecretsJSON_DropsRoller(t *testing.T) {
	in := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"kept"}]},{"type":"rollTable","attrs":{"table":"rumours","count":3}}]}`
	got := StripSecretsJSON(in)
	if strings.Contains(got, "rollTable") {
		t.Errorf("roller reached players: %s", got)
	}
	if !strings.Contains(got, "kept") {
		t.Errorf("ordinary text lost: %s", got)
	}
}
