package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A person the owner removed is remembered on the campaign, so the shared
// join link cannot let them straight back in. They return only when the
// owner allows it, or invites them again.

// setRemoved adds or drops userID on the campaign's removed list.
func (s *campaignService) setRemoved(ctx context.Context, campaignID, userID string, removed bool) error {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}
	settings := campaign.ParseSettings()
	if slices.Contains(settings.RemovedMemberIDs, userID) == removed {
		return nil
	}
	if removed {
		settings.RemovedMemberIDs = append(settings.RemovedMemberIDs, userID)
	} else {
		settings.RemovedMemberIDs = slices.DeleteFunc(settings.RemovedMemberIDs, func(id string) bool { return id == userID })
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	if err := s.saveSettingsChanges(ctx, campaign, campaignID, settingsJSON); err != nil {
		return apperror.NewInternal(fmt.Errorf("saving removed members: %w", err))
	}
	return nil
}

// ListRemovedMembers returns the people the owner removed who have not come
// back. Someone invited again since, or whose account is gone, is left out.
func (s *campaignService) ListRemovedMembers(ctx context.Context, campaignID string) ([]MemberUser, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if campaign == nil {
		return nil, apperror.NewNotFound("campaign not found")
	}
	var out []MemberUser
	for _, id := range campaign.ParseSettings().RemovedMemberIDs {
		if _, err := s.repo.FindMember(ctx, campaignID, id); err == nil {
			continue
		}
		user, err := s.users.FindUserByID(ctx, id)
		if err != nil || user == nil {
			continue
		}
		out = append(out, *user)
	}
	return out, nil
}

// AllowRemovedMemberBack lets a removed person use the join link again.
func (s *campaignService) AllowRemovedMemberBack(ctx context.Context, campaignID, userID string) error {
	return s.setRemoved(ctx, campaignID, userID, false)
}
