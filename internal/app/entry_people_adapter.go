package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// entryPeopleAdapter finds the campaign's characters by what their kind
// fields hold (systems.EntryPeople), without the systems package importing
// entities. Every read goes through the entity service with the reader's
// own role and account, so a page the reader may not see never appears, and
// a GM-only or owner-only field is read only for those it is served to.
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
	types := fieldTypes{}
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
				hidden, err := a.hiddenFields(ctx, &list[i], r, types)
				if err != nil {
					return nil, err
				}
				if p := entryPerson(&list[i], fieldKey, r.UserID, hidden); p != nil {
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
	hidden, err := a.hiddenFields(ctx, e, r, fieldTypes{})
	if err != nil {
		return nil, err
	}
	return entryPerson(e, fieldKey, r.UserID, hidden), nil
}

// fieldTypes caches character types across one walk of the roster, which
// holds many pages of few types.
type fieldTypes map[int]*entities.EntityType

// hiddenFields is the set of the page's fields the reader is not served: the
// entities plugin's own rule, so a kind value or a bought trait kept in a
// GM-only or owner-only field never reaches the rules pages, hover cards or
// "told the players" lists of someone the page itself would not show it to.
// A category that cannot be read hides every field, never none.
func (a entryPeopleAdapter) hiddenFields(ctx context.Context, e *entities.Entity, r systems.PeopleReader, types fieldTypes) (map[string]struct{}, error) {
	canSeeGM := r.Role >= int(campaigns.RoleOwner)
	if canSeeGM {
		return nil, nil
	}
	et, ok := types[e.EntityTypeID]
	if !ok {
		var err error
		if et, err = a.entities.GetEntityTypeByID(ctx, e.EntityTypeID); err != nil {
			return nil, err
		}
		types[e.EntityTypeID] = et
	}
	if et == nil {
		return allFieldsHidden, nil
	}
	return entities.RestrictedFieldKeys(e, et, false, e.IsOwnedBy(r.UserID)), nil
}

// allFieldsHidden stands for a page whose category could not be read.
var allFieldsHidden = map[string]struct{}{"": {}}

// entryPerson reads what a character's field holds and the traits it bought
// (the hero creator keeps those beside the field as a JSON list of names);
// nil when the field is empty or hidden from the reader. Traits hidden from
// the reader are left out even when the kind itself is not.
func entryPerson(e *entities.Entity, fieldKey, readerID string, hidden map[string]struct{}) *systems.EntryPerson {
	if isHidden(hidden, fieldKey) {
		return nil
	}
	value, _ := e.FieldsData[fieldKey].(string)
	if strings.TrimSpace(value) == "" {
		return nil
	}
	p := &systems.EntryPerson{ID: e.ID, Name: e.Name, Value: strings.TrimSpace(value)}
	if e.OwnerUserID != nil && *e.OwnerUserID != "" {
		p.Hero = true
		p.Mine = readerID != "" && *e.OwnerUserID == readerID
	}
	if raw, ok := e.FieldsData[fieldKey+"_choices_json"].(string); ok && raw != "" && !isHidden(hidden, fieldKey+"_choices_json") {
		var names []string
		if json.Unmarshal([]byte(raw), &names) == nil {
			p.Traits = names
		}
	}
	return p
}

func isHidden(hidden map[string]struct{}, key string) bool {
	if _, all := hidden[""]; all {
		return true
	}
	_, ok := hidden[key]
	return ok
}
