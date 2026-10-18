package app

import (
	"context"
	"slices"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// entryCharacterLists is the slice of the character list service the
// adapter needs: which page types are the campaign's characters.
type entryCharacterLists interface {
	CharacterTypeIDs(ctx context.Context, campaignID string) ([]int, error)
}

// entryCharactersAdapter lets a campaign entry be kept to one character page
// (systems.EntryCharacters) without the systems package importing entities.
// A character is a page of one of the campaign's character types; whoever
// claimed it plays it.
type entryCharactersAdapter struct {
	entities entities.EntityService
	lists    entryCharacterLists
}

func (a entryCharactersAdapter) Character(ctx context.Context, campaignID, entityID string) (*systems.EntryCharacter, error) {
	if entityID == "" {
		return nil, nil
	}
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		// Missing or another campaign's page: kept to nobody.
		return nil, nil
	}
	ids, err := a.lists.CharacterTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(ids, e.EntityTypeID) {
		return nil, nil
	}
	return entryCharacter(e), nil
}

func (a entryCharactersAdapter) CharacterNamed(ctx context.Context, campaignID, name string) (*systems.EntryCharacter, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	ids, err := a.lists.CharacterTypeIDs(ctx, campaignID)
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	found, err := a.entities.SearchRoster(ctx, campaignID, ids, entities.RosterSearch{Query: name, Limit: 50}, permissions.RoleOwner, "")
	if err != nil {
		return nil, err
	}
	var match *entities.Entity
	for i := range found {
		if !strings.EqualFold(strings.TrimSpace(found[i].Name), name) {
			continue
		}
		if match != nil {
			return nil, nil // two characters share the name: ambiguous
		}
		match = &found[i]
	}
	if match == nil {
		return nil, nil
	}
	return entryCharacter(match), nil
}

func entryCharacter(e *entities.Entity) *systems.EntryCharacter {
	c := &systems.EntryCharacter{ID: e.ID, Name: e.Name}
	if e.OwnerUserID != nil {
		c.OwnerUserID = *e.OwnerUserID
	}
	return c
}
