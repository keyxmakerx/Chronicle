package notes

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// A note's body links out two ways, both as anchors in entry_html:
//
//	<a data-note-id="…">Journal note</a>   a [[link]] to another note
//	<a data-mention-id="…">@Name</a>       a link to a campaign page
//
// A note link never stores the target's title: the anchor text is a fixed
// word, and every reader labels it through the viewer's own visibility
// (Labels, or the index). So no copy of the HTML — an export, the REST API,
// an old version — can carry a title its reader may not see.

// Link kinds.
const (
	LinkNote = "note"
	LinkPage = "page"
)

// NoteLinkText is the stored text of every [[note]] anchor.
const NoteLinkText = "Journal note"

// Link is one outgoing link in a note body. Label is set for page links
// only: the page name as written in the anchor, which the reader of this
// body can already see.
type Link struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// idPattern is the shape of a note or entity id; anything else in a link
// attribute is ignored rather than trusted.
var idPattern = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

// ParseLinks returns the distinct outgoing links of a note body, in order.
func ParseLinks(body string) []Link {
	var out []Link
	seen := map[string]bool{}
	walkAnchors(body, func(kind, id, text string) {
		key := kind + ":" + id
		if seen[key] {
			return
		}
		seen[key] = true
		l := Link{Kind: kind, ID: id}
		if kind == LinkPage {
			l.Label = strings.TrimPrefix(strings.TrimSpace(text), "@")
		}
		out = append(out, l)
	})
	return out
}

// PlainText returns a body's text with whitespace collapsed. Each note link
// reads as label(id): the target's title when the reader may see it, or a
// neutral word when not, never the stored anchor text.
func PlainText(body string, label func(noteID string) string) string {
	if body == "" {
		return ""
	}
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(body))
	skipDepth := 0 // inside a note-link anchor whose text is replaced
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(b.String()), " ")
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			if isBlock(tok.Data) {
				b.WriteByte(' ')
			}
			if tok.Data == "a" {
				if id, ok := attr(tok, "data-note-id"); ok && idPattern.MatchString(id) {
					b.WriteString(label(id))
					skipDepth++
				}
			}
		case html.EndTagToken:
			tok := z.Token()
			if tok.Data == "a" && skipDepth > 0 {
				skipDepth--
			}
			if isBlock(tok.Data) {
				b.WriteByte(' ')
			}
		case html.TextToken:
			if skipDepth == 0 {
				b.Write(z.Text())
			}
		}
	}
}

// walkAnchors calls fn for each note or page anchor in body.
func walkAnchors(body string, fn func(kind, id, text string)) {
	if body == "" || !strings.Contains(body, "data-") {
		return
	}
	z := html.NewTokenizer(strings.NewReader(body))
	var kind, id string
	var text strings.Builder
	inAnchor := false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return
		case html.StartTagToken:
			tok := z.Token()
			if tok.Data != "a" {
				continue
			}
			kind, id = "", ""
			if v, ok := attr(tok, "data-note-id"); ok && idPattern.MatchString(v) {
				kind, id = LinkNote, v
			} else if v, ok := attr(tok, "data-mention-id"); ok && idPattern.MatchString(v) {
				kind, id = LinkPage, v
			}
			inAnchor = kind != ""
			text.Reset()
		case html.TextToken:
			if inAnchor {
				text.Write(z.Text())
			}
		case html.EndTagToken:
			if inAnchor && z.Token().Data == "a" {
				fn(kind, id, text.String())
				inAnchor = false
			}
		}
	}
}

func attr(tok html.Token, name string) (string, bool) {
	for _, a := range tok.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

// isBlock reports tags whose boundary separates words in plain text.
func isBlock(tag string) bool {
	switch tag {
	case "p", "div", "br", "li", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6",
		"blockquote", "pre", "tr", "td", "th", "table", "hr":
		return true
	}
	return false
}

// snippet returns up to max runes of text, cut at a word boundary with an
// ellipsis when shortened.
func snippet(text string, max int) string {
	r := []rune(text)
	if len(r) <= max {
		return text
	}
	cut := string(r[:max])
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "…"
}

// matchSnippet returns the text around the first case-insensitive match of
// q, or "" when there is none. Works on runes, lower-cased one for one, so
// the match position maps back onto the original text exactly.
func matchSnippet(text, q string) string {
	tr := lowerRunes(text)
	qr := lowerRunes(q)
	i := indexRunes(tr, qr)
	if i < 0 || len(qr) == 0 {
		return ""
	}
	orig := []rune(text)
	start, prefix := i-30, "…"
	if start <= 0 {
		start, prefix = 0, ""
	}
	end, suffix := i+len(qr)+60, "…"
	if end >= len(orig) {
		end, suffix = len(orig), ""
	}
	return prefix + strings.TrimSpace(string(orig[start:end])) + suffix
}

// containsFold reports whether q occurs in text, ignoring case.
func containsFold(text, q string) bool {
	return indexRunes(lowerRunes(text), lowerRunes(q)) >= 0
}

func lowerRunes(s string) []rune {
	r := []rune(s)
	for i, c := range r {
		r[i] = unicode.ToLower(c)
	}
	return r
}

func indexRunes(hay, needle []rune) int {
	if len(needle) == 0 {
		return 0
	}
outer:
	for i := 0; i+len(needle) <= len(hay); i++ {
		for j, c := range needle {
			if hay[i+j] != c {
				continue outer
			}
		}
		return i
	}
	return -1
}
