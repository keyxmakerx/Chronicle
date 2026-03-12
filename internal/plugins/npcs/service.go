// service.go contains business logic for the NPC gallery. Resolves the
// campaign's character entity type and delegates listing to the repository.
package npcs

import (
	"context"
	"fmt"
)

// EntityTypeFinder resolves entity types by slug for a campaign.
// Implemented by the entities.EntityService — injected to avoid circular imports.
type EntityTypeFinder interface {
	FindCharacterTypeID(ctx context.Context, campaignID string) (int, error)
}

// TagLister fetches tags for a set of entity IDs in batch.
// Implemented by tags.TagService — injected to decorate NPC cards with tags.
type TagLister interface {
	ListTagsForEntities(ctx context.Context, entityIDs []string) (map[string][]TagInfo, error)
}

// TagInfo holds tag display data returned by TagLister.
type TagInfo struct {
	ID    int
	Name  string
	Slug  string
	Color string
}

// NPCService handles business logic for the NPC gallery.
type NPCService interface {
	// ListNPCs returns revealed character entities for the NPC gallery.
	ListNPCs(ctx context.Context, campaignID string, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error)

	// CountNPCs returns the number of visible NPCs for badge/nav display.
	CountNPCs(ctx context.Context, campaignID string, role int, userID string) (int, error)
}

// npcService implements NPCService.
type npcService struct {
	repo       NPCRepository
	typeFinder EntityTypeFinder
	tagLister  TagLister
}

// NewNPCService creates a new NPC service.
func NewNPCService(repo NPCRepository, typeFinder EntityTypeFinder) NPCService {
	return &npcService{repo: repo, typeFinder: typeFinder}
}

// SetTagLister injects the tag batch fetcher for card decoration.
func (s *npcService) SetTagLister(tl TagLister) {
	s.tagLister = tl
}

// ListNPCs resolves the character type and queries for revealed NPCs.
func (s *npcService) ListNPCs(ctx context.Context, campaignID string, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error) {
	typeID, err := s.typeFinder.FindCharacterTypeID(ctx, campaignID)
	if err != nil {
		return nil, 0, fmt.Errorf("resolving character type: %w", err)
	}

	cards, total, err := s.repo.ListRevealed(ctx, campaignID, typeID, role, userID, opts)
	if err != nil {
		return nil, 0, err
	}

	// Decorate with tags if available.
	if s.tagLister != nil && len(cards) > 0 {
		ids := make([]string, len(cards))
		for i := range cards {
			ids[i] = cards[i].ID
		}
		tagMap, err := s.tagLister.ListTagsForEntities(ctx, ids)
		if err == nil {
			for i := range cards {
				if infos, ok := tagMap[cards[i].ID]; ok {
					for _, t := range infos {
						cards[i].Tags = append(cards[i].Tags, NPCTagInfo(t))
					}
				}
			}
		}
	}

	return cards, total, nil
}

// CountNPCs resolves the character type and returns the revealed count.
func (s *npcService) CountNPCs(ctx context.Context, campaignID string, role int, userID string) (int, error) {
	typeID, err := s.typeFinder.FindCharacterTypeID(ctx, campaignID)
	if err != nil {
		return 0, fmt.Errorf("resolving character type: %w", err)
	}
	return s.repo.CountRevealed(ctx, campaignID, typeID, role, userID)
}
