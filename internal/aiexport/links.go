package aiexport

import (
	"strings"
	"unicode"
)

// slugify converts an entity / timeline / session name into a markdown
// anchor fragment. Mirrors the GitHub-style slug rules common AI
// consumers expect:
//
//   - lowercase
//   - alphanumerics + hyphens only
//   - spaces collapse to single hyphens
//   - leading/trailing hyphens trimmed
//
// Used for the wikilink target and for the `## NAME {#slug}` anchor
// on each rendered item. NOT the same as the database `slug` column
// (which is enforced uniqueness across the campaign); aiexport's slug
// is purely document-scoped and stable per-name.
func slugify(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	prevHyphen := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevHyphen = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		default:
			// Drop punctuation entirely. "The Captain's Pact" → "the-captains-pact".
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// wikilink renders a "[Name](#slug)" reference. Used for cross-entity
// links inside relations / session linked-entities / calendar event
// entity links / timeline event entity links. The fragment is
// document-local so it resolves inside the single-file export the
// owner pastes into Claude/ChatGPT.
//
// In a future v2 zip-with-INDEX mode, the resolver would map to
// `entities.md#slug` rather than `#slug`; deferred per the scoping
// report §3.5.
func wikilink(name string) string {
	if name == "" {
		return ""
	}
	return "[" + name + "](#" + slugify(name) + ")"
}
