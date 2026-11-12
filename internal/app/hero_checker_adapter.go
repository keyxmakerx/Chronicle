package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// heroCheckerAdapter answers sessions.HeroChecker from the entities service:
// whether players claim heroes in a campaign, and whether one member has.
type heroCheckerAdapter struct {
	svc    entities.EntityService
	addons interface {
		IsEnabledForCampaign(ctx context.Context, campaignID, slug string) (bool, error)
	}
}

func (a *heroCheckerAdapter) PlayingHero(ctx context.Context, campaignID, userID string) (tracked, has bool, err error) {
	on, err := a.addons.IsEnabledForCampaign(ctx, campaignID, entities.AddonPlayerCharacterClaiming)
	if err != nil || !on {
		return false, false, err
	}
	// Read as the owner so a hero the member plays is found whatever its
	// visibility; only the member's own are counted.
	claimed, err := a.svc.ListClaimed(ctx, campaignID, int(campaigns.RoleOwner), userID)
	if err != nil {
		return true, false, err
	}
	for _, e := range claimed {
		if e.OwnerUserID != nil && *e.OwnerUserID == userID {
			return true, true, nil
		}
	}
	return true, false, nil
}
