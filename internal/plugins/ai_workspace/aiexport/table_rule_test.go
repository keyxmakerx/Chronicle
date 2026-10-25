package aiexport

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

func TestHtmlToMarkdown_Tables(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"editor table, cells hold paragraphs",
			`<table><tbody><tr><th colspan="1" rowspan="1"><p>d6</p></th><th colspan="1" rowspan="1"><p>What waits</p></th></tr><tr><td colspan="1" rowspan="1"><p>1</p></td><td colspan="1" rowspan="1"><p>A <strong>rope</strong> bridge</p><p>over the drop</p></td></tr></tbody></table><p>After.</p>`,
			"| d6 | What waits |\n| --- | --- |\n| 1 | A **rope** bridge over the drop |\n\nAfter."},
		{"imported table, thead and line breaks between tags",
			"<p>Before.</p>\n<table>\n<thead>\n<tr>\n<th>Room</th>\n<th>Danger</th>\n</tr>\n</thead>\n<tbody>\n<tr>\n<td>Hall</td>\n<td>None</td>\n</tr>\n</tbody>\n</table>",
			"Before.\n\n| Room | Danger |\n| --- | --- |\n| Hall | None |"},
		{"pipes escaped, code spans too",
			`<table><tr><th>a|b</th><th><code>x|y</code></th></tr></table>`,
			"| a\\|b | `x\\|y` |\n| --- | --- |"},
		{"short and spanned rows padded to the widest",
			`<table><tr><th>A</th><th>B</th><th>C</th></tr><tr><td colspan="2">wide</td><td>c</td></tr><tr><td>only</td></tr></table>`,
			"| A | B | C |\n| --- | --- | --- |\n| wide |  | c |\n| only |  |  |"},
		{"mention in a cell", `<table><tr><th>Who</th></tr><tr><td><a data-mention-id="m" href="/x">@Lyra Dawn</a></td></tr></table>`,
			"| Who |\n| --- |\n| @[Lyra Dawn] |"},
		{"empty cell kept", `<table><tr><th>A</th><th>B</th></tr><tr><td><p></p></td><td>b</td></tr></table>`,
			"| A | B |\n| --- | --- |\n|  | b |"},
		{"no rows", `<p>x</p><table></table>`, "x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			got, err := htmlToMarkdown(&in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

// A GM-only passage in a cell keeps its markers for the owner's AI, and the
// table still reads back as a table, so a page read and sent back unchanged
// keeps its rows.
func TestHtmlToMarkdown_TableSecretsAndReadBack(t *testing.T) {
	in := `<table><tr><th>d4</th><th>Find</th></tr><tr><td>1</td><td>A key <span data-secret="true">to the vault</span></td></tr></table>`
	got, err := htmlToMarkdownWithSecrets(&in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "| 1 | A key {gm{to the vault}gm} |") {
		t.Fatalf("secret lost in cell:\n%s", got)
	}
	var out bytes.Buffer
	if err := goldmark.New(goldmark.WithExtensions(extension.GFM)).Convert([]byte(got), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	if strings.Count(html, "<tr>") != 2 || strings.Count(html, "<td>") != 2 || strings.Count(html, "<th>") != 2 {
		t.Errorf("does not read back as a 2x2 table:\n%s", html)
	}
}
