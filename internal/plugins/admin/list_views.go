// list_views.go turns repository counts into the chip rows and view models of
// the people and campaign lists. Pure functions, so the handlers stay a bind
// / call / render and the chip logic is testable without Echo or a database.

package admin

import (
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// userListView builds the people-list view model. page is the already
// clamped page; filter the already validated chip.
func userListView(lq listQuery, filter auth.UserFilter, counts auth.UserFilterCounts, page int) listView {
	chips := []listChip{
		{Label: "All", Value: "", Count: counts.All},
		{Label: "Admins", Value: string(auth.UserFilterAdmins), Count: counts.Admins},
		{Label: "Disabled", Value: string(auth.UserFilterDisabled), Count: counts.Disabled},
	}
	for i := range chips {
		chips[i].Active = chips[i].Value == string(filter)
	}
	return listView{
		BaseURL:     "/admin/users",
		RegionID:    "users-list",
		Placeholder: "Search by name or email",
		Noun:        "people",
		Query:       lq.Q,
		Filter:      string(filter),
		Chips:       chips,
		Total:       counts.For(filter),
		Page:        page,
		PerPage:     adminListPerPage,
	}
}

// systemChipLabel names a game-system chip. System ids are shown as stored;
// the two synthetic buckets get readable names.
func systemChipLabel(key string) string {
	switch key {
	case campaigns.SystemFilterNone:
		return "No system"
	case campaigns.SystemFilterCustom:
		return "Custom"
	default:
		return key
	}
}

// resolveSystemFilter returns the requested chip if it names a system that
// actually has campaigns for this search, otherwise "" (all). Validating
// against the counts means an unknown or stale ?f= can never reach the SQL
// as a filter that silently matches nothing.
func resolveSystemFilter(raw string, counts []campaigns.SystemCount) string {
	for _, sc := range counts {
		if sc.Key == raw {
			return raw
		}
	}
	return ""
}

// campaignListView builds the campaign-list view model. The "All" chip
// always comes first; system chips follow in the repository's order
// (most campaigns first).
func campaignListView(lq listQuery, filter string, counts []campaigns.SystemCount, page int) listView {
	all := 0
	chips := make([]listChip, 0, len(counts)+1)
	chips = append(chips, listChip{Label: "All", Value: ""})
	total := 0
	for _, sc := range counts {
		all += sc.Count
		if sc.Key == filter {
			total = sc.Count
		}
		chips = append(chips, listChip{Label: systemChipLabel(sc.Key), Value: sc.Key, Count: sc.Count})
	}
	chips[0].Count = all
	if filter == "" {
		total = all
	}
	for i := range chips {
		chips[i].Active = chips[i].Value == filter
	}
	// A single system adds nothing over All; drop the chips so the toolbar
	// does not show two identical choices.
	if len(chips) == 2 {
		chips = chips[:1]
	}
	return listView{
		BaseURL:     "/admin/campaigns",
		RegionID:    "campaigns-list",
		Placeholder: "Search by campaign name",
		Noun:        "campaigns",
		Query:       lq.Q,
		Filter:      filter,
		Chips:       chips,
		Total:       total,
		Page:        page,
		PerPage:     adminListPerPage,
	}
}
