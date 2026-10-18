package importer

import (
	"errors"
	"strings"

	htmltok "golang.org/x/net/html"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SecretOpen and SecretClose bracket a GM-only passage in a page body:
// `{gm{the cult is led by the mayor}gm}`. The editor's secret is an inline
// mark on text, so one marker pair covers one paragraph; a longer secret
// repeats the pair in each paragraph. The shape was picked because neither
// half is markdown syntax, so the converter leaves it intact, and prose does
// not contain it by accident; the export neutralizes a literal occurrence so
// only a deliberate marker ever round-trips.
const (
	SecretOpen  = "{gm{"
	SecretClose = "}gm}"
)

// ErrSecretNotAllowed means a page body carries GM-only markers but the
// importing user may not write GM-only text.
var ErrSecretNotAllowed = errors.New("importer: GM-only passages need the campaign owner or a co-DM")

// secretWriter reports whether a viewer may author GM-only text. The role is
// the visibility role, which already folds a co-DM grant into the owner tier.
func secretWriter(v Viewer) bool { return v.Role >= int(campaigns.RoleOwner) }

// hasSecretMarker is a cheap pre-check on markdown or HTML text.
func hasSecretMarker(s string) bool { return strings.Contains(s, SecretOpen) }

// blockTags end any open GM-only passage: the editor's secret is inline, so a
// passage can never straddle a block boundary. An unclosed marker therefore
// hides the rest of its block and no more (failing closed).
var blockTags = map[string]bool{
	"p": true, "li": true, "ul": true, "ol": true, "blockquote": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"td": true, "th": true, "tr": true, "table": true, "thead": true,
	"tbody": true, "pre": true, "hr": true, "div": true, "figure": true,
}

var voidTags = map[string]bool{"br": true, "img": true, "hr": true, "wbr": true}

const secretSpanOpen = `<span data-secret="true">`

type openTag struct{ name, raw string }

// convertSecretMarkers turns marker pairs in rendered HTML text into the
// editor's secret spans. It runs on the HTML, after markdown, so code samples
// keep their text, and before sanitize, which vets the result like any other
// markup. Inline tags opened outside a passage and closed inside it (or the
// reverse) are closed and reopened around the span so the passage always
// covers exactly the text between its markers, even when the markdown nested
// emphasis across them; the HTML parser would otherwise drop the tail of the
// passage out of the span and publish it.
func convertSecretMarkers(htmlIn string) string {
	if !hasSecretMarker(htmlIn) {
		return htmlIn
	}
	var (
		out       strings.Builder
		stack     []openTag
		depth     int // marker nesting; only the outermost makes a span
		openDepth int // len(stack) when the span opened
		skip      int // depth inside <pre> or <code>
	)
	closeSpan := func() {
		for i := len(stack) - 1; i >= openDepth; i-- {
			out.WriteString("</" + stack[i].name + ">")
		}
		out.WriteString("</span>")
		for i := openDepth; i < len(stack); i++ {
			out.WriteString(stack[i].raw)
		}
	}
	z := htmltok.NewTokenizer(strings.NewReader(htmlIn))
	for {
		tt := z.Next()
		if tt == htmltok.ErrorToken {
			if depth > 0 {
				closeSpan()
			}
			return out.String()
		}
		raw := string(z.Raw())
		switch tt {
		case htmltok.StartTagToken, htmltok.SelfClosingTagToken:
			nameB, _ := z.TagName()
			name := string(nameB)
			if blockTags[name] {
				if depth > 0 {
					closeSpan()
					depth = 0
				}
				stack = stack[:0]
			} else if tt == htmltok.StartTagToken && !voidTags[name] {
				stack = append(stack, openTag{name, raw})
			}
			if name == "pre" || name == "code" {
				if tt == htmltok.StartTagToken {
					skip++
				}
			}
			out.WriteString(raw)
		case htmltok.EndTagToken:
			nameB, _ := z.TagName()
			name := string(nameB)
			if name == "pre" || name == "code" {
				if skip > 0 {
					skip--
				}
			}
			if blockTags[name] {
				if depth > 0 {
					closeSpan()
					depth = 0
				}
				stack = stack[:0]
				out.WriteString(raw)
				continue
			}
			idx := -1
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == name {
					idx = i
					break
				}
			}
			if idx >= 0 && depth > 0 && idx < openDepth {
				// Closing a tag that was opened before the passage: end the
				// passage first, then reopen it inside the surviving tags.
				closeSpan()
				out.WriteString(raw)
				stack = append(stack[:idx], stack[idx+1:]...)
				openDepth = len(stack)
				out.WriteString(secretSpanOpen)
				continue
			}
			if idx >= 0 {
				stack = append(stack[:idx], stack[idx+1:]...)
			}
			out.WriteString(raw)
		case htmltok.TextToken:
			if skip > 0 || !hasSecretMarker(raw) && !strings.Contains(raw, SecretClose) {
				out.WriteString(raw)
				continue
			}
			rest := raw
			for rest != "" {
				o := strings.Index(rest, SecretOpen)
				c := strings.Index(rest, SecretClose)
				switch {
				case o >= 0 && (c < 0 || o < c):
					out.WriteString(rest[:o])
					rest = rest[o+len(SecretOpen):]
					if depth == 0 {
						out.WriteString(secretSpanOpen)
						openDepth = len(stack)
					}
					depth++
				case c >= 0 && depth > 0:
					out.WriteString(rest[:c])
					rest = rest[c+len(SecretClose):]
					depth--
					if depth == 0 {
						closeSpan()
					}
				case c >= 0:
					// A close with nothing open is ordinary text.
					out.WriteString(rest[:c+len(SecretClose)])
					rest = rest[c+len(SecretClose):]
				default:
					out.WriteString(rest)
					rest = ""
				}
			}
		default:
			out.WriteString(raw)
		}
	}
}

// secretPassages returns the plain text of each outermost GM-only span in
// HTML, whitespace-collapsed, empty ones dropped.
func secretPassages(src string) []string {
	var (
		res   []string
		cur   strings.Builder
		depth int
	)
	z := htmltok.NewTokenizer(strings.NewReader(src))
	for {
		switch z.Next() {
		case htmltok.ErrorToken:
			if depth > 0 {
				res = append(res, cur.String())
			}
			return normalizePassages(res)
		case htmltok.StartTagToken:
			name, hasAttr := z.TagName()
			if string(name) != "span" {
				continue
			}
			if depth > 0 {
				depth++
			} else if hasAttr && secretAttr(z) {
				depth = 1
				cur.Reset()
			}
		case htmltok.EndTagToken:
			if name, _ := z.TagName(); depth > 0 && string(name) == "span" {
				depth--
				if depth == 0 {
					res = append(res, cur.String())
				}
			}
		case htmltok.TextToken:
			if depth > 0 {
				cur.Write(z.Text())
			}
		}
	}
}

func secretAttr(z *htmltok.Tokenizer) bool {
	for {
		k, _, more := z.TagAttr()
		if string(k) == "data-secret" {
			return true
		}
		if !more {
			return false
		}
	}
}

func normalizePassages(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.Join(strings.Fields(s), " "); s != "" {
			out = append(out, s)
		}
	}
	return out
}
