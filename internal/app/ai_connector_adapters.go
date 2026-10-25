package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// aiConnectorOwners tells the AI connector whether a key's maker still owns
// its campaign. A campaign in the site Trash reads as not found, and so does
// its membership, so a trashed campaign's key stops working too. Not found is
// an answer here, not a fault to log.
type aiConnectorOwners struct {
	campaigns campaigns.CampaignService
}

func (a aiConnectorOwners) Check(ctx context.Context, campaignID, userID string) (string, error) {
	c, err := a.campaigns.GetByID(ctx, campaignID)
	if isNotFound(err) || (err == nil && c == nil) {
		return "", connector.ErrNotOwner
	}
	if err != nil {
		return "", err
	}
	m, err := a.campaigns.GetMember(ctx, campaignID, userID)
	if isNotFound(err) || (err == nil && m == nil) {
		return "", connector.ErrNotOwner
	}
	if err != nil {
		return "", err
	}
	if m.Role < campaigns.RoleOwner {
		return "", connector.ErrNotOwner
	}
	return c.Name, nil
}
