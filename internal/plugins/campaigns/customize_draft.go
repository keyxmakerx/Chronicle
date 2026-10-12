package campaigns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// maxCustomizeDraftBytes caps one stored draft. A draft is a few settings and
// lists of labels; a megabyte leaves room for a large category tree while
// keeping a runaway client from filling the table.
const maxCustomizeDraftBytes = 1 << 20

// CustomizeDraft is the owner's unsaved Customize work for one campaign: a
// JSON object of named sections, each owned by the code that applies it.
type CustomizeDraft struct {
	// Version counts the writes to this draft; a writer must echo the version
	// it read so two tabs cannot overwrite each other.
	Version   int
	Draft     json.RawMessage
	UpdatedAt time.Time
}

// CustomizeDraftService keeps the Customize draft on the server until the
// owner presses Save or Discard. It is separate from CampaignService because
// it knows nothing about what a section means, only how to store one safely.
type CustomizeDraftService interface {
	// Get returns the owner's draft, or nil, nil when there is none.
	Get(ctx context.Context, campaignID, userID string) (*CustomizeDraft, error)
	// Save stores a draft. version is the one the caller last saw; 0 means
	// "I believe there is no draft". A stale version is a Conflict. An empty
	// object removes the draft and returns version 0.
	Save(ctx context.Context, campaignID, userID string, version int, draft json.RawMessage, allowedKeys []string) (int, error)
	// Discard removes the draft. version 0 removes it whatever its version.
	Discard(ctx context.Context, campaignID, userID string, version int) error
}

type customizeDraftService struct {
	repo CustomizeDraftRepository
}

// NewCustomizeDraftService creates the draft service.
func NewCustomizeDraftService(repo CustomizeDraftRepository) CustomizeDraftService {
	return &customizeDraftService{repo: repo}
}

func (s *customizeDraftService) Get(ctx context.Context, campaignID, userID string) (*CustomizeDraft, error) {
	d, err := s.repo.Get(ctx, campaignID, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return d, nil
}

func (s *customizeDraftService) Save(ctx context.Context, campaignID, userID string, version int, draft json.RawMessage, allowedKeys []string) (int, error) {
	if version < 0 {
		return 0, apperror.NewBadRequest("invalid draft version")
	}
	if len(draft) > maxCustomizeDraftBytes {
		return 0, apperror.NewBadRequest("the draft is too large")
	}
	sections, err := parseCustomizeDraft(draft)
	if err != nil {
		return 0, err
	}
	// Unknown sections are refused rather than dropped: a section nobody
	// applies would sit in the draft forever and make Save look unfinished.
	allowed := make(map[string]bool, len(allowedKeys))
	for _, k := range allowedKeys {
		allowed[k] = true
	}
	for k := range sections {
		if !allowed[k] {
			return 0, apperror.NewBadRequest(fmt.Sprintf("unknown draft section %q", k))
		}
	}

	if len(sections) == 0 {
		if version == 0 {
			// "No draft" over an existing one must not delete it unseen.
			cur, err := s.repo.Get(ctx, campaignID, userID)
			if err != nil {
				return 0, apperror.NewInternal(err)
			}
			if cur != nil {
				return 0, errDraftChanged
			}
			return 0, nil
		}
		return 0, s.clear(ctx, campaignID, userID, version)
	}

	// Stored compact so the size cap and the stored bytes agree.
	var buf bytes.Buffer
	if err := json.Compact(&buf, draft); err != nil {
		return 0, apperror.NewBadRequest("the draft is not valid JSON")
	}

	if version == 0 {
		ok, err := s.repo.Insert(ctx, campaignID, userID, buf.Bytes())
		if err != nil {
			return 0, apperror.NewInternal(err)
		}
		if !ok {
			return 0, errDraftChanged
		}
		return 1, nil
	}
	ok, err := s.repo.Update(ctx, campaignID, userID, version, buf.Bytes())
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	if !ok {
		return 0, errDraftChanged
	}
	return version + 1, nil
}

func (s *customizeDraftService) Discard(ctx context.Context, campaignID, userID string, version int) error {
	if version < 0 {
		return apperror.NewBadRequest("invalid draft version")
	}
	return s.clear(ctx, campaignID, userID, version)
}

// clear removes a draft. Removing one that is already gone succeeds, since
// the caller wanted it gone; removing a newer one than the caller saw does
// not, because it holds work the caller has not seen.
func (s *customizeDraftService) clear(ctx context.Context, campaignID, userID string, version int) error {
	ok, err := s.repo.Delete(ctx, campaignID, userID, version)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if ok {
		return nil
	}
	cur, err := s.repo.Get(ctx, campaignID, userID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if cur != nil {
		return errDraftChanged
	}
	return nil
}

// errDraftChanged is the Conflict a stale version produces.
var errDraftChanged = apperror.NewConflict("the Customize draft changed somewhere else; reload to see it")

// parseCustomizeDraft decodes a draft as an object of sections.
func parseCustomizeDraft(draft json.RawMessage) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(draft)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, apperror.NewBadRequest("the draft must be a JSON object")
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &sections); err != nil {
		return nil, apperror.NewBadRequest("the draft is not valid JSON")
	}
	return sections, nil
}
