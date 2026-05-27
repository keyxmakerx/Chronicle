// Package htmlconv converts sanitized HTML to a Chronicle-shaped
// ProseMirror JSON document. Phase 5's committer dual-writes
// EntryHTML (the sanitize.HTML output) + Entry (this package's JSON
// output) so a freshly-imported entity opens cleanly in the TipTap
// editor on first edit — the editor reads Entry, not EntryHTML
// (static/js/widgets/editor.js:761).
//
// Schema target: TipTap StarterKit + Link + Table (per editor.js
// extensions inventory). Covered nodes/marks:
//
//   - doc, paragraph, text
//   - heading (attrs.level 1-6)
//   - bulletList, orderedList, listItem
//   - codeBlock (no language attribute in V1)
//   - blockquote, horizontalRule, hardBreak
//   - table, tableRow, tableHeader, tableCell
//   - marks: bold, italic, strike, code, link, underline
//
// Unrecognised tags fall back to a paragraph containing the
// concatenated text content of the element's descendants — better
// to lose styling than to crash on edge-case AI output.
//
// Per cordinator/reports/chronicle/2026-05-26-c-ai-workspace-scoping.md
// §1.4 (recommendation A — server-side conversion).
package htmlconv

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// Node is the ProseMirror JSON shape goldmark / TipTap consume. The
// struct uses interface{} for content + attrs because the JSON
// shape varies per node type (e.g. text has Text + Marks but no
// Content; heading has Content + attrs.level).
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
	Text    string         `json:"text,omitempty"`
}

// Mark is one inline mark (bold / italic / link / etc).
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Convert parses html and emits the ProseMirror JSON string the
// TipTap editor's commands.setContent expects. Empty input returns
// an empty document — TipTap renders that as an empty editor
// (caller can guard if it wants the field to stay NULL).
func Convert(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}
	body := doc.Find("body").First()
	if body.Length() == 0 {
		// No <body> wrapping — use the document root directly.
		body = doc.Selection
	}
	pm := Node{
		Type:    "doc",
		Content: convertChildren(body),
	}
	if len(pm.Content) == 0 {
		// TipTap requires at least one block node — emit an empty
		// paragraph so commands.setContent doesn't reject the doc.
		pm.Content = []Node{{Type: "paragraph"}}
	}
	out, err := json.Marshal(pm)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// convertChildren walks a goquery Selection and returns the
// ProseMirror node slice for each top-level child. Inline children
// (text / em / strong / etc.) without a wrapping block are
// gathered into an implicit paragraph.
func convertChildren(sel *goquery.Selection) []Node {
	var out []Node
	var inlineBuffer []Node

	flushInline := func() {
		if len(inlineBuffer) > 0 {
			out = append(out, Node{Type: "paragraph", Content: inlineBuffer})
			inlineBuffer = nil
		}
	}

	sel.Contents().Each(func(_ int, child *goquery.Selection) {
		nodes, inline := convertNode(child)
		if inline {
			inlineBuffer = append(inlineBuffer, nodes...)
		} else {
			flushInline()
			out = append(out, nodes...)
		}
	})
	flushInline()
	return out
}

// convertNode dispatches one DOM node to its node-type handler.
// Returns the resulting ProseMirror nodes + a flag indicating
// whether they're inline (must be wrapped in a paragraph by the
// caller's flushInline).
func convertNode(s *goquery.Selection) ([]Node, bool) {
	n := s.Get(0)
	if n == nil {
		return nil, false
	}
	switch n.Type {
	case html.ElementNode:
		return convertElement(s)
	case html.TextNode:
		text := n.Data
		if strings.TrimSpace(text) == "" {
			return nil, true
		}
		return []Node{{Type: "text", Text: text}}, true
	}
	return nil, false
}

