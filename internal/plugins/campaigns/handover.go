package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// HandoverChoices are the "If you're away" settings an owner can pick, in
// months. 0 is Never, the default: an admin can't hand the campaign over.
var HandoverChoices = []int{0, 3, 6, 12}

// OwnerPresence is who owns a campaign and when they last used the site.
type OwnerPresence struct {
	OwnerID   string
	OwnerName string
	LastSeen  time.Time
}

// HandoverStatus is what Admin › Campaigns shows about one campaign.
type HandoverStatus struct {
	OwnerPresence
	// AfterMonths is the owner's setting; 0 means never.
	AfterMonths int
	// OpensAt is when an admin may hand over; zero when never.
	OpensAt time.Time
	// Open is true once the owner has been away longer than they allowed.
	Open bool
}

func validHandoverChoice(months int) bool {
	for _, m := range HandoverChoices {
		if m == months {
			return true
		}
	}
	return false
}

// handoverStatus works out one campaign's status at a moment.
func handoverStatus(p OwnerPresence, months int, now time.Time) HandoverStatus {
	st := HandoverStatus{OwnerPresence: p, AfterMonths: months}
	if months > 0 && !p.LastSeen.IsZero() {
		st.OpensAt = p.LastSeen.AddDate(0, months, 0)
		st.Open = !now.Before(st.OpensAt)
	}
	return st
}

// SetHandoverAfter saves the owner's "If you're away" choice.
func (s *campaignService) SetHandoverAfter(ctx context.Context, campaignID string, months int) error {
	if !validHandoverChoice(months) {
		return apperror.NewBadRequest("choose never, 3, 6 or 12 months")
	}
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	settings := campaign.ParseSettings()
	settings.HandoverAfterMonths = months
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("marshaling settings: %w", err))
	}
	return s.saveSettingsChanges(ctx, campaign, campaignID, settingsJSON)
}

// HandoverStatuses returns the hand-over status of each listed campaign.
func (s *campaignService) HandoverStatuses(ctx context.Context, list []Campaign) (map[string]HandoverStatus, error) {
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	presence, err := s.repo.OwnerPresence(ctx, ids)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	now := time.Now().UTC()
	out := make(map[string]HandoverStatus, len(list))
	for _, c := range list {
		if p, ok := presence[c.ID]; ok {
			out[c.ID] = handoverStatus(p, c.ParseSettings().HandoverAfterMonths, now)
		}
	}
	return out, nil
}

// AdminHandOver gives a campaign to one of its members once the owner has
// been away longer than they allowed. The old owner stays on as Co-DM and is
// emailed. Everything is checked again here, never trusted from the page.
func (s *campaignService) AdminHandOver(ctx context.Context, campaignID, toUserID string) (*HandoverStatus, error) {
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	statuses, err := s.HandoverStatuses(ctx, []Campaign{*campaign})
	if err != nil {
		return nil, err
	}
	st, ok := statuses[campaignID]
	if !ok {
		return nil, apperror.NewNotFound("campaign owner not found")
	}
	if st.AfterMonths == 0 {
		return nil, apperror.NewForbidden("the owner hasn't allowed a hand-over")
	}
	if !st.Open {
		return nil, apperror.NewForbidden("the owner hasn't been away long enough yet")
	}
	if toUserID == st.OwnerID {
		return nil, apperror.NewBadRequest("choose someone other than the owner")
	}
	members, err := s.repo.ListMembers(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	var target *CampaignMember
	for i := range members {
		if members[i].UserID == toUserID {
			target = &members[i]
		}
	}
	if target == nil {
		return nil, apperror.NewBadRequest("the new owner must already be a member")
	}
	if target.IsGuest {
		return nil, apperror.NewBadRequest("a guest can't own a campaign; they need to keep their account first")
	}

	// A hand-over the absent owner started earlier would otherwise still be
	// waiting to give the campaign to someone else.
	if pending, err := s.repo.FindTransferByCampaign(ctx, campaignID); err == nil && pending != nil {
		if err := s.repo.DeleteTransfer(ctx, pending.ID); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("cancelling pending transfer: %w", err))
		}
	}
	if err := s.repo.TransferOwnership(ctx, campaignID, st.OwnerID, toUserID); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("handing over: %w", err))
	}
	// The new owner no longer needs a DM grant; the old one gets Co-DM.
	if err := s.revokeDmGrant(ctx, campaignID, toUserID); err != nil {
		slog.Warn("dropping new owner's DM grant", slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	if err := s.SetMemberAccess(ctx, campaignID, st.OwnerID, AccessCoDM); err != nil {
		slog.Error("making old owner Co-DM", slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	if s.connRevoker != nil {
		s.connRevoker.RevokeUser(campaignID, st.OwnerID)
	}
	s.emailOldOwner(ctx, campaign.Name, st, target.DisplayName)

	slog.Info("campaign handed over by an admin",
		slog.String("campaign_id", campaignID),
		slog.String("from", st.OwnerID),
		slog.String("to", toUserID),
	)
	return &st, nil
}

// emailOldOwner tells the absent owner what happened and that they are
// still in the campaign. A failure is logged; the hand-over stands.
func (s *campaignService) emailOldOwner(ctx context.Context, campaignName string, st HandoverStatus, newOwner string) {
	if s.mail == nil || !s.mail.IsConfigured(ctx) || s.users == nil {
		return
	}
	owner, err := s.users.FindUserByID(ctx, st.OwnerID)
	if err != nil || owner.Email == "" {
		return
	}
	body := fmt.Sprintf(
		"Hello %s,\n\n"+
			"You set the campaign \"%s\" on Chronicle so a site admin could hand it over if you were away for %d months. "+
			"You were last seen on %s, so an admin has made %s its owner.\n\n"+
			"You're still in the campaign as Co-DM, and everything you made is still there. "+
			"If you'd like it back, ask %s or your site admin.\n\n%s",
		owner.DisplayName, campaignName, st.AfterMonths, st.LastSeen.Format("January 2, 2006"),
		newOwner, newOwner, s.baseURL)
	if err := s.mail.SendMail(ctx, []string{owner.Email}, "Your campaign has a new owner", body); err != nil {
		slog.Warn("emailing old owner after hand-over", slog.String("user_id", st.OwnerID), slog.Any("error", err))
	}
}
