package admin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/settings"
)

// trashHeaders is the hx-headers value that carries the CSRF token on every
// request the Trash region makes. Marshalled, not spliced, so the token is
// always valid JSON.
func trashHeaders(csrf string) string {
	b, _ := json.Marshal(map[string]string{"X-CSRF-Token": csrf})
	return string(b)
}

// trashRetention is the retention to show as chosen.
func trashRetention(d TrashPageData) int {
	if d.Overview == nil {
		return settings.DefaultSiteTrashRetentionDays
	}
	return d.Overview.RetentionDays
}

// trashIcon picks the row icon; the icon repeats what the title says, so it
// is decoration only.
func trashIcon(e TrashEntry) string {
	if e.Kind == TrashCampaign {
		return "fa-book-open"
	}
	return "fa-images"
}

// trashMeta joins the parts of the second line, skipping any that are empty.
func trashMeta(e TrashEntry) string {
	var parts []string
	for _, p := range []string{e.Who, e.When, e.Empties} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// trashUndoURL is the Undo endpoint for a row. Ids are generated uuids; they
// are still path-escaped by templ when written to the attribute.
func trashUndoURL(e TrashEntry) string {
	if e.Kind == TrashCampaign {
		return "/admin/trash/campaigns/" + e.ID + "/undo"
	}
	return "/admin/trash/batches/" + e.ID + "/undo"
}

// longTermLabel names a Long-term trash choice, with its rough size once
// the site has packed anything to go by.
func longTermLabel(months int, v LongTermView) string {
	if months == 0 {
		return "Off"
	}
	label := fmt.Sprintf("%d months", months)
	switch months {
	case 12:
		label = "1 year"
	case 24:
		label = "2 years"
	}
	if v.Known {
		label += " (about " + formatBytes(v.MonthlyBytes*int64(months)) + ")"
	}
	return label
}
