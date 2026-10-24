// Package sanitize provides HTML sanitization for user-generated content.
// Uses bluemonday to strip dangerous HTML (script tags, event handlers,
// javascript: URLs) while preserving safe formatting and Chronicle-specific
// attributes like data-mention-id for @mention links.
package sanitize

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// noteIDPattern is the shape a note link's data-note-id must have.
var noteIDPattern = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

// The rich editor's checklist markup: which list is a checklist, and whether
// an item is ticked. Only these exact values pass; the checkbox <input> the
// editor draws is not kept, since readers style ticked items from the
// attribute and no stored HTML needs a form control.
var (
	taskTypePattern    = regexp.MustCompile(`^(taskList|taskItem)$`)
	taskCheckedPattern = regexp.MustCompile(`^(true|false)$`)
)

// policy is the singleton bluemonday policy for sanitizing user-generated HTML.
// Initialized once via sync.Once for thread-safe lazy initialization.
var (
	policy     *bluemonday.Policy
	policyOnce sync.Once
)

// getPolicy returns the shared sanitization policy, initializing it on first call.
func getPolicy() *bluemonday.Policy {
	policyOnce.Do(func() {
		policy = bluemonday.UGCPolicy()

		// Allow Chronicle-specific data attributes on anchor tags for @mentions
		// and entity preview tooltips.
		policy.AllowAttrs("data-mention-id").OnElements("a")
		policy.AllowAttrs("data-entity-preview").OnElements("a")

		// A [[link]] to a note: only an id-shaped value survives, since
		// readers resolve it to a title through the viewer's visibility.
		policy.AllowAttrs("data-note-id").Matching(noteIDPattern).OnElements("a")

		// Allow class attributes broadly — needed for TipTap/ProseMirror output
		// which uses classes for text alignment, code blocks, etc.
		policy.AllowAttrs("class").Globally()

		// Allow style attribute with a whitelist of safe CSS properties.
		// Restricts to formatting-only properties to prevent CSS-based attacks
		// (e.g., position/overlay abuse, content exfiltration via background-image).
		policy.AllowStyles("color", "background-color", "text-align",
			"font-weight", "font-style", "text-decoration",
			"font-size", "line-height", "margin", "padding",
		).OnElements("span", "p", "div", "td", "th")

		// Allow table elements for rich text tables.
		policy.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col", "caption")
		policy.AllowAttrs("colspan", "rowspan").OnElements("td", "th")

		// Allow data attributes used by the editor for various features.
		policy.AllowAttrs("data-type").OnElements("div", "span")
		policy.AllowAttrs("data-type").Matching(taskTypePattern).OnElements("ul", "li")
		policy.AllowAttrs("data-checked").Matching(taskCheckedPattern).OnElements("li")

		// Allow inline secrets (GM-only text wrapped in <span data-secret>).
		policy.AllowAttrs("data-secret").OnElements("span")

		// SECURITY NOTE: bluemonday uses an allowlist model. All attributes not
		// explicitly allowed above are stripped. This includes HTMX attributes
		// (hx-get, hx-post, hx-on:*, data-hx-*) and <meta> tags, which could
		// otherwise be used for request forgery or HTMX config override.
	})
	return policy
}

// HTML sanitizes user-generated HTML content by stripping dangerous elements
// (script, iframe, event handlers, javascript: URLs) while preserving safe
// formatting tags and Chronicle-specific attributes.
//
// This MUST be called on all user-provided HTML before storing it in the database.
// The sanitized output is safe for rendering in browsers via innerHTML or Templ's
// Raw() function.
func HTML(input string) string {
	if input == "" {
		return ""
	}
	return getPolicy().Sanitize(input)
}

// HTMLPtr sanitizes the HTML behind a nullable string pointer and returns a
// fresh pointer to the sanitized value; nil input returns nil. The original
// *p is not mutated.
//
// Defense-in-depth companion to HTML: ingress sanitization (write path) is
// the primary guarantee; calling HTMLPtr on response fields covers
// historical or otherwise-unsanitized rows on the egress path.
func HTMLPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := HTML(*p)
	return &s
}

