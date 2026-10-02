// service.go contains business logic for the Armory gallery. Resolves the
// campaign's item-category entity types and delegates listing to the repository.
package armory

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// ItemTypeFinder resolves item-category entity types for a campaign.
// Implemented by the entities.EntityService — injected to avoid circular imports.
type ItemTypeFinder interface {
	FindItemTypeIDs(ctx context.Context, campaignID string) ([]int, error)
	FindItemTypes(ctx context.Context, campaignID string) ([]ItemTypeInfo, error)
}

// EntityVisibilityFilter resolves which of a set of entity IDs a viewer
// (role + userID) may see, applying the entities plugin's own canonical
// visibility policy (default is_private, custom per-subject grants, tag
// grants). Wraps entities.EntityService.FilterViewableEntityIDs — the SAME
// method sessions and the relations widget use — so the Armory gallery
// never hand-rolls its own copy of that predicate, which used to miss
// visibility='custom' items (is_private untouched by SetEntityPermissions)
// and leave them listed to Players and anonymous visitors.
type EntityVisibilityFilter interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// TagLister fetches tags for a set of entity IDs in batch.
// Implemented by tags.TagService — injected to decorate item cards with tags.
// includeDmOnly must be false for viewers below Scribe so GM-only tags never
// reach a player's card.
type TagLister interface {
	ListTagsForEntities(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]TagInfo, error)
}

// TagInfo holds tag display data returned by TagLister.
type TagInfo struct {
	ID    int
	Name  string
	Slug  string
	Color string
}

// ArmoryService handles business logic for the Armory gallery.
type ArmoryService interface {
	// ListItems returns item entities for the Armory gallery.
	ListItems(ctx context.Context, campaignID string, role int, userID string, opts ItemListOptions) ([]ItemCard, int, error)

	// CountItems returns the number of visible items for badge/nav display.
	CountItems(ctx context.Context, campaignID string, role int, userID string) (int, error)

	// GetItemTypes returns all item-category entity types for filter dropdowns.
	GetItemTypes(ctx context.Context, campaignID string) ([]ItemTypeInfo, error)

	// SetTagLister injects the tag batch fetcher that decorates item cards.
	SetTagLister(tl TagLister)
}

// armoryService implements ArmoryService.
type armoryService struct {
	repo             ArmoryRepository
	typeFinder       ItemTypeFinder
	tagLister        TagLister
	entityVisibility EntityVisibilityFilter
}

// NewArmoryService creates a new Armory service. entityVisibility is the
// canonical visibility gate (see EntityVisibilityFilter), required so a
// Player/anonymous viewer's list can never fall back to "everything
// visible" — see visibleItemIDs's fail-closed branch.
func NewArmoryService(repo ArmoryRepository, typeFinder ItemTypeFinder, entityVisibility EntityVisibilityFilter) ArmoryService {
	return &armoryService{repo: repo, typeFinder: typeFinder, entityVisibility: entityVisibility}
}

// SetTagLister injects the tag batch fetcher for card decoration.
func (s *armoryService) SetTagLister(tl TagLister) {
	s.tagLister = tl
}

// ListItems resolves item types, narrows the campaign's matching items to
// what the viewer may see, and returns one page of cards. The total is the
// size of that SAME narrowed set (see visibleItemIDs), so a filtered list
// and its count can never disagree — an inflated count is itself a leak.
func (s *armoryService) ListItems(ctx context.Context, campaignID string, role int, userID string, opts ItemListOptions) ([]ItemCard, int, error) {
	typeIDs, err := s.typeFinder.FindItemTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, 0, fmt.Errorf("resolving item types: %w", err)
	}

	// No item types configured — return empty rather than error.
	if len(typeIDs) == 0 {
		return nil, 0, nil
	}

	visibleIDs, err := s.visibleItemIDs(ctx, campaignID, typeIDs, role, userID, opts)
	if err != nil {
		return nil, 0, err
	}
	total := len(visibleIDs)
	if total == 0 {
		return nil, 0, nil
	}

	pageIDs := paginateIDs(visibleIDs, opts.Offset(), opts.PerPage)
	cards, err := s.repo.GetItemCardsByIDs(ctx, campaignID, pageIDs)
	if err != nil {
		return nil, 0, err
	}

	// Decorate with tags if available.
	if s.tagLister != nil && len(cards) > 0 {
		ids := make([]string, len(cards))
		for i := range cards {
			ids[i] = cards[i].ID
		}
		tagMap, err := s.tagLister.ListTagsForEntities(ctx, ids, role >= permissions.RoleScribe)
		if err == nil {
			for i := range cards {
				if infos, ok := tagMap[cards[i].ID]; ok {
					for _, t := range infos {
						cards[i].Tags = append(cards[i].Tags, ItemTagInfo(t))
					}
				}
			}
		}
	}

	return cards, total, nil
}

