package app

// rumours_adapters.go bridges the rumours plugin to the entities service. The
// plugin owns the narrow interface; the adapter lives here so it never imports
// another plugin.

import (
	"context"
	"log/slog"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/rumours"
)

// rumoursPagesAdapter implements rumours.Pages over the entities service.
type rumoursPagesAdapter struct {
	svc entities.EntityService
}

// Open applies the rules the page itself uses: CheckEntityAccess for what the
// viewer may open. A page that is missing, of another campaign or hidden from
// the viewer is the same answer, so a foreign or hidden id learns nothing.
func (a *rumoursPagesAdapter) Open(ctx context.Context, campaignID, entityID string, role int, userID string) error {
	entity, err := a.svc.GetByID(ctx, entityID)
	if err != nil || entity == nil || entity.CampaignID != campaignID {
		return apperror.NewNotFound("page not found")
	}
	access, err := a.svc.CheckEntityAccess(ctx, entityID, role, userID)
	if err != nil || access == nil || !access.CanView {
		return apperror.NewNotFound("page not found")
	}
	return nil
}

// registerRumoursBlock registers the "Rumours" block: what the party has heard
// about a page, with the Director's truth and the party's belief joined at the
// foot of each. It has no settings in the layout and is one per page, because
// its list is a single sheet. What a viewer is told is decided by the rumours
// service for that viewer; the block adds no rule of its own, and a block the
// service hides for the viewer draws nothing.
func registerRumoursBlock(blockRegistry *entities.BlockRegistry, svc rumours.Service) {
	blockRegistry.Register(entities.BlockMeta{
		Type: rumours.BlockType, Label: "Rumours", Icon: "fa-ear-listen",
		Description: "What the party has heard about this page, and whether it is true",
		Contexts:    []string{"template"},
		Singleton:   true,
	}, func(rc entities.BlockRenderContext) templ.Component {
		if rc.CC == nil || rc.CC.Campaign == nil || rc.Entity == nil {
			return templ.NopComponent
		}
		view, err := svc.View(context.Background(), rc.CC.Campaign.ID, rc.Entity.ID, rumours.ViewerOf(rc.CC, rc.UserID))
		if err != nil {
			slog.Error("rumours: cannot read the page's rumours",
				slog.String("entity_id", rc.Entity.ID), slog.Any("error", err))
			return templ.NopComponent
		}
		if view.Hidden {
			return templ.NopComponent
		}
		return rumours.BlockRumours(rc.CC.Campaign.ID, rc.Entity.ID, view)
	})
}
