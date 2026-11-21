// Package entities helpers provide utility functions used by entity templates.
package entities

import (
	"fmt"
	"github.com/keyxmakerx/chronicle/internal/colour"
	"net/url"
	"strconv"
)

// typeBadgeStyle paints a category badge from the category's colour: the fill
// is nudged only as far as its words need to reach 4.5:1, and the words are
// white or dark ink, whichever the fill supports.
func typeBadgeStyle(hexColor string) string {
	bg, fg := colour.BadgeColours(hexColor)
	return "background-color: " + bg + "; color: " + fg
}

// entityListParams is the All Pages query string the page links carry, so
// paging through search results keeps the search.
func entityListParams(typeID int, query, sort string) string {
	v := url.Values{}
	v.Set("type", strconv.Itoa(typeID))
	if query != "" {
		v.Set("q", query)
	}
	// A–Z is the page's own order, so only another order rides on the links.
	if sort != "" && sort != "name" {
		v.Set("sort", sort)
	}
	return v.Encode()
}

// entityKindWords is a row's category as a quiet phrase: the category, then
// the page's own descriptor when it has one ("Character · NPC").
func entityKindWords(e *Entity) string {
	if e.TypeLabel != nil && *e.TypeLabel != "" {
		return e.TypeName + " · " + *e.TypeLabel
	}
	return e.TypeName
}

// searchMatchSummary is the line under the All Pages title while a search
// filters the list.
func searchMatchSummary(total int, query string) string {
	noun := "pages match"
	if total == 1 {
		noun = "page matches"
	}
	return fmt.Sprintf("%d %s “%s”", total, noun, query)
}
