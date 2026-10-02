package npcs

import (
	"fmt"
	"net/url"
)

// sectionURL builds the NPC section fragment URL for a page, carrying the
// current filters so "Show more" continues the same search rather than the
// unfiltered list.
func sectionURL(campaignID, search, tag string, page int) string {
	q := url.Values{}
	if search != "" {
		q.Set("q", search)
	}
	if tag != "" {
		q.Set("tag", tag)
	}
	q.Set("page", fmt.Sprint(page))
	return fmt.Sprintf("/campaigns/%s/npcs/section?%s", url.PathEscape(campaignID), q.Encode())
}