// convertElement is the per-tag dispatch table. Each branch returns
// (nodes, inline). Block-level tags return (nodes, false); inline
// (mark / text) tags return (nodes, true).
func convertElement(s *goquery.Selection) ([]Node, bool) {
	tag := goquery.NodeName(s)
	switch tag {
	case "p":
		return []Node{{Type: "paragraph", Content: convertInline(s)}}, false
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level, _ := strconv.Atoi(tag[1:])
		return []Node{{
			Type:    "heading",
			Attrs:   map[string]any{"level": level},
			Content: convertInline(s),
		}}, false
	case "ul":
		return []Node{{Type: "bulletList", Content: convertListItems(s)}}, false
	case "ol":
		attrs := map[string]any{}
		if start, ok := s.Attr("start"); ok {
			if n, err := strconv.Atoi(start); err == nil {
				attrs["start"] = n
			}
		}
		node := Node{Type: "orderedList", Content: convertListItems(s)}
		if len(attrs) > 0 {
			node.Attrs = attrs
		}
		return []Node{node}, false
	case "li":
		return []Node{{Type: "listItem", Content: wrapAsListItemChildren(s)}}, false
	case "pre":
		// Goldmark emits <pre><code class="language-x">…</code></pre>
		// for fenced code; the inner <code> carries the body. Strip
		// the wrapping <code> + use the text content.
		return []Node{{
			Type:    "codeBlock",
			Content: []Node{{Type: "text", Text: s.Text()}},
		}}, false
	case "blockquote":
		return []Node{{Type: "blockquote", Content: convertChildren(s)}}, false
	case "hr":
		return []Node{{Type: "horizontalRule"}}, false
	case "br":
		return []Node{{Type: "hardBreak"}}, true
	case "table":
		return []Node{convertTable(s)}, false
	case "strong", "b", "em", "i", "u", "s", "del", "code", "a":
		// Inline marks — wrap each text descendant with the mark.
		return convertInlineWithMark(s, tag), true
	}
	// Unknown tag: surface its text content as plain inline text.
	if t := strings.TrimSpace(s.Text()); t != "" {
		return []Node{{Type: "text", Text: t}}, true
	}
	return nil, true
}

// convertInline walks an element's children expecting inline-only
// content (used for paragraph + heading bodies). Block children
// encountered inside an inline context are flattened to their text.
func convertInline(s *goquery.Selection) []Node {
	var out []Node
	s.Contents().Each(func(_ int, child *goquery.Selection) {
		nodes, _ := convertNode(child)
		out = append(out, nodes...)
	})
	return out
}

// convertInlineWithMark wraps each descendant text node with the
// given mark type. Nested marks accumulate (a <strong><em>X</em></strong>
// produces text X with marks [bold, italic]).
func convertInlineWithMark(s *goquery.Selection, tag string) []Node {
	mark := tagToMark(tag, s)
	if mark.Type == "" {
		return convertInline(s)
	}
	var out []Node
	for _, n := range convertInline(s) {
		if n.Type == "text" {
			n.Marks = append(n.Marks, mark)
		}
		out = append(out, n)
	}
	return out
}

// tagToMark maps an inline HTML tag to its TipTap mark type. Strike
// covers both <s> and <del>; bold covers <strong> + <b>; italic
// covers <em> + <i>.
func tagToMark(tag string, s *goquery.Selection) Mark {
	switch tag {
	case "strong", "b":
		return Mark{Type: "bold"}
	case "em", "i":
		return Mark{Type: "italic"}
	case "u":
		return Mark{Type: "underline"}
	case "s", "del":
		return Mark{Type: "strike"}
	case "code":
		return Mark{Type: "code"}
	case "a":
		attrs := map[string]any{}
		if href, ok := s.Attr("href"); ok {
			attrs["href"] = href
		}
		if target, ok := s.Attr("target"); ok {
			attrs["target"] = target
		}
		// Preserve the entity-mention attribute the editor's
		// MentionLink extension reads.
		if mid, ok := s.Attr("data-mention-id"); ok {
			attrs["data-mention-id"] = mid
		}
		return Mark{Type: "link", Attrs: attrs}
	}
	return Mark{}
}

// convertListItems walks <ul>/<ol> children, expecting only <li>.
// Non-<li> children are ignored (HTML5 parsers may emit text nodes
// for whitespace between list items).
func convertListItems(s *goquery.Selection) []Node {
	var out []Node
	s.Children().Each(func(_ int, child *goquery.Selection) {
		if goquery.NodeName(child) != "li" {
			return
		}
		nodes, _ := convertElement(child)
		out = append(out, nodes...)
	})
	return out
}

// wrapAsListItemChildren returns the inner children of a <li>
// guaranteed to be block-level (TipTap's listItem schema requires
// block-level children). Inline content gets wrapped in a paragraph.
func wrapAsListItemChildren(s *goquery.Selection) []Node {
	kids := convertChildren(s)
	if len(kids) == 0 {
		return []Node{{Type: "paragraph"}}
	}
	return kids
}

// convertTable produces a TipTap-shaped table. AI tools emit
// `<table><thead><tr><th>…</th></tr></thead><tbody><tr><td>…</td></tr></tbody></table>`;
// flatten thead+tbody into a single rows list.
func convertTable(s *goquery.Selection) Node {
	var rows []Node
	s.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		var cells []Node
		tr.Children().Each(func(_ int, td *goquery.Selection) {
			cellType := "tableCell"
			if goquery.NodeName(td) == "th" {
				cellType = "tableHeader"
			}
			cells = append(cells, Node{
				Type:    cellType,
				Content: wrapAsListItemChildren(td), // same block-content guarantee
			})
		})
		if len(cells) > 0 {
			rows = append(rows, Node{Type: "tableRow", Content: cells})
		}
	})
	return Node{Type: "table", Content: rows}
}
