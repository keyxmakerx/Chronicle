package pagelink

import (
	"strings"
	"testing"
)

const id = "11111111-1111-4111-8111-111111111111"

func TestAnchor(t *testing.T) {
	got := Anchor("camp", id, "@A & B")
	want := `<a data-mention-id="` + id + `" href="/campaigns/camp/entities/` + id +
		`" data-entity-preview="/campaigns/camp/entities/` + id + `/preview">@A &amp; B</a>`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestRewriteLinks(t *testing.T) {
	tests := []struct {
		name, in string
		mention  bool
	}{
		{"this campaign's page", `<p><a href="/campaigns/camp/entities/` + id + `">x</a></p>`, true},
		{"other campaign", `<p><a href="/campaigns/other/entities/` + id + `">x</a></p>`, false},
		{"outside link", `<p><a href="https://example.com/">x</a></p>`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RewriteLinks(tc.in, "camp")
			if has := strings.Contains(got, `data-mention-id="`+id+`"`); has != tc.mention {
				t.Errorf("mention=%v, want %v: %s", has, tc.mention, got)
			}
		})
	}
}