// CountItems resolves item types and returns the visible count, computed
// from the exact same visibleItemIDs path ListItems uses — never a separate
// SQL COUNT that could drift from what the list actually shows.
func (s *armoryService) CountItems(ctx context.Context, campaignID string, role int, userID string) (int, error) {
	typeIDs, err := s.typeFinder.FindItemTypeIDs(ctx, campaignID)
	if err != nil {
		return 0, fmt.Errorf("resolving item types: %w", err)
	}
	if len(typeIDs) == 0 {
		return 0, nil
	}
	visibleIDs, err := s.visibleItemIDs(ctx, campaignID, typeIDs, role, userID, ItemListOptions{})
	if err != nil {
		return 0, err
	}
	return len(visibleIDs), nil
}

// visibleItemIDs returns the item entity IDs matching opts' type/search/tag/
// instance filters, narrowed to the viewer's visibility. Callers pass a
// co-DM in as a promoted RoleOwner via cc.VisibilityRole() upstream, so
// only a true OWNER is unrestricted here.
//
// Everyone below Owner, Scribes included, is narrowed through the SAME
// canonical visibility policy the entities plugin itself applies
// (EntityVisibilityFilter), so this gallery can't disagree with the
// entity page about what is hidden.
func (s *armoryService) visibleItemIDs(ctx context.Context, campaignID string, typeIDs []int, role int, userID string, opts ItemListOptions) ([]string, error) {
	allIDs, err := s.repo.ListItemIDs(ctx, campaignID, typeIDs, opts)
	if err != nil {
		return nil, err
	}
	// Bypass at OWNER, not Scribe: the canonical policy (entities/
	// repository.go's visibilityFilter) returns an empty predicate only
	// for role >= RoleOwner. For a Scribe it still evaluates, and a
	// visibility='custom' entity is NOT automatically visible without an
	// actual matching grant.
	if role >= permissions.RoleOwner || len(allIDs) == 0 {
		return allIDs, nil
	}

	if s.entityVisibility == nil {
		// Fail CLOSED: with no way to check visibility, a Player/anonymous
		// viewer must see nothing rather than everything.
		return nil, nil
	}
	viewable, err := s.entityVisibility.FilterViewableEntityIDs(ctx, campaignID, allIDs, role, userID)
	if err != nil {
		return nil, fmt.Errorf("filtering item visibility: %w", err)
	}
	filtered := make([]string, 0, len(allIDs))
	for _, id := range allIDs {
		if viewable[id] {
			filtered = append(filtered, id)
		}
	}
	return filtered, nil
}

// paginateIDs slices a sorted id list to the [offset, offset+perPage) window,
// clamped to the slice bounds. perPage <= 0 returns the whole remainder.
func paginateIDs(ids []string, offset, perPage int) []string {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(ids) {
		return nil
	}
	end := len(ids)
	if perPage > 0 && offset+perPage < end {
		end = offset + perPage
	}
	return ids[offset:end]
}

// GetItemTypes returns item-category entity types for the type filter dropdown.
func (s *armoryService) GetItemTypes(ctx context.Context, campaignID string) ([]ItemTypeInfo, error) {
	return s.typeFinder.FindItemTypes(ctx, campaignID)
}