// gmPictureRe matches a GM-only picture from the editor: a <figure> whose
// class list holds ce-img--gm. The editor writes a figure as one img plus an
// optional plain-text figcaption, never a nested figure, so the lazy match
// ends at its own closing tag.
var gmPictureRe = regexp.MustCompile(`(?s)<figure\b[^>]*\bclass="[^"]*\bce-img--gm\b[^"]*"[^>]*>.*?</figure>`)

// rollerRe matches a rolling-table roller from the editor: an empty <div>
// whose class list holds ce-roll. The roller is a DM tool, so players never
// get it; only what the DM puts into the text reaches them.
var rollerRe = regexp.MustCompile(`(?s)<div\b[^>]*\bclass="[^"]*\bce-roll\b[^"]*"[^>]*>.*?</div>`)

// gmDiagramRe matches a GM-only diagram from the editor: a <pre> whose class
// list holds ce-diagram--gm. The diagram's Mermaid source is stored as the
// <pre>'s escaped text, so a literal closing tag cannot occur inside it and
// the lazy match ends at the diagram's own </pre>.
var gmDiagramRe = regexp.MustCompile(`(?s)<pre\b[^>]*\bclass="[^"]*\bce-diagram--gm\b[^"]*"[^>]*>.*?</pre>`)

// StripSecretsHTML removes all <span data-secret>...</span> elements,
// GM-only pictures, GM-only diagrams and rolling-table rollers from HTML, used
// to hide GM-only content from players.
func StripSecretsHTML(html string) string {
	if html == "" {
		return ""
	}
	html = gmPictureRe.ReplaceAllString(html, "")
	html = gmDiagramRe.ReplaceAllString(html, "")
	html = rollerRe.ReplaceAllString(html, "")
	out, _ := stripSecretSpans(html)
	return out
}

// stripSecretSpans removes every <span data-secret> element and returns the
// remaining HTML plus how many secret spans it removed. It tracks span depth
// from each secret's opening tag because API and import paths can submit
// nested spans, and a lazy "first </span>" match would leave the tail of the
// secret in the output. An unclosed secret is removed to the end of the
// input: failing closed is safer than leaking. Untouched markup is copied
// byte for byte from the tokenizer's raw output.
func stripSecretSpans(src string) (string, int) {
	z := html.NewTokenizer(strings.NewReader(src))
	var out strings.Builder
	out.Grow(len(src))
	depth, count := 0, 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			// Raw bytes already consumed are emitted below; the error token
			// itself carries none, so stopping here keeps the input intact.
			return out.String(), count
		}
		raw := z.Raw()
		if depth > 0 {
			switch tt {
			case html.StartTagToken:
				if name, _ := z.TagName(); string(name) == "span" {
					depth++
				}
			case html.EndTagToken:
				if name, _ := z.TagName(); string(name) == "span" {
					depth--
				}
			}
			continue
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			name, hasAttr := z.TagName()
			if string(name) == "span" && hasAttr && hasSecretAttr(z) {
				count++
				if tt == html.StartTagToken {
					depth = 1
				}
				continue
			}
		}
		out.Write(raw)
	}
}

// hasSecretAttr reports whether the tokenizer's current tag carries data-secret.
func hasSecretAttr(z *html.Tokenizer) bool {
	for {
		key, _, more := z.TagAttr()
		if string(key) == "data-secret" {
			return true
		}
		if !more {
			return false
		}
	}
}

