// member_access.go — the People page's one role menu: Player, Scribe or
// Co-DM. Co-DM is not a stored role: it is a Scribe who also holds the
// campaign's DM-only grant (CampaignSettings.DmGrantIDs). Keeping it as role
// plus grant means everything that already reads either one keeps working.

package campaigns

import (
	"context"
	"slices"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// Member access choices on the role menu.
const (
	AccessPlayer = "player"
	AccessScribe = "scribe"
	AccessCoDM   = "codm"
)

// MemberAccess is the menu value for a member: Co-DM for a Scribe with the
// DM-only grant, otherwise their role. A Player who holds the grant (set up
// before the menu existed) reads as Player; MemberAccessNote says so.
func MemberAccess(c *Campaign, m CampaignMember) string {
	if m.Role == RoleScribe && hasDmGrant(c, m.UserID) {
		return AccessCoDM
	}
	if m.Role == RoleScribe {
		return AccessScribe
	}
	return AccessPlayer
}

// MemberAccessLabel is the member's role as the People page words it.
func MemberAccessLabel(c *Campaign, m CampaignMember) string {
	switch {
	case m.Role == RoleOwner:
		return "Owner"
	case MemberAccess(c, m) == AccessCoDM:
		return "Co-DM"
	case m.Role == RolePlayer && hasDmGrant(c, m.UserID):
		return "Player with DM access"
	default:
		return m.Role.DisplayName()
	}
}

// MemberAccessNote explains the one setup the menu cannot show: a Player who
// can also read DM-only content. It keeps that until their role is changed.
func MemberAccessNote(c *Campaign, m CampaignMember) string {
	if m.Role == RolePlayer && hasDmGrant(c, m.UserID) {
		return "Can also read DM-only content. Choosing a role here replaces that."
	}
	return ""
}

// accessRoleGrant maps a menu choice to the stored role and grant.
func accessRoleGrant(access string) (Role, bool, error) {
	switch access {
	case AccessPlayer:
		return RolePlayer, false, nil
	case AccessScribe:
		return RoleScribe, false, nil
	case AccessCoDM:
		return RoleScribe, true, nil
	}
	return RoleNone, false, apperror.NewBadRequest("role must be player, scribe or codm")
}

// SetMemberAccess applies a role-menu choice: the role (only when it
// changes, since an unchanged write updates no rows) and then the DM-only
// grant. Both go through the existing guarded paths, so the owner stays
// untouchable and anyone losing access has their live socket dropped.
func (s *campaignService) SetMemberAccess(ctx context.Context, campaignID, userID, access string) error {
	role, grant, err := accessRoleGrant(access)
	if err != nil {
		return err
	}
	member, err := s.repo.FindMember(ctx, campaignID, userID)
	if err != nil {
		return err
	}
	if member.Role == RoleOwner {
		return apperror.NewBadRequest("cannot change the owner's role; transfer ownership first")
	}
	campaign, err := s.repo.FindByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign == nil {
		return apperror.NewNotFound("campaign not found")
	}
	grants := campaign.ParseSettings().DmGrantIDs
	has := slices.Contains(grants, userID)
	// Taking DM access away goes first and does not re-check the other
	// grants, so an id left behind by a deleted account can't stop
	// it.
	if !grant && has {
		if err := s.revokeDmGrant(ctx, campaignID, userID); err != nil {
			return err
		}
		if s.connRevoker != nil {
			s.connRevoker.RevokeUser(campaignID, userID)
		}
	}
	if member.Role != role {
		if err := s.UpdateMemberRole(ctx, campaignID, userID, role); err != nil {
			return err
		}
	}
	if grant && !has {
		// Grants for people who are no longer members are dropped, since
		// UpdateDmGrants refuses a list naming a non-member.
		kept := make([]string, 0, len(grants)+1)
		for _, id := range grants {
			if m, err := s.repo.FindMember(ctx, campaignID, id); err == nil && m != nil {
				kept = append(kept, id)
			}
		}
		return s.UpdateDmGrants(ctx, campaignID, append(kept, userID))
	}
	return nil
}

// showFoundryColumn is whether the People table shows "In Foundry": only to
// the owner (what Foundry reports about people is the owner's diagnostic,
// never shown to players) and only once the campaign syncs with Foundry.
func showFoundryColumn(ctx context.Context, cc *CampaignContext) bool {
	return cc.MemberRole >= RoleOwner && layouts.IsAddonEnabled(ctx, "sync-api")
}
