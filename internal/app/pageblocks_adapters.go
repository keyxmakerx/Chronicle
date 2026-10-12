package app

// pageblocks_adapters.go bridges the pageblocks plugin to the entities
// service. The plugin owns the narrow interface; the adapter lives here so it
// never imports another plugin.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/pageblocks"
)

// pageBlocksEntityAdapter implements pageblocks.EntityDirectory.
type pageBlocksEntityAdapter struct {
	svc   entities.EntityService
	cards entities.PageCards
}

// Access applies the rules the page itself uses: CheckEntityAccess for what
// the viewer may open, and the Scribe-or-above requirement the page adds
// before it shows edit controls. A page of another campaign is reported as
// absent, so a foreign id can never be resolved through this plugin.
func (a *pageBlocksEntityAdapter) Access(ctx context.Context, campaignID, entityID string, v pageblocks.Viewer) (pageblocks.PageAccess, error) {
	cards, err := a.cards.InCampaign(ctx, campaignID, []string{entityID})
	if err != nil {
		return pageblocks.PageAccess{}, err
	}
	if _, ok := cards[entityID]; !ok {
		return pageblocks.PageAccess{}, nil
	}
	ep, err := a.svc.CheckEntityAccess(ctx, entityID, v.VisibilityRole, v.UserID)
	if err != nil {
		return pageblocks.PageAccess{}, err
	}
	return pageblocks.PageAccess{
		Exists:  true,
		CanView: ep.CanView,
		CanEdit: ep.CanView && ep.CanEdit && v.MemberRole >= int(campaigns.RoleScribe),
	}, nil
}
