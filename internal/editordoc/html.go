// Package editordoc renders the page editor's ProseMirror JSON as HTML on the
// server, for bodies that arrive as editor JSON without the HTML copy the
// browser editor would have rendered (REST API clients).
package editordoc

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
)

// node is one ProseMirror node or mark.
type node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs"`
	Content []node         `json:"content"`
	Text    string         `json:"text"`
	Marks   []node         `json:"marks"`
}

// blockTags maps the StarterKit and table nodes to their element.
var blockTags = map[string]string{
	"paragraph":   "p",
	"blockquote":  "blockquote",
	"bulletList":  "ul",
	"listItem":    "li",
	"table":       "table",
	"tableRow":    "tr",
	"tableHeader": "th",
	"tableCell":   "td",
}

// markTags maps the simple formatting marks to their element.
var markTags = map[string]string{
	"bold":      "strong",
	"italic":    "em",
	"underline": "u",
	"strike":    "s",
	"code":      "code",
}

// ToHTML renders an editor document. ok is false when doc is not a
// ProseMirror document. A node type this renderer does not know is left out
// with everything inside it: an editor extension can carry GM-only content
// (pictures, diagrams) that only the browser editor knows how to mark, so
// guessing would risk showing it to players. Opening the page in the editor
// and saving renders it in full.
func ToHTML(doc string) (out string, ok bool) {
	var root node
	if err := json.Unmarshal([]byte(doc), &root); err != nil || root.Type != "doc" {
		return "", false
	}
	var b strings.Builder
	renderChildren(&b, root.Content)
	return b.String(), true
}

func renderChildren(b *strings.Builder, nodes []node) {
	for _, n := range nodes {
		render(b, n)
	}
}

func render(b *strings.Builder, n node) {
	switch n.Type {
	case "text":
		renderText(b, n)
	case "hardBreak":
		b.WriteString("<br>")
	case "horizontalRule":
		b.WriteString("<hr>")
	case "heading":
		level := intAttr(n.Attrs, "level", 1)
		if level < 1 || level > 6 {
			level = 1
		}
		tag := "h" + strconv.Itoa(level)
		wrap(b, tag, "", n.Content)
	case "orderedList":
		attrs := ""
		if start := intAttr(n.Attrs, "start", 1); start != 1 {
			attrs = ` start="` + strconv.Itoa(start) + `"`
		}
		wrap(b, "ol", attrs, n.Content)
	case "codeBlock":
		b.WriteString("<pre><code>")
		for _, c := range n.Content {
			b.WriteString(html.EscapeString(c.Text))
		}
		b.WriteString("</code></pre>")
	default:
		if tag, known := blockTags[n.Type]; known {
			wrap(b, tag, "", n.Content)
		}
	}
}

func wrap(b *strings.Builder, tag, attrs string, content []node) {
	b.WriteString("<" + tag + attrs + ">")
	renderChildren(b, content)
	b.WriteString("</" + tag + ">")
}

// renderText writes a text node inside its marks, outermost first.
func renderText(b *strings.Builder, n node) {
	var closers []string
	for _, m := range n.Marks {
		switch m.Type {
		case "secret":
			b.WriteString(`<span data-secret="true" class="chronicle-secret">`)
			closers = append(closers, "</span>")
		case "link":
			href, _ := m.Attrs["href"].(string)
			b.WriteString(`<a href="` + html.EscapeString(href) + `"`)
			if id, _ := m.Attrs["data-mention-id"].(string); id != "" {
				b.WriteString(` data-mention-id="` + html.EscapeString(id) + `" class="entity-link"`)
			}
			b.WriteString(">")
			closers = append(closers, "</a>")
		default:
			if tag, known := markTags[m.Type]; known {
				b.WriteString("<" + tag + ">")
				closers = append(closers, "</"+tag+">")
			}
		}
	}
	b.WriteString(html.EscapeString(n.Text))
	for i := len(closers) - 1; i >= 0; i-- {
		b.WriteString(closers[i])
	}
}

// intAttr reads a numeric attribute, which JSON decodes as float64.
func intAttr(attrs map[string]any, key string, def int) int {
	if v, ok := attrs[key].(float64); ok {
		return int(v)
	}
	return def
}
