package quests

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// gate is the one place that decides whether a page exists in the campaign
// and whether a viewer may see it. A hidden page is reported exactly like a
// missing one so its existence is not confirmed.
type gate struct {
	entities EntityDirectory
}

// viewable returns which of ids exist in the campaign and are visible to v.
func (g gate) viewable(ctx context.Context, campaignID string, v Viewer, ids []string) (map[string]EntityInfo, map[string]bool, error) {
	infos, err := g.entities.Entities(ctx, campaignID, ids)
	if err != nil {
		return nil, nil, apperrorInternal(err)
	}
	ok := make(map[string]bool, len(infos))
	// Owner-level visibility (owner or co-DM) sees every page.
	if v.VisibilityRole >= permissions.RoleOwner {
		for id := range infos {
			ok[id] = true
		}
		return infos, ok, nil
	}
	existing := make([]string, 0, len(infos))
	for id := range infos {
		existing = append(existing, id)
	}
	if len(existing) == 0 {
		return infos, ok, nil
	}
	allowed, err := g.entities.FilterViewable(ctx, campaignID, existing, v.VisibilityRole, v.UserID)
	if err != nil {
		return nil, nil, apperrorInternal(err)
	}
	for id := range infos {
		ok[id] = allowed[id]
	}
	return infos, ok, nil
}

// requireViewable is the gate for a single page: NotFound unless it is in the
// campaign and visible to v.
func (g gate) requireViewable(ctx context.Context, campaignID, entityID string, v Viewer) (EntityInfo, error) {
	infos, ok, err := g.viewable(ctx, campaignID, v, []string{entityID})
	if err != nil {
		return EntityInfo{}, err
	}
	if !ok[entityID] {
		return EntityInfo{}, errNotFound("page")
	}
	return infos[entityID], nil
}
