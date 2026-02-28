// Package sanitize provides HTML sanitization for user-generated content.
// Uses bluemonday to strip dangerous HTML (script tags, event handlers,
// javascript: URLs) while preserving safe formatting and Chronicle-specific
// attributes like data-mention-id for @mention links.
package sanitize

import (
	"encoding/json"
	"regexp"
	"sync"

	"github.com/microcosm-cc/bluemonday"
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

		// Allow class attributes broadly — needed for TipTap/ProseMirror output
		// which uses classes for text alignment, code blocks, etc.
		policy.AllowAttrs("class").Globally()

		// Allow style attribute on spans for inline formatting from the editor
		// (e.g., text color, background color).
		policy.AllowAttrs("style").OnElements("span", "p", "div", "td", "th")

		// Allow table elements for rich text tables.
		policy.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col", "caption")
		policy.AllowAttrs("colspan", "rowspan").OnElements("td", "th")

		// Allow data attributes used by the editor for various features.
		policy.AllowAttrs("data-type").OnElements("div", "span")

		// Allow inline secrets (GM-only text wrapped in <span data-secret>).
		policy.AllowAttrs("data-secret").OnElements("span")
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

// secretSpanRe matches <span data-secret="true" ...>...</span> including nested tags.
// Uses a non-greedy match with a lookahead for the closing tag.
var secretSpanRe = regexp.MustCompile(`<span[^>]*\bdata-secret\b[^>]*>.*?</span>`)

// StripSecretsHTML removes all <span data-secret>...</span> elements from HTML,
// used to hide GM-only inline secrets from players.
func StripSecretsHTML(html string) string {
	if html == "" {
		return ""
	}
	return secretSpanRe.ReplaceAllString(html, "")
}

// StripSecretsJSON removes text nodes marked with the "secret" mark from
// ProseMirror JSON content. Returns the modified JSON string. If the input
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

// stripSecretNodes recursively walks ProseMirror JSON and removes text nodes
// that carry a "secret" mark.
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

		// Check if this is a text node with a "secret" mark.
		if childMap["type"] == "text" {
			if hasSecretMark(childMap) {
				continue // strip this text node
			}
		}

		// Recurse into child nodes.
		stripSecretNodes(childMap)
		filtered = append(filtered, childMap)
	}
	node["content"] = filtered
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
