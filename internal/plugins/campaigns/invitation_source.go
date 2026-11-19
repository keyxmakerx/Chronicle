package campaigns

import (
	"context"
	"log/slog"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// invitationMemberCap bounds the roster read for the door, so a big table
// costs one short list.
const invitationMemberCap = 24

// NextNightFunc finds a campaign's next planned game night, nil when none.
// It is supplied by the app so campaigns imports no sessions code.
type NextNightFunc func(ctx context.Context, campaignID string, now time.Time) *auth.InvitationNight

// InvitationSource answers the door's three lookups: a share link, an
// emailed invite and a guest code. Anything not valid right now, including a
// campaign that is archived or in the Trash, gives nil, so the pages stay
// plain and never confirm what is real.
type InvitationSource struct {
	campaigns CampaignRepository
	invites   InviteRepository
	codes     *GuestCodeService
	nextNight NextNightFunc
	now       func() time.Time
}

// NewInvitationSource builds the door's lookups. nextNight may be nil.
func NewInvitationSource(campaigns CampaignRepository, invites InviteRepository, codes *GuestCodeService, nextNight NextNightFunc) *InvitationSource {
	return &InvitationSource{campaigns: campaigns, invites: invites, codes: codes, nextNight: nextNight, now: time.Now}
}

// ByLink reads the invitation for a campaign's share link.
func (s *InvitationSource) ByLink(ctx context.Context, code string) *auth.Invitation {
	if code == "" {
		return nil
	}
	c, err := s.campaigns.FindByJoinCode(ctx, code)
	if err != nil || c == nil {
		return nil
	}
	return s.build(ctx, c)
}

// ByToken reads the invitation for an emailed invite that is unused and unexpired.
func (s *InvitationSource) ByToken(ctx context.Context, token string) *auth.Invitation {
	if token == "" {
		return nil
	}
	inv, err := s.invites.GetByToken(ctx, token)
	if err != nil || inv == nil || !inv.IsPending() {
		return nil
	}
	c, err := s.campaigns.FindByID(ctx, inv.CampaignID)
	if err != nil || c == nil {
		return nil
	}
	return s.build(ctx, c)
}

// ByGuestCode reads the invitation for a guest code that is still live.
func (s *InvitationSource) ByGuestCode(ctx context.Context, code string) *auth.Invitation {
	campaignID, ok := s.codes.Peek(ctx, code)
	if !ok {
		return nil
	}
	c, err := s.campaigns.FindByID(ctx, campaignID)
	if err != nil || c == nil {
		return nil
	}
	return s.build(ctx, c)
}

// build is the invitation for a live campaign: names and pictures of its
// members and the next night, nothing a player could not already see about
// their own table. An archived campaign takes no one, so it shows nothing.
func (s *InvitationSource) build(ctx context.Context, c *Campaign) *auth.Invitation {
	if c.IsArchived() {
		return nil
	}
	inv := &auth.Invitation{CampaignName: c.Name}
	if c.BackdropPath != nil {
		inv.BackdropPath = *c.BackdropPath
	}
	// The table and the night are extras: if either can't be read the
	// invitation still stands with the name and picture.
	members, err := s.campaigns.ListMembers(ctx, c.ID)
	if err != nil {
		slog.Warn("reading the table for an invitation", slog.String("campaign_id", c.ID), slog.Any("error", err))
	}
	for _, m := range members {
		if len(inv.Members) >= invitationMemberCap {
			break
		}
		if m.DisplayName == "" {
			continue
		}
		pm := auth.InvitationMember{Name: m.DisplayName}
		if m.AvatarPath != nil {
			pm.AvatarPath = *m.AvatarPath
		}
		inv.Members = append(inv.Members, pm)
	}
	if s.nextNight != nil {
		inv.NextNight = s.nextNight(ctx, c.ID, s.now())
	}
	return inv
}
