package app

import (
	"context"
	"log/slog"
	"time"

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

// aiKeyKeeper is the access revoker the campaigns plugin calls; it ends the
// AI keys a campaign change takes away. Trash ends every key of the
// campaign; a person's access dropping there (an ownership handover among
// them) ends the keys they made. Revoking rather than pausing means a
// restore, or ownership coming back, can't bring an old key to life. svc is
// set once the connector is wired, and is nil when its tables did not
// migrate. A failure is logged: the per-call owner check still refuses.
type aiKeyKeeper struct {
	svc *connector.Service
}

// aiKeyRevokeTimeout bounds the revoke, which runs inline in the campaign
// change that triggered it.
const aiKeyRevokeTimeout = 5 * time.Second

func (k *aiKeyKeeper) RevokeUser(campaignID, userID string) {
	if k.svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiKeyRevokeTimeout)
	defer cancel()
	if err := k.svc.EndKeysMadeBy(ctx, campaignID, userID); err != nil {
		slog.Error("ending AI keys failed", slog.String("campaign_id", campaignID), slog.String("user_id", userID), slog.Any("error", err))
	}
}

func (k *aiKeyKeeper) RevokeCampaign(campaignID string) {
	if k.svc == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiKeyRevokeTimeout)
	defer cancel()
	if err := k.svc.Disconnect(ctx, campaignID); err != nil {
		slog.Error("ending AI keys failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
}

// campaignRevokers hands one campaign access change to every revoker.
type campaignRevokers []campaigns.ConnectionRevoker

func (rs campaignRevokers) RevokeUser(campaignID, userID string) {
	for _, r := range rs {
		r.RevokeUser(campaignID, userID)
	}
}

func (rs campaignRevokers) RevokeCampaign(campaignID string) {
	for _, r := range rs {
		r.RevokeCampaign(campaignID)
	}
}
