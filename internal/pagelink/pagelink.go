// Package pagelink builds the in-Chronicle page link the editor writes for an
// @mention, so importers that produce text rather than editor input make the
// same anchor: readers get the hover card and click-to-preview from the
// attributes alone. It is core, not a plugin, because several import plugins
// need it and plugins may not import each other.
package pagelink

import (
	"html"
	"regexp"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// UUIDPattern is the shape of every page id.
const UUIDPattern = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

// Href is the address of a page inside its campaign, the href the editor
// writes for a mention.
func Href(campaignID, pageID string) string {
	return "/campaigns/" + campaignID + "/entities/" + pageID
}

// Anchor is the mention anchor for a page: the id (backlinks and page-link
// readers key on it), the address, and the hover-card address. text is
// escaped here, so callers pass it as plain text.
func Anchor(campaignID, pageID, text string) string {
	href := html.EscapeString(Href(campaignID, pageID))
	return `<a data-mention-id="` + html.EscapeString(pageID) + `" href="` + href +
		`" data-entity-preview="` + href + `/preview">` + html.EscapeString(text) + `</a>`
}

// RewriteLinks turns each plain <a> that points at one of this campaign's
// pages into the mention anchor, keeping its text. Callers that render
// Markdown links to Href use this to finish them; the result is sanitised
// again before it is returned.
func RewriteLinks(htmlIn, campaignID string) string {
	re := regexp.MustCompile(`<a\b[^>]*?\bhref="(/campaigns/` + regexp.QuoteMeta(campaignID) + `/entities/(` + UUIDPattern + `))"[^>]*?>`)
	out := re.ReplaceAllString(htmlIn, `<a data-mention-id="$2" href="$1" data-entity-preview="$1/preview">`)
	return sanitize.HTML(out)
}
