// Package htmlconv converts sanitized HTML to a Chronicle-shaped
// ProseMirror JSON document. The committer dual-writes EntryHTML (the
// sanitize.HTML output) and Entry (this package's JSON output) so a
// freshly-imported entity opens cleanly in the TipTap editor on first
// edit — the editor reads Entry, not EntryHTML.
//
// Schema target: TipTap StarterKit + Link + Table. Covered
// nodes/marks: doc, paragraph, text, heading (level 1-6), bulletList,
// orderedList, listItem, codeBlock (no language attribute), blockquote,
// horizontalRule, hardBreak, table/tableRow/tableHeader/tableCell,
// marks bold/italic/strike/code/link/underline, and the editor's own
// picture (chronicleImage, from a figure.ce-img or an img with a /media/<id> src).
//
// Unrecognised tags fall back to a paragraph containing the
// concatenated text content of the element's descendants — better to
// lose styling than to crash on edge-case AI output.
package htmlconv

import (
	"encoding/json"
	"fmt"
	"regexp"
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
//
// Operator-facing errors are friendly — no raw library prefixes
// (`goquery:`, `json:`) reach the review/result UIs.
func Convert(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", fmt.Errorf("could not parse the page HTML — try simpler markdown: %w", err)
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
		return "", fmt.Errorf("could not convert the page to editor format: %w", err)
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
			out = append(out, hoistPictures(Node{Type: "paragraph", Content: inlineBuffer})...)
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
		return hoistPictures(Node{Type: "paragraph", Content: convertInline(s)}), false
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level, _ := strconv.Atoi(tag[1:])
		return hoistPictures(Node{
			Type:    "heading",
			Attrs:   map[string]any{"level": level},
			Content: convertInline(s),
		}), false
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
		kids := wrapAsListItemChildren(s)
		// A list item must open with a paragraph; a picture alone in an item
		// ("- ![a](/media/id)") gets an empty one in front.
		if len(kids) > 0 && kids[0].Type != "paragraph" {
			kids = append([]Node{{Type: "paragraph"}}, kids...)
		}
		return []Node{{Type: "listItem", Content: kids}}, false
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
	case "figure":
		// The editor's own picture: <figure class="ce-img ..."><img src="/media/id">.
		if n, ok := pictureNode(s); ok {
			return []Node{n}, false
		}
		return convertChildren(s), false
	case "img":
		if n, ok := pictureNode(s); ok {
			return []Node{n}, false
		}
		return nil, true
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

// mediaSrcRe matches only this site's own media path; a picture from another
// website is never stored as a hotlink.
var mediaSrcRe = regexp.MustCompile(`^/media/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// ceWidthRe reads the width step from the editor's figure classes.
var ceWidthRe = regexp.MustCompile(`(?:^|\s)ce-img--w(\d{1,3})(?:\s|$)`)

// pictureNode builds the editor's chronicleImage node from a <figure> or a
// bare <img> whose source is a /media/<id> path. ok is false for anything else.
func pictureNode(s *goquery.Selection) (Node, bool) {
	img := s
	isFigure := goquery.NodeName(s) == "figure"
	if isFigure {
		img = s.Find("img").First()
	}
	src, _ := img.Attr("src")
	m := mediaSrcRe.FindStringSubmatch(src)
	if m == nil {
		return Node{}, false
	}
	alt, _ := img.Attr("alt")
	attrs := map[string]any{"mediaId": m[1], "alt": alt, "caption": "", "width": 100, "align": "center", "gmOnly": false}
	if isFigure {
		class, _ := s.Attr("class")
		padded := " " + class + " "
		if w := ceWidthRe.FindStringSubmatch(class); w != nil {
			if n, err := strconv.Atoi(w[1]); err == nil {
				attrs["width"] = n
			}
		}
		for _, a := range []string{"left", "right", "center"} {
			if strings.Contains(padded, " ce-img--"+a+" ") {
				attrs["align"] = a
			}
		}
		attrs["gmOnly"] = strings.Contains(padded, " ce-img--gm ")
		attrs["caption"] = strings.TrimSpace(s.Find("figcaption").First().Text())
	}
	return Node{Type: "chronicleImage", Attrs: attrs}, true
}

// hoistPictures keeps a picture out of a paragraph or heading: the editor's
// picture is a block, and a block inside inline content is an invalid document.
// The pictures follow the block they were written in; a block left empty by
// the move is dropped rather than kept as a blank line.
func hoistPictures(n Node) []Node {
	var kept, pics []Node
	for _, c := range n.Content {
		if c.Type == "chronicleImage" {
			pics = append(pics, c)
		} else {
			kept = append(kept, c)
		}
	}
	if len(pics) == 0 {
		return []Node{n}
	}
	if len(kept) == 0 {
		return pics
	}
	n.Content = kept
	return append([]Node{n}, pics...)
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
		// The hover-card address the editor writes beside a mention.
		if prev, ok := s.Attr("data-entity-preview"); ok {
			attrs["data-entity-preview"] = prev
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
