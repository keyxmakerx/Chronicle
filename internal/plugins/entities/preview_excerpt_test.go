package entities

import (
	"strings"
	"testing"
)

func TestPreviewExcerpt_GMOnlyContent(t *testing.T) {
	const entry = `<p>The innkeeper is friendly. <span data-secret="true">She is the cult leader.</span></p>` +
		`<figure class="ce-img ce-img--gm"><img src="/media/a"><figcaption>Hidden vault</figcaption></figure>` +
		`<p>Open late.</p>`

	tests := []struct {
		name     string
		canSeeGM bool
		wantGM   bool
	}{
		{"player sees no GM-only text", false, false},
		{"scribe and owner see everything", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := previewExcerpt(entry, tt.canSeeGM)
			for _, public := range []string{"The innkeeper is friendly.", "Open late."} {
				if !strings.Contains(got, public) {
					t.Errorf("previewExcerpt() = %q, missing public text %q", got, public)
				}
			}
			for _, secret := range []string{"cult leader", "Hidden vault"} {
				if strings.Contains(got, secret) != tt.wantGM {
					t.Errorf("previewExcerpt() = %q, GM-only %q shown = %v, want %v", got, secret, !tt.wantGM, tt.wantGM)
				}
			}
		})
	}
}

func TestPreviewExcerpt_Truncates(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		want  string
	}{
		{"empty", "", ""},
		{"short kept whole", "<p>Short   text</p>", "Short text"},
		{"long cut at a word", "<p>" + strings.Repeat("word ", 40) + "</p>", strings.TrimSpace(strings.Repeat("word ", 30)) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := previewExcerpt(tt.entry, false); got != tt.want {
				t.Errorf("previewExcerpt() = %q, want %q", got, tt.want)
			}
		})
	}
}
