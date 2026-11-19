package app

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
)

// dmFightAdapter fills the DM Screen's Fight tab with the encounter
// tracker, so the screen and encounters never name each other. svc is set
// once the encounters service exists, later in wiring.
type dmFightAdapter struct{ svc encounters.Service }

// FightMount returns the tab's content for campaignID.
func (*dmFightAdapter) FightMount(campaignID string, conditions []string) templ.Component {
	return encounters.RunMount(campaignID, conditions)
}

// ReadyCount counts the fights marked Ready, which is what the Fight line
// says on a phone. The service applies the DM rule itself.
func (a *dmFightAdapter) ReadyCount(ctx context.Context, campaignID string, v dmscreen.Viewer) (int, error) {
	if a.svc == nil {
		return 0, apperror.NewNotFound("encounters are not available")
	}
	items, _, err := a.svc.Search(ctx, campaignID, "", encounters.MaxSearchLimit,
		encounters.Viewer{UserID: v.UserID, VisibilityRole: permissions.RoleOwner})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, it := range items {
		if it.Ready {
			n++
		}
	}
	return n, nil
}
