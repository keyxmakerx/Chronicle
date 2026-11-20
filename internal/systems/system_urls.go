package systems

import "github.com/keyxmakerx/chronicle/internal/plugins/campaigns"

// systemIndexURL is the system's own page: its book when it ships one, else
// the category browser.
func systemIndexURL(cc *campaigns.CampaignContext, manifest *SystemManifest) string {
	return "/campaigns/" + cc.Campaign.ID + "/systems/" + manifest.ID
}
