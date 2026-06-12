package tags

import (
	"context"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// MemberChecker resolves a campaign member for grant subject validation +
// label display. Satisfied by campaigns.CampaignService.
type MemberChecker interface {
	GetMember(ctx context.Context, campaignID, userID string) (*campaigns.CampaignMember, error)
}

// GroupChecker resolves a campaign group for grant subject validation + label
// display. Satisfied by campaigns.GroupService.
type GroupChecker interface {
	GetGroup(ctx context.Context, groupID int) (*campaigns.CampaignGroup, error)
}

// TagGrantService owns the business logic for tag visibility grants: it gates
// every operation on the tag belonging to the acting campaign, and validates
// that a grant's subject actually exists in that campaign before persisting.
// Granting a tag is a power — it can reveal dm_only content — so the handler
// surface is Owner-gated and the service refuses to grant to a non-existent
// role/user/group (a dangling grant would be invisible in the UI yet still
// widen visibility, exactly the silent-exposure failure this feature guards).
type TagGrantService interface {
	// ListByTag returns the grants on a tag (verifying the tag is in campaign).
	ListByTag(ctx context.Context, campaignID string, tagID int) ([]TagPermission, error)

	// Create validates the subject and adds a grant to the tag.
	Create(ctx context.Context, campaignID string, tagID int, subjectType, subjectID, createdBy string) (*TagPermission, error)

	// Delete removes a grant, verifying it belongs to a tag in the campaign.
	Delete(ctx context.Context, campaignID string, tagID, permID int) error

	// GrantsForEntity returns the entity's tag-derived grants with resolved
	// human subject labels, for the effective-visibility glance.
	GrantsForEntity(ctx context.Context, campaignID, entityID string) ([]EntityTagGrant, error)
}

// tagGrantService implements TagGrantService.
type tagGrantService struct {
	repo    TagPermissionRepository
	tags    TagRepository
	members MemberChecker
	groups  GroupChecker
}

// NewTagGrantService creates a TagGrantService. members/groups validate +
// label grant subjects; either may be nil in tests that don't exercise those
// subject types (validation then rejects user/group grants defensively).
func NewTagGrantService(repo TagPermissionRepository, tags TagRepository, members MemberChecker, groups GroupChecker) TagGrantService {
	return &tagGrantService{repo: repo, tags: tags, members: members, groups: groups}
}

// requireTagInCampaign loads a tag and confirms it belongs to the campaign,
// returning a 404 (not 403) on mismatch so cross-campaign tag IDs can't be
// probed.
func (s *tagGrantService) requireTagInCampaign(ctx context.Context, campaignID string, tagID int) (*Tag, error) {
	tag, err := s.tags.FindByID(ctx, tagID)
	if err != nil {
		return nil, err
	}
	if tag.CampaignID != campaignID {
		return nil, apperror.NewNotFound("tag not found")
	}
	return tag, nil
}

func (s *tagGrantService) ListByTag(ctx context.Context, campaignID string, tagID int) ([]TagPermission, error) {
	if _, err := s.requireTagInCampaign(ctx, campaignID, tagID); err != nil {
		return nil, err
	}
	return s.repo.ListByTag(ctx, tagID)
}

func (s *tagGrantService) Create(ctx context.Context, campaignID string, tagID int, subjectType, subjectID, createdBy string) (*TagPermission, error) {
	if _, err := s.requireTagInCampaign(ctx, campaignID, tagID); err != nil {
		return nil, err
	}
	if err := s.validateSubject(ctx, campaignID, subjectType, subjectID); err != nil {
		return nil, err
	}
	p := &TagPermission{
		TagID:       tagID,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		CreatedBy:   createdBy,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *tagGrantService) Delete(ctx context.Context, campaignID string, tagID, permID int) error {
	if _, err := s.requireTagInCampaign(ctx, campaignID, tagID); err != nil {
		return err
	}
	perm, err := s.repo.GetByID(ctx, permID)
	if err != nil {
		return err
	}
	if perm.TagID != tagID {
		return apperror.NewNotFound("tag grant not found")
	}
	return s.repo.Delete(ctx, permID)
}

// validateSubject confirms the grant subject exists in the campaign. A grant to
// a non-existent subject is rejected so the UI can always render every grant
// (no silent, un-revocable exposure).
func (s *tagGrantService) validateSubject(ctx context.Context, campaignID, subjectType, subjectID string) error {
	switch subjectType {
	case SubjectRole:
		role, err := strconv.Atoi(subjectID)
		if err != nil || role < permissions.RolePlayer || role > permissions.RoleOwner {
			return apperror.NewBadRequest("invalid role for grant (must be Player, Scribe, or Owner)")
		}
		return nil
	case SubjectUser:
		if s.members == nil {
			return apperror.NewBadRequest("user grants are not available")
		}
		member, err := s.members.GetMember(ctx, campaignID, subjectID)
		if err != nil || member == nil {
			return apperror.NewBadRequest("user is not a member of this campaign")
		}
		return nil
	case SubjectGroup:
		groupID, err := strconv.Atoi(subjectID)
		if err != nil {
			return apperror.NewBadRequest("invalid group id")
		}
		if s.groups == nil {
			return apperror.NewBadRequest("group grants are not available")
		}
		group, err := s.groups.GetGroup(ctx, groupID)
		if err != nil || group == nil || group.CampaignID != campaignID {
			return apperror.NewBadRequest("group does not belong to this campaign")
		}
		return nil
	default:
		return apperror.NewBadRequest("subject type must be role, user, or group")
	}
}

func (s *tagGrantService) GrantsForEntity(ctx context.Context, campaignID, entityID string) ([]EntityTagGrant, error) {
	grants, err := s.repo.ListGrantsForEntity(ctx, entityID)
	if err != nil {
		return nil, err
	}
	for i := range grants {
		grants[i].SubjectLabel = s.subjectLabel(ctx, campaignID, grants[i].SubjectType, grants[i].SubjectID)
	}
	return grants, nil
}

// subjectLabel resolves a grant subject to a human label for the glance
// tooltip. Resolution failures fall back to a safe generic label rather than
// erroring — the glance must always render something truthful, never blank.
func (s *tagGrantService) subjectLabel(ctx context.Context, campaignID, subjectType, subjectID string) string {
	switch subjectType {
	case SubjectRole:
		switch subjectID {
		case "1":
			return "Players"
		case "2":
			return "Scribes"
		case "3":
			return "Owners"
		default:
			return "a role"
		}
	case SubjectUser:
		if s.members != nil {
			if m, err := s.members.GetMember(ctx, campaignID, subjectID); err == nil && m != nil && m.DisplayName != "" {
				return m.DisplayName
			}
		}
		return "a member"
	case SubjectGroup:
		if s.groups != nil {
			if gid, err := strconv.Atoi(subjectID); err == nil {
				if g, err := s.groups.GetGroup(ctx, gid); err == nil && g != nil {
					return g.Name
				}
			}
		}
		return "a group"
	default:
		return "someone"
	}
}
