package htmlconv

import (
	"encoding/json"
	"strings"
	"testing"
)

// checkSchema walks a converted document and reports the ways a picture can
// make it invalid: a picture block inside paragraph or heading content, or a
// list item that does not open with a paragraph.
func checkSchema(t *testing.T, n map[string]any, problems *[]string) (pictures int) {
	t.Helper()
	typ, _ := n["type"].(string)
	content, _ := n["content"].([]any)
	if typ == "chronicleImage" {
		pictures++
	}
	if typ == "listItem" && len(content) > 0 {
		if first, _ := content[0].(map[string]any); first["type"] != "paragraph" {
			*problems = append(*problems, "list item opens with "+first["type"].(string))
		}
	}
	for _, c := range content {
		child := c.(map[string]any)
		if (typ == "paragraph" || typ == "heading") && child["type"] == "chronicleImage" {
			*problems = append(*problems, "picture inside "+typ)
		}
		pictures += checkSchema(t, child, problems)
	}
	return pictures
}

// A picture is a block in the editor. Written inside a list item, heading,
// table cell or paragraph it must come out as a valid document, never as a
// block nested in inline content.
func TestConvert_PictureInsideOtherBlocksStaysValid(t *testing.T) {
	const img = `<img src="/media/0b6a3f0e-8c1d-4f6a-9a51-2f3c4d5e6f70" alt="a">`
	cases := []struct {
		name, html string
		wantText   string // text that must survive, if any
	}{
		{"list item holding only a picture", `<ul><li>` + img + `</li></ul>`, ""},
		{"list item with words and a picture", `<ul><li>Map ` + img + `</li></ul>`, "Map"},
		{"list item with a linked picture", `<ul><li><a href="/x">` + img + `</a></li></ul>`, ""},
		{"heading holding only a picture", `<h1>` + img + `</h1>`, ""},
		{"heading with words and a picture", `<h2>Coast ` + img + `</h2>`, "Coast"},
		{"table cell", `<table><tr><td>` + img + `</td></tr></table>`, ""},
		{"table cell with words", `<table><tr><td>Here ` + img + `</td></tr></table>`, "Here"},
		{"paragraph with words around a picture", `<p>before ` + img + ` after</p>`, "after"},
		{"quote", `<blockquote><p>` + img + `</p></blockquote>`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Convert(c.html)
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal([]byte(got), &doc); err != nil {
				t.Fatal(err)
			}
			var problems []string
			if n := checkSchema(t, doc, &problems); n != 1 {
				t.Errorf("found %d pictures, want exactly 1:\n%s", n, got)
			}
			if len(problems) > 0 {
				t.Errorf("invalid document: %v\n%s", problems, got)
			}
			if c.wantText != "" && !strings.Contains(got, c.wantText) {
				t.Errorf("text %q was lost:\n%s", c.wantText, got)
			}
		})
	}
}
