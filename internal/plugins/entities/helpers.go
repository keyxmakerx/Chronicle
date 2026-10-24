// Package entities helpers provide utility functions used by entity templates.
package entities

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// contrastTextColor returns a text color (CSS hex) that contrasts well with
// the given hex background color. Returns dark text for light backgrounds
// and white text for dark backgrounds, using the ITU-R BT.709 luminance formula.
func contrastTextColor(hexColor string) string {
	hexColor = strings.TrimPrefix(hexColor, "#")
	if len(hexColor) == 3 {
		hexColor = string(hexColor[0]) + string(hexColor[0]) +
			string(hexColor[1]) + string(hexColor[1]) +
			string(hexColor[2]) + string(hexColor[2])
	}
	if len(hexColor) != 6 {
		return "#ffffff"
	}

	r, _ := strconv.ParseInt(hexColor[0:2], 16, 64)
	g, _ := strconv.ParseInt(hexColor[2:4], 16, 64)
	b, _ := strconv.ParseInt(hexColor[4:6], 16, 64)

	// Perceived brightness (ITU-R BT.709).
	luminance := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if luminance > 186 {
		return "#1f2937" // dark text for light backgrounds
	}
	return "#ffffff"
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
