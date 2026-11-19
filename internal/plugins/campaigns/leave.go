package campaigns

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// leave.go is a member walking away from a campaign by their own choice. It
// is not a removal: the person is not remembered as removed and the join
// link keeps working, so an invite or the link brings them back. What they
// wrote is never touched; their characters go back to the table.

// NotifMemberLeft is the bell type of the owner's "Tam left Therin" note.
// It has no category on the account page, so it is always delivered.
const NotifMemberLeft = "member_left"

// CharacterReleaser hands a leaver's claimed characters back to the table
// ("Free to pick"). Campaigns can't reach into the entities plugin's
// repository, so the app wires this narrow door.
type CharacterReleaser interface {
	// ReleaseCharacters clears the user's claim on every character in the
	// campaign and returns the names of the live ones, so the owner can be
	// told which are free.
	ReleaseCharacters(ctx context.Context, campaignID, userID string) ([]string, error)
	// ClaimedCharacters names the live characters the user holds, by
	// campaign, so the Leave box can say who goes back to the table.
	ClaimedCharacters(ctx context.Context, userID string, campaignIDs []string) (map[string][]string, error)
}

// LeaveNotifier tells the owner somebody left. The sessions plugin's
// notification store sits behind it.
type LeaveNotifier interface {
	// MemberLeft writes one bell row to each of userIDs. action is the label
	// of the link, shown under the line.
	MemberLeft(ctx context.Context, campaignID string, userIDs []string, message, detail, action, link string) error
}

// ConfigureLeaving wires what leaving needs beyond the campaign's own data.
// Either may be nil, in which case that step is skipped.
func ConfigureLeaving(svc CampaignService, chars CharacterReleaser, notifier LeaveNotifier) {
	if s, ok := svc.(*campaignService); ok {
		s.characters = chars
		s.leaveNotifier = notifier
	}
}

// LeaveCampaign ends the user's own membership. The owner is refused, since
// every campaign needs one. Unlike RemoveMember it neither marks the person
// removed nor replaces the join link.
func (s *campaignService) LeaveCampaign(ctx context.Context, campaignID, userID string) error {
	member, err := s.repo.FindMember(ctx, campaignID, userID)
	if err != nil {
		return err
	}
	if member.Role == RoleOwner {
		return apperror.NewBadRequest("hand the campaign over before you leave it")
	}
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}

	// Access ends first, on every device: the grant, look-around passes,
	// live sockets and the removal hooks all run inside removeMember. If
	// that fails nothing else has changed and the person can try again.
	if err := s.removeMember(ctx, campaignID, userID, false); err != nil {
		return err
	}

	// Past this point the person is already out, so a failure is logged and
	// the owner can still free a character by hand; it never turns a
	// completed leave into an error.
	var freed []string
	if s.characters != nil {
		freed, err = s.characters.ReleaseCharacters(ctx, campaignID, userID)
		if err != nil {
			slog.Error("freeing a leaver's characters failed",
				slog.String("campaign_id", campaignID), slog.String("user_id", userID), slog.Any("error", err))
			freed = nil
		}
	}
	s.tellOwnersMemberLeft(ctx, campaign, member, freed)

	slog.Info("member left campaign",
		slog.String("campaign_id", campaignID),
		slog.String("user_id", userID),
	)
	return nil
}

// tellOwnersMemberLeft sends the owner their single note. Other players are
// not told; they just see the character change hands.
func (s *campaignService) tellOwnersMemberLeft(ctx context.Context, campaign *Campaign, member *CampaignMember, freed []string) {
	if s.leaveNotifier == nil {
		return
	}
	members, err := s.repo.ListMembers(ctx, campaign.ID)
	if err != nil {
		slog.Warn("leave note: members unavailable", slog.String("campaign_id", campaign.ID), slog.Any("error", err))
		return
	}
	var owners []string
	for _, m := range members {
		if m.Role == RoleOwner {
			owners = append(owners, m.UserID)
		}
	}
	if len(owners) == 0 {
		return
	}
	who := strings.TrimSpace(member.DisplayName)
	if who == "" {
		who = "A player"
	}
	message, detail, action := leaveNoteText(who, campaign.Name, freed)
	link := fmt.Sprintf("/campaigns/%s/characters?open=players", campaign.ID)
	if err := s.leaveNotifier.MemberLeft(ctx, campaign.ID, owners, message, detail, action, link); err != nil {
		slog.Warn("leave note: sending failed", slog.String("campaign_id", campaign.ID), slog.Any("error", err))
	}
}

// leaveNoteText words the owner's note. With no character the free-to-pick
// sentence and the action go; with several, all are named.
func leaveNoteText(who, campaignName string, freed []string) (message, detail, action string) {
	message = who + " left " + campaignName
	if len(freed) == 0 {
		return message, who + "'s posts and notes stay.", ""
	}
	names := joinNames(freed)
	verb := "is"
	if len(freed) > 1 {
		verb = "are"
	}
	detail = names + " " + verb + " free to pick. " + who + "'s posts and notes stay."
	return message, detail, "Give " + names + " to someone"
}

// joinNames reads names as "A, B and C".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// ClaimedCharacters names the characters the user holds in each of the given
// campaigns. A failed or unwired lookup gives none, so the Leave box simply
// drops its character line rather than blocking the list.
func (s *campaignService) ClaimedCharacters(ctx context.Context, userID string, campaignIDs []string) (map[string][]string, error) {
	if s.characters == nil || len(campaignIDs) == 0 {
		return map[string][]string{}, nil
	}
	return s.characters.ClaimedCharacters(ctx, userID, campaignIDs)
}

// OwnedMemberships lists the campaigns the user owns now, which they hand
// over rather than leave.
func (s *campaignService) OwnedMemberships(ctx context.Context, userID string) ([]string, error) {
	ids, err := s.repo.ListOwnedMembershipIDs(ctx, userID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing owned memberships: %w", err))
	}
	return ids, nil
}

// LeaveView is what My campaigns needs to draw each card's menu: which
// campaigns the person owns (they hand over instead) and, for the rest, the
// characters that would go back to the table.
type LeaveView struct {
	Owned      map[string]bool
	Characters map[string][]string
}

// leaveCharacterLine is the Leave box's line about the person's characters,
// empty when they hold none. It never picks a pronoun for a character: the
// code does not know one.
func leaveCharacterLine(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0] + " stays with the campaign. The DM can give them to someone else."
	}
	return joinNames(names) + " stay with the campaign. The DM can give them to someone else."
}
