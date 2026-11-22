package app

import (
	"context"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/addons"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
)

// dmDecksAdapter fills the DM Screen's Card decks section with the decks
// plugin's own fragment, so the screen and decks never name each other.
type dmDecksAdapter struct {
	svc    decks.Service
	addons addons.AddonService
}

// DecksSection reads the section for a Director. A campaign with the add-on off
// has no section; the decks service itself refuses anyone who is not a Director.
func (a *dmDecksAdapter) DecksSection(ctx context.Context, campaignID string, v dmscreen.Viewer) (templ.Component, string, bool, error) {
	on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, decks.AddonSlug)
	if err != nil || !on {
		return nil, "", false, err
	}
	view, err := a.svc.Screen(ctx, campaignID, decks.Viewer{
		UserID: v.UserID, MemberRole: v.Role, VisibilityRole: v.Role, Director: v.IsOwner(),
	})
	if err != nil {
		return nil, "", false, err
	}
	return decks.ScreenComponent(view), view.Summary(), true, nil
}