// CountSecrets reports how many GM-only parts a page's HTML holds: secret
// passages, GM-only pictures and GM-only diagrams. It uses the same patterns
// and order as StripSecretsHTML so the count matches exactly what a player
// copy would lose; rollers are a DM tool, not page content, and are not
// counted. Pictures and diagrams are removed before spans are counted so a
// part is never counted twice.
func CountSecrets(html string) (passages, pictures, diagrams int) {
	if html == "" {
		return 0, 0, 0
	}
	pictures = len(gmPictureRe.FindAllStringIndex(html, -1))
	html = gmPictureRe.ReplaceAllString(html, "")
	diagrams = len(gmDiagramRe.FindAllStringIndex(html, -1))
	html = gmDiagramRe.ReplaceAllString(html, "")
	_, passages = stripSecretSpans(html)
	return passages, pictures, diagrams
}

// StripSecretsJSON removes nodes marked with the "secret" mark, GM-only
// pictures, GM-only diagrams and rolling-table rollers from ProseMirror JSON content,
// and template prompts from the lines that carry them. Returns the modified JSON string. If the input
// is not valid ProseMirror JSON, it is returned unchanged.
func StripSecretsJSON(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &doc); err != nil {
		return jsonStr
	}

	stripSecretNodes(doc)

	out, err := json.Marshal(doc)
	if err != nil {
		return jsonStr
	}
	return string(out)
}

// stripSecretNodes recursively walks ProseMirror JSON and removes every
// node that carries a "secret" mark: text, and inline nodes such as a
// [[note]] link, which would otherwise show a player what a secret links to.
func stripSecretNodes(node map[string]interface{}) {
	content, ok := node["content"].([]interface{})
	if !ok {
		return
	}

	var filtered []interface{}
	for _, child := range content {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			filtered = append(filtered, child)
			continue
		}

		if hasSecretMark(childMap) || isGMPicture(childMap) || isGMDiagram(childMap) || childMap["type"] == "rollTable" {
			continue // strip this node
		}

		// A template's writing prompt is an editing hint, never page content.
		if attrs, ok := childMap["attrs"].(map[string]interface{}); ok {
			delete(attrs, "prompt")
		}
		// Recurse into child nodes.
		stripSecretNodes(childMap)
		filtered = append(filtered, childMap)
	}
	node["content"] = filtered
}

// isGMPicture reports whether a node is an editor picture marked GM-only.
func isGMPicture(node map[string]interface{}) bool {
	if node["type"] != "chronicleImage" {
		return false
	}
	attrs, _ := node["attrs"].(map[string]interface{})
	gm, _ := attrs["gmOnly"].(bool)
	return gm
}

// isGMDiagram reports whether a node is an editor diagram marked GM-only.
func isGMDiagram(node map[string]interface{}) bool {
	if node["type"] != "diagram" {
		return false
	}
	attrs, _ := node["attrs"].(map[string]interface{})
	gm, _ := attrs["gmOnly"].(bool)
	return gm
}

// hasSecretMark returns true if a ProseMirror node has a mark of type "secret".
func hasSecretMark(node map[string]interface{}) bool {
	marks, ok := node["marks"].([]interface{})
	if !ok {
		return false
	}
	for _, m := range marks {
		markMap, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		if markMap["type"] == "secret" {
			return true
		}
	}
	return false
}

// HasSecrets reports whether stored editor content holds anything
// StripSecretsHTML or StripSecretsJSON would remove: secret passages, GM-only
// pictures and diagrams, and rollers. A writer who is served the stripped
// copy must not save over such content, or the GM-only parts are lost.
func HasSecrets(jsonStr, htmlStr string) bool {
	if htmlStr != "" && StripSecretsHTML(htmlStr) != htmlStr {
		return true
	}
	if jsonStr == "" {
		return false
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &doc); err != nil {
		return false
	}
	return hasSecretNodes(doc)
}

// hasSecretNodes reports whether any node under node is one stripSecretNodes
// removes.
func hasSecretNodes(node map[string]interface{}) bool {
	content, _ := node["content"].([]interface{})
	for _, child := range content {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			continue
		}
		if hasSecretMark(childMap) || isGMPicture(childMap) || isGMDiagram(childMap) || childMap["type"] == "rollTable" {
			return true
		}
		if hasSecretNodes(childMap) {
			return true
		}
	}
	return false
}
