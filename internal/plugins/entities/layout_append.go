package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AppendBlockToCategoryLayout puts one block in a full-width section at the
// end of a category's page layout, and of each of its named layouts, so every
// page of the category shows it. It goes through the same checks as a
// Customizer save. A layout that already holds the block is left alone; added
// is false when none needed it.
func (s *entityService) AppendBlockToCategoryLayout(ctx context.Context, campaignID string, typeID int, blockType string) (bool, error) {
	if s.blockRegistry == nil || !s.blockRegistry.IsValidForContext(blockType, "template") {
		return false, apperror.NewBadRequest("that cannot be put on a page")
	}
	et, err := s.types.FindByID(ctx, typeID)
	if err != nil {
		return false, err
	}
	if et.CampaignID != campaignID {
		return false, apperror.NewNotFound("category not found")
	}

	var named PageLayouts
	if s.pageLayouts != nil {
		named = s.pageLayouts.ForType(ctx, et)
	}
	main, mainAdded := withBlockAtEnd(et.Layout, blockType)
	namedAdded := false
	for i := range named.Extra {
		l, ok := withBlockAtEnd(named.Extra[i].Layout, blockType)
		named.Extra[i].Layout = l
		namedAdded = namedAdded || ok
	}
	if !mainAdded && !namedAdded {
		return false, nil
	}

	// Check both before writing either, so a full layout never leaves the
	// category half done.
	if mainAdded {
		if err := ValidateLayout(main, s.isValidBlockType, s.isSingletonBlockType, s.tileSizesFor(ctx, typeID, main)); err != nil {
			return false, err
		}
	}
	if namedAdded {
		if err := s.checkPageLayouts(ctx, typeID, named); err != nil {
			return false, err
		}
	}
	if mainAdded {
		if err := s.UpdateEntityTypeLayout(ctx, typeID, main); err != nil {
			return false, err
		}
	}
	if namedAdded {
		if err := s.pageLayouts.SaveTypeLayouts(ctx, campaignID, typeID, named); err != nil {
			return false, err
		}
	}
	return true, nil
}

// withBlockAtEnd returns the layout with the block in a new last section, and
// whether it changed. A container keeps its children in free-form config, so
// the whole layout is searched, not only its columns.
func withBlockAtEnd(l EntityTypeLayout, blockType string) (EntityTypeLayout, bool) {
	if raw, err := json.Marshal(l); err == nil && strings.Contains(string(raw), `"type":"`+blockType+`"`) {
		return l, false
	}
	rows := append([]TemplateRow{}, l.Rows...)
	id := generateUUID()
	rows = append(rows, TemplateRow{ID: "row-" + id, Columns: []TemplateColumn{
		{ID: "col-" + id, Width: gridWidth, Blocks: []TemplateBlock{{ID: fmt.Sprintf("blk-%s", id), Type: blockType}}},
	}})
	l.Rows = rows
	return l, true
}

// PlacementOption is one category the "Put on a page" chooser offers.
type PlacementOption struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	NamePlural string `json:"namePlural"`
	// Suggested marks the category the block is made for.
	Suggested bool `json:"suggested"`
	// Placed is true when every layout of the category already holds the block.
	Placed bool `json:"placed"`
}

// suggestedHomes is the category each placeable block is made for, by the
// slug a standard category gets or the preset it carries.
var suggestedHomes = map[string][]string{
	"quest_board":   {"quest", "quests"},
	"notice_boards": {"location", "locations"},
	"trackers":      {PresetCategoryAdventures},
	"page_tests":    {PresetCategoryAdventures},
}

// BlockPlacementOptions lists the campaign's top-level categories for putting
// a block on their pages: the suggested one first, then the rest in menu
// order. Categories that are switched off are left out.
func (s *entityService) BlockPlacementOptions(ctx context.Context, campaignID, blockType string) ([]PlacementOption, error) {
	if s.blockRegistry == nil || !s.blockRegistry.IsValidForContext(blockType, "template") {
		return nil, apperror.NewBadRequest("that cannot be put on a page")
	}
	types, err := s.types.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	homes := suggestedHomes[blockType]
	var first, rest []PlacementOption
	for i := range types {
		et := &types[i]
		if !et.Enabled || et.ParentTypeID != nil {
			continue
		}
		o := PlacementOption{ID: et.ID, Name: et.Name, NamePlural: et.NamePlural, Placed: s.layoutsHoldBlock(ctx, et, blockType)}
		for _, h := range homes {
			if et.Slug == h || (et.PresetCategory != nil && *et.PresetCategory == h) {
				o.Suggested = true
			}
		}
		if o.Suggested {
			first = append(first, o)
		} else {
			rest = append(rest, o)
		}
	}
	return append(first, rest...), nil
}

// layoutsHoldBlock reports whether the category's main layout and each named
// layout already hold the block.
func (s *entityService) layoutsHoldBlock(ctx context.Context, et *EntityType, blockType string) bool {
	if _, add := withBlockAtEnd(et.Layout, blockType); add {
		return false
	}
	if s.pageLayouts == nil {
		return true
	}
	for _, l := range s.pageLayouts.ForType(ctx, et).Extra {
		if _, add := withBlockAtEnd(l.Layout, blockType); add {
			return false
		}
	}
	return true
}
