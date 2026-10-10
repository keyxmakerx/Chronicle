package aiexport

import "testing"

func TestHtmlToMarkdown_MentionsBecomePageLinks(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"editor mention", `<p>See <a data-mention-id="abc" href="/campaigns/c/entities/abc" data-entity-preview="/campaigns/c/entities/abc/preview">@Lyra Dawn</a> now.</p>`, "See @[Lyra Dawn] now."},
		{"plain link untouched", `<p><a href="https://example.com/x">site</a></p>`, "[site](https://example.com/x)"},
		{"brackets stripped", `<p><a data-mention-id="a" href="/x">@A [b]|c</a></p>`, "@[A bc]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			got, err := htmlToMarkdown(&in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
