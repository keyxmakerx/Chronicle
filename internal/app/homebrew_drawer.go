package app

import (
	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// dmHomebrewAdapter fills the DM Screen's Homebrew drawer. It is the one
// place that knows both plugins' addresses, so neither plugin names the
// other: the screen gets a component, homebrew gets the trackers API.
type dmHomebrewAdapter struct {
	// trackers is false while the trackers plugin is degraded; the drawer
	// then has its rule sections and no pages.
	trackers bool
}

// DrawerMount returns the drawer's content for campaignID.
func (a *dmHomebrewAdapter) DrawerMount(campaignID string) templ.Component {
	if !a.trackers {
		return homebrew.DrawerMount(campaignID, "", "")
	}
	return homebrew.DrawerMount(campaignID, "/campaigns/"+campaignID+"/trackers",
		layouts.AssetURL("/static/plugins/trackers/css/trackers.css"))
}
