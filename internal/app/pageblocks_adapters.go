package app

// pageblocks_adapters.go bridges the pageblocks plugin to the entities
// service. The plugin owns the narrow interface; the adapter lives here so it
// never imports another plugin.

import (
	"context"
	"errors"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/mapsketch"
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

// FilterViewable keeps the pages among ids that the viewer may open, over the
// entities service's own batched visibility filter, and names them from the
// campaign-scoped card reader. A foreign id is dropped by both.
func (a *pageBlocksEntityAdapter) FilterViewable(ctx context.Context, campaignID string, ids []string, v pageblocks.Viewer) (map[string]pageblocks.PageLink, error) {
	out := map[string]pageblocks.PageLink{}
	if len(ids) == 0 {
		return out, nil
	}
	seen, err := a.svc.FilterViewableEntityIDs(ctx, campaignID, ids, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, err
	}
	var visible []string
	for _, id := range ids {
		if seen[id] {
			visible = append(visible, id)
		}
	}
	if len(visible) == 0 {
		return out, nil
	}
	cards, err := a.cards.InCampaign(ctx, campaignID, visible)
	if err != nil {
		return nil, err
	}
	for _, id := range visible {
		if c, ok := cards[id]; ok {
			out[id] = pageblocks.PageLink{ID: id, Name: c.Name, URL: "/campaigns/" + campaignID + "/entities/" + id}
		}
	}
	return out, nil
}

// Live asks the campaign-scoped card reader, which returns only live pages.
func (a *pageBlocksEntityAdapter) Live(ctx context.Context, campaignID string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	cards, err := a.cards.InCampaign(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	for id := range cards {
		out[id] = true
	}
	return out, nil
}

// FindByName looks a page up by the slug its name makes, the way AI Import
// resolves a `page:` name, keeps it only when its name is the one written (a
// slug can stand for many names, and every name with no Latin letters makes
// the same one), then asks the visibility filter, so a page the viewer cannot
// open reads exactly like one that does not exist.
func (a *pageBlocksEntityAdapter) FindByName(ctx context.Context, campaignID, name string, v pageblocks.Viewer) (pageblocks.PageLink, bool, error) {
	e, err := a.svc.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return pageblocks.PageLink{}, false, nil
		}
		return pageblocks.PageLink{}, false, err
	}
	if e == nil || e.CampaignID != campaignID || mapsketch.NameKey(e.Name) != mapsketch.NameKey(name) {
		return pageblocks.PageLink{}, false, nil
	}
	links, err := a.FilterViewable(ctx, campaignID, []string{e.ID}, v)
	if err != nil {
		return pageblocks.PageLink{}, false, err
	}
	l, ok := links[e.ID]
	return l, ok, nil
}

// FirstPageMapBlock reads the layout of the page's category and returns its
// first page_map block, whether it sits in a column or in a tile section. It
// does not ask who is looking: callers have already checked access, and the
// answer is only a block id.
func (a *pageBlocksEntityAdapter) FirstPageMapBlock(ctx context.Context, campaignID, entityID string) (string, bool, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	if e == nil || e.CampaignID != campaignID {
		return "", false, nil
	}
	et, err := a.svc.GetEntityTypeByID(ctx, e.EntityTypeID)
	if err != nil {
		return "", false, err
	}
	for _, row := range et.Layout.Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				if b.Type == pageblocks.KindPageMap && b.ID != "" {
					return b.ID, true, nil
				}
			}
		}
		for _, t := range row.Tiles {
			if t.Type == pageblocks.KindPageMap && t.ID != "" {
				return t.ID, true, nil
			}
		}
	}
	return "", false, nil
}
