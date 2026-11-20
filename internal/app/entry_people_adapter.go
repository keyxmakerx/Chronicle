package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// entryPeopleAdapter finds the campaign's characters by what their kind
// fields hold (systems.EntryPeople), without the systems package importing
// entities. Every read goes through the entity service with the reader's
// own role and account, so a page the reader may not see never appears.
type entryPeopleAdapter struct {
	entities entities.EntityService
	lists    entryCharacterLists
}

// peoplePageSize is the entity service's largest page.
const peoplePageSize = 100

func (a entryPeopleAdapter) People(ctx context.Context, campaignID, fieldKey string, r systems.PeopleReader, limit int) ([]systems.EntryPerson, error) {
	ids, err := a.lists.CharacterTypeIDs(ctx, campaignID)
	if err != nil || len(ids) == 0 || fieldKey == "" {
		return nil, err
	}
	seen := map[string]bool{}
	scanned := 0
	var out []systems.EntryPerson
	for _, typeID := range ids {
		for page := 1; scanned < limit; page++ {
			list, total, err := a.entities.List(ctx, campaignID, typeID, r.Role, r.UserID,
				entities.ListOptions{Page: page, PerPage: peoplePageSize, Sort: "name"})
			if err != nil {
				return nil, err
			}
			for i := range list {
				scanned++
				if seen[list[i].ID] {
					continue
				}
				seen[list[i].ID] = true
				if p := entryPerson(&list[i], fieldKey, r.UserID); p != nil {
					out = append(out, *p)
				}
			}
			if len(list) < peoplePageSize || page*peoplePageSize >= total {
				break
			}
		}
	}
	return out, nil
}

func (a entryPeopleAdapter) Person(ctx context.Context, campaignID, entityID, fieldKey string, r systems.PeopleReader) (*systems.EntryPerson, error) {
	if entityID == "" || fieldKey == "" {
		return nil, nil
	}
	e, err := a.entities.GetByID(ctx, entityID)
	if err != nil || e == nil || e.CampaignID != campaignID {
		// Missing or another campaign's page reads as no hero at all.
		return nil, nil
	}
	ids, err := a.lists.CharacterTypeIDs(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(ids, e.EntityTypeID) {
		return nil, nil
	}
	perm, err := a.entities.CheckEntityAccess(ctx, e.ID, r.Role, r.UserID)
	if err != nil || perm == nil || !perm.CanView {
		return nil, nil
	}
	return entryPerson(e, fieldKey, r.UserID), nil
}

// entryPerson reads what a character's field holds and the traits it bought
// (the hero creator keeps those beside the field as a JSON list of names);
// nil when the field is empty.
func entryPerson(e *entities.Entity, fieldKey, readerID string) *systems.EntryPerson {
	value, _ := e.FieldsData[fieldKey].(string)
	if strings.TrimSpace(value) == "" {
		return nil
	}
	p := &systems.EntryPerson{ID: e.ID, Name: e.Name, Value: strings.TrimSpace(value)}
	if e.OwnerUserID != nil && *e.OwnerUserID != "" {
		p.Hero = true
		p.Mine = readerID != "" && *e.OwnerUserID == readerID
	}
	if raw, ok := e.FieldsData[fieldKey+"_choices_json"].(string); ok && raw != "" {
		var names []string
		if json.Unmarshal([]byte(raw), &names) == nil {
			p.Traits = names
		}
	}
	return p
}
