package entities

import (
	"net/http"
	"strings"
	"testing"
)

// The hover card's excerpt is built from the text as its reader is served
// it: no GM-only parts and no name of a page they can't open.
func TestPreviewExcerpt_ServedText(t *testing.T) {
	const entry = `<p>The innkeeper is friendly. <span data-secret="true">She is the cult leader.</span></p>` +
		`<figure class="ce-img ce-img--gm"><img src="/media/a"><figcaption>Hidden vault</figcaption></figure>` +
		`<p>Open late, says <a data-mention-id="` + hiddenPageID + `">The smuggler king</a>.</p>`

	tests := []struct {
		name   string
		dm     bool
		wantGM bool
	}{
		{"player view sees no GM-only text or hidden name", false, false},
		{"owner and Co-DMs see everything", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{service: &hiddenPartsSvc{}}
			c, _ := hiddenPartsCtx(scribeCC(tt.dm), http.MethodGet, "")
			stored := entry
			_, served, _ := h.servedText(c, scribeCC(tt.dm), nil, &stored)
			got := previewExcerpt(*served)
			for _, public := range []string{"The innkeeper is friendly.", "Open late, says"} {
				if !strings.Contains(got, public) {
					t.Errorf("previewExcerpt() = %q, missing public text %q", got, public)
				}
			}
			for _, secret := range []string{"cult leader", "Hidden vault", "smuggler king"} {
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
			if got := previewExcerpt(tt.entry); got != tt.want {
				t.Errorf("previewExcerpt() = %q, want %q", got, tt.want)
			}
		})
	}
}
