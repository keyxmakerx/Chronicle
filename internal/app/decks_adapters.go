package app

// decks_adapters.go bridges card decks to the entities and campaigns
// services. The plugin owns the narrow interfaces; the adapters live here so
// neither side imports the other.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/decks"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// decksHeroAdapter implements decks.Heroes over the entities service.
type decksHeroAdapter struct {
	svc entities.EntityService
}

func (a *decksHeroAdapter) Owned(ctx context.Context, campaignID, userID string) ([]decks.Hero, error) {
	if userID == "" {
		return nil, nil
	}
	list, err := a.svc.ListByOwner(ctx, campaignID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]decks.Hero, 0, len(list))
	for _, e := range list {
		out = append(out, decks.Hero{ID: e.ID, Name: e.Name, OwnerUserID: userID})
	}
	return out, nil
}

// InCampaign finds a live page of this campaign. A page of another campaign
// is reported as absent, so a foreign id never resolves through this plugin.
func (a *decksHeroAdapter) InCampaign(ctx context.Context, campaignID, entityID string) (decks.Hero, bool, error) {
	e, err := a.svc.GetByID(ctx, entityID)
	if err != nil {
		if isNotFound(err) {
			return decks.Hero{}, false, nil
		}
		return decks.Hero{}, false, err
	}
	if e == nil || e.CampaignID != campaignID {
		return decks.Hero{}, false, nil
	}
	h := decks.Hero{ID: e.ID, Name: e.Name}
	if e.OwnerUserID != nil {
		h.OwnerUserID = *e.OwnerUserID
	}
	return h, true, nil
}

// Viewable applies the page's own visibility rule to the viewer.
func (a *decksHeroAdapter) Viewable(ctx context.Context, campaignID, entityID string, v decks.Viewer) (bool, error) {
	if _, ok, err := a.InCampaign(ctx, campaignID, entityID); err != nil || !ok {
		return false, err
	}
	ep, err := a.svc.CheckEntityAccess(ctx, entityID, v.VisibilityRole, v.UserID)
	if err != nil {
		return false, err
	}
	return ep.CanView, nil
}

// decksMemberAdapter implements decks.Members over the campaigns service.
type decksMemberAdapter struct {
	svc campaigns.CampaignService
}

// Members lists the campaign's people. A Director is the owner or anyone
// holding the campaign's DM-only grant, the same rule the request context
// uses.
func (a *decksMemberAdapter) Members(ctx context.Context, campaignID string) ([]decks.Member, error) {
	c, err := a.svc.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	list, err := a.svc.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	grants := map[string]bool{}
	for _, id := range c.ParseSettings().DmGrantIDs {
		grants[id] = true
	}
	out := make([]decks.Member, 0, len(list))
	for _, m := range list {
		out = append(out, decks.Member{UserID: m.UserID, Role: int(m.Role),
			Director: m.Role >= campaigns.RoleOwner || grants[m.UserID], JoinedAt: m.JoinedAt})
	}
	return out, nil
}
