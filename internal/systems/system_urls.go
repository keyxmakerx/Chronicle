package systems

import "github.com/keyxmakerx/chronicle/internal/plugins/campaigns"

// systemIndexURL is the system's own page: its book when it ships one, else
// the category browser.
func systemIndexURL(cc *campaigns.CampaignContext, manifest *SystemManifest) string {
	return "/campaigns/" + cc.Campaign.ID + "/systems/" + manifest.ID
}

// systemCrumbURL is where a reference page's system crumb goes. A system with
// a book answers its index with the whole book, so the crumb goes to the
// category browser, which is one level up from a category or an item.
func systemCrumbURL(cc *campaigns.CampaignContext, manifest *SystemManifest, hasBook bool) string {
	if hasBook {
		return systemIndexURL(cc, manifest) + "/browse"
	}
	return systemIndexURL(cc, manifest)
}
