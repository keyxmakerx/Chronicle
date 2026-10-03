// service.go contains business logic for the NPC gallery. Resolves the
// campaign's character entity type and delegates listing to the repository.
package npcs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// EntityTypeFinder resolves which of a campaign's entity types hold NPCs and
// monsters. Implemented by an adapter over entities.EntityService (injected to
// avoid circular imports). An empty result means the campaign has none.
type EntityTypeFinder interface {
	FindCharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error)
}

// EntityVisibilityFilter resolves which of a set of entity IDs a viewer
// (role + userID) may see, applying the entities plugin's own canonical
// visibility policy (default is_private, custom per-subject grants, tag
// grants). Wraps entities.EntityService.FilterViewableEntityIDs — the SAME
// method sessions and the relations widget use — so the NPC gallery never
// hand-rolls its own copy of that predicate: a `role < 2 AND is_private =
// false` check would miss a visibility='custom' NPC.
type EntityVisibilityFilter interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// TagLister fetches tags for a set of entity IDs in batch.
// Implemented by tags.TagService — injected to decorate NPC cards with tags.
type TagLister interface {
	// includeDmOnly is false for player-facing lists so GM-only tags never decorate or filter them.
	ListTagsForEntities(ctx context.Context, entityIDs []string, includeDmOnly bool) (map[string][]TagInfo, error)
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

	// ListTags returns the distinct tags on the NPCs this viewer can see, for
	// the list's tag filter. Derived from the visibility-narrowed set so a tag
	// that only sits on hidden NPCs is never offered.
	ListTags(ctx context.Context, campaignID string, role int, userID string, includeDmOnly bool) ([]NPCTagInfo, error)

	// SetTagLister injects the batch tag fetcher; without it cards carry no tags
	// and the tag filter is empty.
	SetTagLister(tl TagLister)
}

// npcService implements NPCService.
type npcService struct {
	repo             NPCRepository
	typeFinder       EntityTypeFinder
	tagLister        TagLister
	entityVisibility EntityVisibilityFilter
}

// NewNPCService creates a new NPC service. entityVisibility is the canonical
// visibility gate (see EntityVisibilityFilter) — required so a Player/
// anonymous viewer's list can never fall back to "everything visible" by
// construction; see visibleNPCIDs's fail-closed branch.
func NewNPCService(repo NPCRepository, typeFinder EntityTypeFinder, entityVisibility EntityVisibilityFilter) NPCService {
	return &npcService{repo: repo, typeFinder: typeFinder, entityVisibility: entityVisibility}
}

// SetTagLister injects the tag batch fetcher for card decoration.
func (s *npcService) SetTagLister(tl TagLister) {
	s.tagLister = tl
}

// ListNPCs resolves the character types, narrows the campaign's matching
// characters to what the viewer may see, and returns one page of cards.
// Returns an empty list if the campaign has no character entity types.
// The total is the size of that SAME narrowed set (see visibleNPCIDs), so a
// filtered list and its count can never disagree (ADR-055 rule 3: an
// inflated count is itself a leak).
func (s *npcService) ListNPCs(ctx context.Context, campaignID string, role int, userID string, opts NPCListOptions) ([]NPCCard, int, error) {
	typeIDs, err := s.characterTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, 0, err
	}
	if len(typeIDs) == 0 {
		return nil, 0, nil
	}

	visibleIDs, err := s.visibleNPCIDs(ctx, campaignID, typeIDs, role, userID, opts)
	if err != nil {
		return nil, 0, err
	}
	total := len(visibleIDs)
	if total == 0 {
		return nil, 0, nil
	}

	pageIDs := paginateIDs(visibleIDs, opts.Offset(), opts.PerPage)
	cards, err := s.repo.GetNPCCardsByIDs(ctx, campaignID, pageIDs)
	if err != nil {
		return nil, 0, err
	}

	// Decorate with tags if available.
	if s.tagLister != nil && len(cards) > 0 {
		ids := make([]string, len(cards))
		for i := range cards {
			ids[i] = cards[i].ID
		}
		tagMap, err := s.tagLister.ListTagsForEntities(ctx, ids, opts.IncludeDmOnlyTags)
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

// CountNPCs resolves the character types and returns the revealed count,
// computed from the exact same visibleNPCIDs path ListNPCs uses — never a
// separate SQL COUNT that could drift from what the list actually shows.
// Returns 0 if the campaign has no character entity types.
func (s *npcService) CountNPCs(ctx context.Context, campaignID string, role int, userID string) (int, error) {
	typeIDs, err := s.characterTypeIDs(ctx, campaignID)
	if err != nil {
		return 0, err
	}
	if len(typeIDs) == 0 {
		return 0, nil
	}
	visibleIDs, err := s.visibleNPCIDs(ctx, campaignID, typeIDs, role, userID, NPCListOptions{})
	if err != nil {
		return 0, err
	}
	return len(visibleIDs), nil
}

// ListTags aggregates the tags carried by the viewer's visible NPCs, sorted by
// name. With no tag lister wired it returns nothing (the filter just hides).
func (s *npcService) ListTags(ctx context.Context, campaignID string, role int, userID string, includeDmOnly bool) ([]NPCTagInfo, error) {
	if s.tagLister == nil {
		return nil, nil
	}
	typeIDs, err := s.characterTypeIDs(ctx, campaignID)
	if err != nil || len(typeIDs) == 0 {
		return nil, err
	}
	ids, err := s.visibleNPCIDs(ctx, campaignID, typeIDs, role, userID, NPCListOptions{})
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	tagMap, err := s.tagLister.ListTagsForEntities(ctx, ids, includeDmOnly)
	if err != nil {
		return nil, fmt.Errorf("listing NPC tags: %w", err)
	}
	seen := map[int]bool{}
	var out []NPCTagInfo
	for _, infos := range tagMap {
		for _, t := range infos {
			if !seen[t.ID] {
				seen[t.ID] = true
				out = append(out, NPCTagInfo(t))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// characterTypeIDs resolves the campaign's NPC/monster entity types. A 404
// from the finder means none exist yet: an empty gallery, not an error.
func (s *npcService) characterTypeIDs(ctx context.Context, campaignID string) ([]int, error) {
	ids, err := s.typeFinder.FindCharacterTypeIDs(ctx, campaignID)
	if err != nil {
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == 404 {
			return nil, nil
		}
		return nil, fmt.Errorf("resolving character types: %w", err)
	}
	return ids, nil
}

// visibleNPCIDs returns the character entity IDs matching opts' search/tag
// filters for characterTypeIDs, narrowed to the viewer's visibility.
//
// Only an OWNER is unrestricted here (role >= permissions.RoleOwner).
// Everyone below Owner, Scribes included, is narrowed through the SAME
// canonical visibility policy the entities plugin itself applies
// (EntityVisibilityFilter), so this gallery can't disagree with the entity
// page about what is hidden.
func (s *npcService) visibleNPCIDs(ctx context.Context, campaignID string, characterTypeIDs []int, role int, userID string, opts NPCListOptions) ([]string, error) {
	allIDs, err := s.repo.ListRevealedIDs(ctx, campaignID, characterTypeIDs, opts)
	if err != nil {
		return nil, err
	}
	// Bypass at OWNER, not Scribe: visibilityFilter (entities/repository.go)
	// returns an empty predicate only for role >= RoleOwner. A Scribe still
	// gets evaluated, and a visibility='custom' entity is not automatically
	// visible to them without a matching grant.
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
		return nil, fmt.Errorf("filtering NPC visibility: %w", err)
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
