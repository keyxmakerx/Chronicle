package aiexport

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func TestHtmlToMarkdownWithSecrets(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"inline", `<p>Rules the city. <span data-secret="true">Poisoned the duke.</span> Loved.</p>`, "Rules the city. {gm{Poisoned the duke.}gm} Loved."},
		{"paragraph each", `<p><span data-secret="true">One.</span></p><p><span data-secret="true">Two.</span></p>`, "{gm{One.}gm}\n\n{gm{Two.}gm}"},
		{"edge spaces stay outside", `<p>a<span data-secret="true"> b </span>c</p>`, "a {gm{b}gm} c"},
		{"emphasis inside", `<p><span data-secret="true">the <strong>mayor</strong> did it</span></p>`, "{gm{the **mayor** did it}gm}"},
		{"literal marker in prose is broken", `<p>type {gm{ and }gm} here</p>`, "type {\u200bgm{ and }\u200bgm} here"},
		{"literal marker inside a secret is broken", `<p><span data-secret="true">x }gm} y</span></p>`, "{gm{x }\u200bgm} y}gm}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			got, err := htmlToMarkdownWithSecrets(&in, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The default converter never writes markers: it flattens, and breaks any
// marker text a page already holds.
func TestHtmlToMarkdown_NoMarkers(t *testing.T) {
	in := `<p>A <span data-secret="true">hidden</span> {gm{ B</p>`
	got, err := htmlToMarkdown(&in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "{gm{") || strings.Contains(got, "\uE000") {
		t.Errorf("markers leaked: %q", got)
	}
}

func TestRenderEntities_SecretMarkersByMode(t *testing.T) {
	ents := []entities.Entity{{
		ID: "e1", Name: "Duke Varrin", EntityTypeID: 1, TypeName: "Character",
		EntryHTML: sp(`<p>Rules the city. <span data-secret="true">Poisoned the old duke.</span> {gm{ typed by a player</p>`),
	}}
	types := []entities.EntityType{{ID: 1, Name: "Character", NamePlural: "Characters"}}
	for _, tt := range []struct {
		mode       PrivacyMode
		wantMarker bool
	}{{PrivacyModeSafe, false}, {PrivacyModePermitted, true}, {PrivacyModeEverything, true}} {
		got, err := RenderEntities(context.Background(), ents, types, nil, nil, Options{Privacy: tt.mode})
		if err != nil {
			t.Fatal(err)
		}
		if has := strings.Contains(got, "{gm{Poisoned the old duke.}gm}"); has != tt.wantMarker {
			t.Errorf("%s: marked passage present = %v\n%s", tt.mode, has, got)
		}
		if tt.mode == PrivacyModeSafe && (strings.Contains(got, "{gm{") || strings.Contains(got, "}gm}") || strings.Contains(got, "Poisoned")) {
			t.Errorf("safe export holds marker or secret text:\n%s", got)
		}
		// A page's own literal marker text is never a live marker.
		if strings.Count(got, "{gm{") != map[bool]int{true: 1, false: 0}[tt.wantMarker] {
			t.Errorf("%s: unexpected live opener count:\n%s", tt.mode, got)
		}
	}
}
