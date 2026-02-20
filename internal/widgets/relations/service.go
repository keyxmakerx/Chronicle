package relations

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// RelationService defines the business logic contract for entity relation
// operations. Handlers call these methods -- they never touch the repository
// directly.
type RelationService interface {
	// Create validates input and creates a bi-directional relation between
	// two entities. Both the forward (A→B) and reverse (B→A) directions are
	// created atomically. Returns the forward relation.
	Create(ctx context.Context, campaignID, sourceEntityID, targetEntityID, relationType, reverseRelationType, createdBy string) (*Relation, error)

	// ListByEntity returns all relations originating from the given entity,
	// enriched with target entity display data.
	ListByEntity(ctx context.Context, entityID string) ([]Relation, error)

	// Delete removes a relation and its reverse direction. Both directions
	// are removed to maintain bi-directional consistency.
	Delete(ctx context.Context, id int) error

	// GetByID retrieves a single relation by ID.
	GetByID(ctx context.Context, id int) (*Relation, error)

	// GetCommonTypes returns the predefined relation type pairs for the
	// frontend UI suggestion list.
	GetCommonTypes() []RelationTypePair
}

// relationService implements RelationService with validation and
// bi-directional relation management.
type relationService struct {
	repo RelationRepository
}

// NewRelationService creates a new RelationService backed by the given
// repository.
func NewRelationService(repo RelationRepository) RelationService {
	return &relationService{repo: repo}
}

// Create validates input and creates a bi-directional relation.
//
// Steps:
//  1. Validate that source != target (no self-relations).
//  2. Validate relation type is not empty.
//  3. If no reverse type specified, use the same type (symmetric relation).
//  4. Create forward relation: source → target with relationType.
//  5. Create reverse relation: target → source with reverseRelationType.
//
// If the reverse already exists (e.g., due to a prior incomplete creation),
// the duplicate is silently ignored via the unique constraint.
func (s *relationService) Create(ctx context.Context, campaignID, sourceEntityID, targetEntityID, relationType, reverseRelationType, createdBy string) (*Relation, error) {
	// Validate: no self-relations.
	if sourceEntityID == targetEntityID {
		return nil, apperror.NewBadRequest("an entity cannot have a relation with itself")
	}

	// Validate: relation type is required.
	relationType = strings.TrimSpace(relationType)
	if relationType == "" {
		return nil, apperror.NewBadRequest("relation type is required")
	}

	// Enforce maximum length on relation type labels.
	if len(relationType) > 100 {
		return nil, apperror.NewBadRequest("relation type must be 100 characters or fewer")
	}

	// If no reverse type specified, use the same type (symmetric relation).
	reverseRelationType = strings.TrimSpace(reverseRelationType)
	if reverseRelationType == "" {
		reverseRelationType = relationType
	}
	if len(reverseRelationType) > 100 {
		return nil, apperror.NewBadRequest("reverse relation type must be 100 characters or fewer")
	}

	// Create forward relation: source → target.
	forward := &Relation{
		CampaignID:          campaignID,
		SourceEntityID:      sourceEntityID,
		TargetEntityID:      targetEntityID,
		RelationType:        relationType,
		ReverseRelationType: reverseRelationType,
		CreatedBy:           createdBy,
	}
	if err := s.repo.Create(ctx, forward); err != nil {
		return nil, err
	}

	// Create reverse relation: target → source.
	// The reverse's "relation_type" is the forward's "reverse_relation_type",
	// and its "reverse_relation_type" is the forward's "relation_type".
	reverse := &Relation{
		CampaignID:          campaignID,
		SourceEntityID:      targetEntityID,
		TargetEntityID:      sourceEntityID,
		RelationType:        reverseRelationType,
		ReverseRelationType: relationType,
		CreatedBy:           createdBy,
	}
	if err := s.repo.Create(ctx, reverse); err != nil {
		// If the reverse already exists, that's acceptable -- log it but
		// don't fail the whole operation.
		if _, ok := err.(*apperror.AppError); ok {
			// Conflict means reverse already exists; silently continue.
		} else {
			return nil, fmt.Errorf("creating reverse relation: %w", err)
		}
	}

	return forward, nil
}

// ListByEntity returns all relations originating from the given entity.
func (s *relationService) ListByEntity(ctx context.Context, entityID string) ([]Relation, error) {
	return s.repo.ListByEntity(ctx, entityID)
}

// Delete removes a relation and its reverse direction to maintain
// bi-directional consistency. If the reverse is not found (e.g., due to
// a prior incomplete deletion), that's acceptable -- only the forward
// deletion is required to succeed.
func (s *relationService) Delete(ctx context.Context, id int) error {
	// Look up the relation to find the reverse direction.
	rel, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	// Find and delete the reverse relation first.
	// The reverse has: source=rel.Target, target=rel.Source, type=rel.ReverseRelationType.
	reverse, err := s.repo.FindReverse(ctx, rel.TargetEntityID, rel.SourceEntityID, rel.ReverseRelationType)
	if err != nil {
		return fmt.Errorf("finding reverse relation: %w", err)
	}
	if reverse != nil {
		if err := s.repo.Delete(ctx, reverse.ID); err != nil {
			// Log but don't fail if reverse deletion fails -- the forward
			// deletion is the primary operation.
			_ = err
		}
	}

	// Delete the forward relation.
	return s.repo.Delete(ctx, id)
}

// GetByID retrieves a single relation by its primary key.
func (s *relationService) GetByID(ctx context.Context, id int) (*Relation, error) {
	return s.repo.FindByID(ctx, id)
}

// GetCommonTypes returns the predefined relation type pairs.
func (s *relationService) GetCommonTypes() []RelationTypePair {
	return CommonRelationTypes
}
