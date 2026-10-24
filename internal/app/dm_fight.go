package app

import (
	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
)

// dmFightAdapter fills the DM Screen's Fight tab with the encounter
// tracker, so the screen and encounters never name each other.
type dmFightAdapter struct{}

// FightMount returns the tab's content for campaignID.
func (dmFightAdapter) FightMount(campaignID string, conditions []string) templ.Component {
	return encounters.RunMount(campaignID, conditions)
}
