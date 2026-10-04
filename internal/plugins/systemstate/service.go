package systemstate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
)

// MaxHalfBytes caps each JSON half. The state is a small tracker document, not
// a content store, and the cap keeps one page from growing an unbounded row.
const MaxHalfBytes = 16 * 1024

// idPattern is the shape of a system id and a state key. It is the same for
// both so a key can never smuggle in separators or case variants that would
// split one document across spellings.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// EntityLookup resolves which campaign an entity belongs to. The service
// compares it with the campaign in the URL, so a caller can never reach another
// campaign's page by pairing its own campaign id with a foreign entity id.
// Implemented in app over the entities service (T-B2: no import here).
type EntityLookup interface {
	// EntityCampaignID returns the entity's campaign, or a NotFound error.
	EntityCampaignID(ctx context.Context, entityID string) (string, error)
}

// SystemChecker reports whether a game system is enabled for a campaign, so a
// disabled system cannot accumulate state. Implemented in app over the systems
// registry.
type SystemChecker interface {
	IsSystemEnabled(ctx context.Context, campaignID, systemID string) (bool, error)
}

// UpdatePublisher announces a successful write. The message carries ids only,
// never state content; it is delivered to the GM side. Implemented in app over
// the websocket bus.
type UpdatePublisher interface {
	PublishSystemStateUpdated(campaignID, entityID, systemID, key string)
}

// Service is the business-logic boundary for system state.
type Service interface {
	// Get returns both halves; a page with no row reads as two empty objects.
	// Which halves a caller may see is decided by the caller.
	Get(ctx context.Context, campaignID, entityID, systemID, key string) (State, error)
	// Put stores the halves with the partial-update contract: a nil half
	// keeps the stored half, a non-nil half replaces it. Returns the state
	// as stored.
	Put(ctx context.Context, campaignID, entityID, systemID, key string, gm, public *json.RawMessage, userID string) (State, error)
}

type service struct {
	repo      Repository
	entities  EntityLookup
	systems   SystemChecker
	publisher UpdatePublisher
}

// NewService builds the service. publisher may be nil (no live notification).
func NewService(repo Repository, entities EntityLookup, systems SystemChecker, publisher UpdatePublisher) Service {
	return &service{repo: repo, entities: entities, systems: systems, publisher: publisher}
}

func (s *service) Get(ctx context.Context, campaignID, entityID, systemID, key string) (State, error) {
	if err := validateIDs(systemID, key); err != nil {
		return State{}, err
	}
	if err := s.checkEntity(ctx, campaignID, entityID); err != nil {
		return State{}, err
	}
	row, err := s.repo.Get(ctx, campaignID, entityID, systemID, key)
	if err != nil {
		return State{}, apperrorInternal(err)
	}
	if row == nil {
		return emptyState(), nil
	}
	return *row, nil
}

func (s *service) Put(ctx context.Context, campaignID, entityID, systemID, key string, gm, public *json.RawMessage, userID string) (State, error) {
	if err := validateIDs(systemID, key); err != nil {
		return State{}, err
	}
	gmBytes, err := validateHalf("gm", gm)
	if err != nil {
		return State{}, err
	}
	pubBytes, err := validateHalf("public", public)
	if err != nil {
		return State{}, err
	}
	if err := s.checkEntity(ctx, campaignID, entityID); err != nil {
		return State{}, err
	}
	enabled, err := s.systems.IsSystemEnabled(ctx, campaignID, systemID)
	if err != nil {
		return State{}, apperrorInternal(err)
	}
	if !enabled {
		return State{}, errSystemNotEnabled()
	}

	if err := s.repo.Upsert(ctx, Write{
		CampaignID: campaignID,
		EntityID:   entityID,
		SystemID:   systemID,
		Key:        key,
		GM:         gmBytes,
		Public:     pubBytes,
		UpdatedBy:  userID,
	}); err != nil {
		return State{}, apperrorInternal(err)
	}
	if s.publisher != nil {
		s.publisher.PublishSystemStateUpdated(campaignID, entityID, systemID, key)
	}
	return s.Get(ctx, campaignID, entityID, systemID, key)
}

// checkEntity enforces that the entity exists in the stated campaign. A
// mismatch is reported as NotFound, the same as a missing entity, so the
// response does not reveal that the id exists elsewhere.
func (s *service) checkEntity(ctx context.Context, campaignID, entityID string) error {
	owner, err := s.entities.EntityCampaignID(ctx, entityID)
	if err != nil {
		return err
	}
	if owner != campaignID {
		return errEntityNotFound()
	}
	return nil
}

func validateIDs(systemID, key string) error {
	if !idPattern.MatchString(systemID) {
		return errInvalid("system id must match ^[a-z0-9][a-z0-9_-]{0,63}$")
	}
	if !idPattern.MatchString(key) {
		return errInvalid("state key must match ^[a-z0-9][a-z0-9_-]{0,63}$")
	}
	return nil
}

// validateHalf returns the compacted bytes for a present half, or nil for an
// absent one. A present half must be a JSON object: arrays, scalars and null
// are rejected so a widget can always treat each half as a keyed record.
func validateHalf(name string, raw *json.RawMessage) ([]byte, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := bytes.TrimSpace(*raw)
	if len(trimmed) > MaxHalfBytes {
		return nil, errInvalid(fmt.Sprintf("%s state exceeds %d bytes", name, MaxHalfBytes))
	}
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, errInvalid(name + " state must be a JSON object")
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, trimmed); err != nil {
		return nil, errInvalid(name + " state must be a JSON object")
	}
	return buf.Bytes(), nil
}
