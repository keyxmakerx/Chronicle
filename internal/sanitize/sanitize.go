// Package sanitize provides HTML sanitization for user-generated content.
// Uses bluemonday to strip dangerous HTML (script tags, event handlers,
// javascript: URLs) while preserving safe formatting and Chronicle-specific
// attributes like data-mention-id for @mention links.
package sanitize

import (
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
