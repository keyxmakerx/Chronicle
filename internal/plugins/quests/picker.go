package quests

import (
	"context"
	"strings"
)

// MaxPickerResults caps a picker answer.
const MaxPickerResults = 25

// Picker kinds. A quest is any page, since any page can hold a quest sheet.
const (
	PickPage  = "page"
	PickQuest = "quest"
	PickMap   = "map"
	// PickCharacter lists who rewards can be handed to: DM only.
	PickCharacter = "character"
)

// minPickerQuery matches the entity search's own minimum.
const minPickerQuery = 2

// PickerService searches what a viewer may pin.
type PickerService interface {
	Search(ctx context.Context, campaignID string, v Viewer, kind, q string) ([]PickerItem, error)
}

type pickerService struct {
	entities   EntityDirectory
	maps       MapDirectory
	characters CharacterDirectory
}

// NewPickerService builds the service. characters may be nil, in which case
// the character kind answers an empty list.
func NewPickerService(entities EntityDirectory, maps MapDirectory, characters CharacterDirectory) PickerService {
	return &pickerService{entities: entities, maps: maps, characters: characters}
}

func (s *pickerService) Search(ctx context.Context, campaignID string, v Viewer, kind, q string) ([]PickerItem, error) {
	if !v.IsPlayer() {
		return nil, errForbidden("only campaign members may search")
	}
	q = strings.TrimSpace(q)
	out := []PickerItem{}
	switch kind {
	case PickMap:
		maps, err := s.maps.ListMaps(ctx, campaignID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		needle := strings.ToLower(q)
		for _, m := range maps {
			if needle == "" || strings.Contains(strings.ToLower(m.Name), needle) {
				out = append(out, PickerItem{ID: m.ID, Name: m.Name})
				if len(out) == MaxPickerResults {
					break
				}
			}
		}
	case PickCharacter:
		// Only the DM hands out rewards, so only the DM gets the party list.
		if !v.IsDM {
			return nil, errForbidden("only the DM can hand out rewards")
		}
		if s.characters == nil {
			return out, nil
		}
		chars, err := s.characters.ListCharacters(ctx, campaignID, v.VisibilityRole, v.UserID)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		// The party is the claimed characters; a campaign where nobody has
		// claimed one yet still gets every character to choose from.
		claimed := 0
		for _, c := range chars {
			if c.Player != "" {
				claimed++
			}
		}
		needle := strings.ToLower(q)
		for _, c := range chars {
			if claimed > 0 && c.Player == "" {
				continue
			}
			if needle != "" && !strings.Contains(strings.ToLower(c.Name), needle) {
				continue
			}
			out = append(out, PickerItem{ID: c.ID, Name: c.Name, Player: c.Player})
		}
	case PickPage, PickQuest:
		if len([]rune(q)) < minPickerQuery {
			return out, nil
		}
		// Ask for extra: the visibility re-check below can drop hits.
		found, err := s.entities.Search(ctx, campaignID, q, v.VisibilityRole, v.UserID, MaxPickerResults*2)
		if err != nil {
			return nil, apperrorInternal(err)
		}
		ids := make([]string, len(found))
		for i, e := range found {
			ids[i] = e.ID
		}
		_, ok, err := gate{entities: s.entities}.viewable(ctx, campaignID, v, ids)
		if err != nil {
			return nil, err
		}
		for _, e := range found {
			if !ok[e.ID] {
				continue
			}
			out = append(out, PickerItem{ID: e.ID, Name: e.Name, TypeName: e.TypeName, ImagePath: e.ImagePath})
			if len(out) == MaxPickerResults {
				break
			}
		}
	default:
		return nil, errInvalid("kind must be page, quest, map or character")
	}
	return out, nil
}
