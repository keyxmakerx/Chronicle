package designs

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CampaignDirectory is what the sharing rules need from campaigns. App wiring
// implements it over the campaigns service, so this package never imports it.
type CampaignDirectory interface {
	// Campaign returns nil, nil for a campaign that is missing or in the Trash.
	Campaign(ctx context.Context, id string) (*CampaignInfo, error)
	// Member says whether userID is the campaign's Owner (campaigns.RoleOwner,
	// not a co-DM) and the display name to put on what they make. A user who is
	// not a member is (name "", owner false).
	Member(ctx context.Context, campaignID, userID string) (name string, owner bool, err error)
	// Live says whether the campaign exists and is neither in the Trash nor
	// archived. A design from a campaign that is not live is not offered to
	// anyone else, as if it had never been shared.
	Live(ctx context.Context, id string) (bool, error)
	// OwnedCampaigns lists the live, unarchived campaigns userID is Owner of.
	OwnedCampaigns(ctx context.Context, userID string) ([]CampaignInfo, error)
}

// Service is the library and its sharing. Every method takes the acting user
// and checks they are the Owner of the campaign it acts for (or, for links,
// of the campaign they copy into); handlers pass the route's campaign and do
// no checking of their own beyond the route gate.
type Service interface {
	List(ctx context.Context, campaignID, userID, kind string) ([]Design, error)
	Get(ctx context.Context, campaignID, userID, id string) (*Design, error)
	Save(ctx context.Context, campaignID, userID string, in SaveInput) (*Design, error)
	// Check runs Save's checks without saving.
	Check(ctx context.Context, campaignID, userID string, in SaveInput) error
	Update(ctx context.Context, campaignID, userID, id string, in UpdateDesignInput) (*Design, error)
	Delete(ctx context.Context, campaignID, userID, id string) error

	ShareState(ctx context.Context, campaignID, userID, id string) (*ShareState, error)
	SetLink(ctx context.Context, campaignID, userID, id string, on bool) (*ShareState, error)
	SearchOwners(ctx context.Context, campaignID, userID, query string) ([]OwnerMatch, error)
	AddOwner(ctx context.Context, campaignID, userID, id, ownerUserID string) (*ShareState, error)
	RemoveOwner(ctx context.Context, campaignID, userID, id, ownerUserID string) (*ShareState, error)
	StopSharing(ctx context.Context, campaignID, userID, id string) (*ShareState, error)

	PreviewCode(ctx context.Context, campaignID, userID, code string) (*Preview, error)
	TakeCode(ctx context.Context, campaignID, userID, code string) (*Design, error)
	PreviewLink(ctx context.Context, userID, token string) (*LinkPreview, error)
	TakeLink(ctx context.Context, userID, token, targetCampaignID string) (*Design, error)
	SharedWithMe(ctx context.Context, campaignID, userID, kind string) ([]SharedDesign, error)
	PreviewShared(ctx context.Context, campaignID, userID, designID string) (*Preview, error)
	TakeShared(ctx context.Context, campaignID, userID, designID string) (*Design, error)
}

type service struct {
	repo  Repository
	dir   CampaignDirectory
	kinds KindRegistry
}

// NewService creates the service.
func NewService(repo Repository, dir CampaignDirectory, kinds KindRegistry) Service {
	return &service{repo: repo, dir: dir, kinds: kinds}
}

const (
	msgOwnersOnly   = "only a campaign's owner can use saved designs"
	msgNotFound     = "that design was not found"
	msgOtherSystem  = "this design was made for a different game system than this campaign uses"
	msgLinkGone     = "this link is not available"
	msgNotAnOwner   = "only a campaign's owner can open a design link"
	linkTokenBytes  = 16
	searchMinRunes  = 2
	searchMaxOwners = 10
)

// requireOwner is the gate every campaign-scoped call passes first. It is
// checked here as well as on the route so a service caller (the AI, a test)
// cannot skip it; a co-DM shares the Owner's visibility role but is refused.
func (s *service) requireOwner(ctx context.Context, campaignID, userID string) (*CampaignInfo, string, error) {
	if userID == "" {
		return nil, "", apperror.NewForbidden(msgOwnersOnly)
	}
	c, err := s.dir.Campaign(ctx, campaignID)
	if err != nil {
		return nil, "", apperror.NewInternal(err)
	}
	if c == nil {
		return nil, "", apperror.NewNotFound("campaign not found")
	}
	name, owner, err := s.dir.Member(ctx, campaignID, userID)
	if err != nil {
		return nil, "", apperror.NewInternal(err)
	}
	if !owner {
		return nil, "", apperror.NewForbidden(msgOwnersOnly)
	}
	return c, name, nil
}

// own loads a design of this campaign. One in another campaign is NotFound,
// the same as a missing one, so ids are never confirmed across campaigns.
func (s *service) own(ctx context.Context, campaignID, id string) (*Design, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if d == nil || d.CampaignID != campaignID {
		return nil, apperror.NewNotFound(msgNotFound)
	}
	return d, nil
}

func (s *service) kind(name string) (Kind, error) {
	k, ok := s.kinds.Kind(name)
	if !ok {
		return nil, apperror.NewBadRequest(fmt.Sprintf("%q is not a kind of design this server knows", name))
	}
	return k, nil
}

// cleanText trims a label and drops control characters, which have no place
// in a name that is shown to other owners.
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func checkLabel(what, s string, max int, required bool) (string, error) {
	s = cleanText(s)
	if required && s == "" {
		return "", apperror.NewBadRequest(what + " is required")
	}
	if utf8.RuneCountInString(s) > max {
		return "", apperror.NewBadRequest(fmt.Sprintf("%s must be at most %d characters", what, max))
	}
	return s, nil
}

func newToken() (string, error) {
	b := make([]byte, linkTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newID() string { return uuid.NewString() }
