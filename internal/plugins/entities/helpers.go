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
func entityListParams(typeID int, query string) string {
	v := url.Values{}
	v.Set("type", strconv.Itoa(typeID))
	if query != "" {
		v.Set("q", query)
	}
	return v.Encode()
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
