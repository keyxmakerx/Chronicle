package campaigns

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// OwnedCampaign is a campaign that stops its owner deleting their account
// until it is handed over or deleted. Archived campaigns count too.
type OwnedCampaign struct {
	ID          string
	Name        string
	MemberCount int
}

// OwnedCampaigns lists every campaign the user created or holds the owner
// role in, archived ones included.
func (s *campaignService) OwnedCampaigns(ctx context.Context, userID string) ([]OwnedCampaign, error) {
	out, err := s.repo.ListOwnedByUser(ctx, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing owned campaigns: %w", err))
	}
	return out, nil
}

// LeaveAllForDeletedAccount takes a departing account out of every campaign
// through RemoveMember, so DM grants, live connections and the member-removed
// hooks are cleaned up exactly as when an owner removes someone. Pending
// ownership hand-overs to or from the account are cancelled. It refuses while
// the account still owns a campaign: every campaign needs an owner.
func (s *campaignService) LeaveAllForDeletedAccount(ctx context.Context, userID string) error {
	owned, err := s.OwnedCampaigns(ctx, userID)
	if err != nil {
		return err
	}
	if len(owned) > 0 {
		return apperror.NewConflict("hand over or delete the campaigns you own first")
	}
	if err := s.repo.DeleteTransfersInvolving(ctx, userID); err != nil {
		return apperror.NewInternal(fmt.Errorf("cancelling hand-overs: %w", err))
	}
	ids, err := s.repo.ListMemberCampaignIDs(ctx, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("listing memberships: %w", err))
	}
	for _, id := range ids {
		if err := s.RemoveMember(ctx, id, userID); err != nil {
			return err
		}
	}
	return nil
}
