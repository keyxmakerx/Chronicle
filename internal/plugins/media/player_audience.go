package media

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PlainPlayerCanOpen reports whether any player of the file's campaign could
// open it with their own session, counting no share or grant of their own. A
// host that hands one cookieless link to every player at the table (the
// Foundry module showing a shop room) asks this first, because such a link
// opens the file for whoever holds it.
//
// It is stricter than the media server: a map's picture or a card's picture
// is refused outright, since either can turn hidden while the link still
// works, and every check that cannot run refuses.
func (h *Handler) PlainPlayerCanOpen(ctx context.Context, file *MediaFile) (bool, error) {
	if file == nil || file.CampaignID == nil || file.IsBound() {
		return false, nil
	}
	if h.mapImages == nil || h.cardPictures == nil || h.entityVisibility == nil {
		return false, fmt.Errorf("media: player audience check not fully wired")
	}
	campaignID := *file.CampaignID

	isMap, err := h.mapImages.IsMapPicture(ctx, campaignID, file.ID)
	if err != nil || isMap {
		return false, err
	}
	isCard, err := h.cardPictures.IsCardPicture(ctx, campaignID, file.ID)
	if err != nil || isCard {
		return false, err
	}

	refs, err := h.service.FindReferences(ctx, campaignID, file.ID)
	if err != nil {
		return false, fmt.Errorf("finding media references for player audience: %w", err)
	}
	// No page shows it: campaign membership decides, and every player is a member.
	if len(refs) == 0 {
		return true, nil
	}
	ids := make([]string, 0, len(refs))
	seen := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if !seen[ref.EntityID] {
			seen[ref.EntityID] = true
			ids = append(ids, ref.EntityID)
		}
	}
	// No user id: a page shared with one player only does not count.
	viewable, err := h.entityVisibility.FilterViewableEntityIDs(ctx, campaignID, ids, int(campaigns.RolePlayer), "")
	if err != nil {
		return false, fmt.Errorf("filtering pages for player audience: %w", err)
	}
	for _, ok := range viewable {
		if ok {
			return true, nil
		}
	}
	return false, nil
}
