// adventures.go defines the Adventures category: a default category only the
// DM team can see, with the trial-page layout as its preset.
//
// "DM team only" is a property of the category, not of each page, so a page
// in it stays hidden from players whatever its own visibility says (a custom
// grant or a tag grant included). The marker is the category's preset
// category, which already round-trips through campaign export and import, so
// no schema change is needed. See ADR-068.

package entities

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// errNoKindsInAdventures is why a category cannot be made a kind of Adventures:
// kinds are listed under their parent for everyone, so a player-visible kind
// would hang off a category players cannot see.
const errNoKindsInAdventures = "Adventures is for the DM team only, so it cannot have kinds of its own"

// PresetCategoryAdventures marks the Adventures category. Anything carrying it
// is invisible to a viewer below Scribe, in every read path.
const PresetCategoryAdventures = "adventures"

// SlugAdventures is the slug the seeded category gets.
const SlugAdventures = "adventures"

// DMOnly reports whether the category is for the DM team only. It is nil safe
// so a caller holding a failed lookup fails closed in its own way.
func (et *EntityType) DMOnly() bool {
	return et != nil && et.PresetCategory != nil && *et.PresetCategory == PresetCategoryAdventures
}

// ViewerSeesType reports whether a viewer with the given visibility role may
// know the category exists: every category except a DM-only one is open to
// everyone who can view the campaign, and a DM-only one needs Scribe or above
// (an owner and a member with DM access arrive here as Owner).
func ViewerSeesType(et *EntityType, role int) bool {
	return !et.DMOnly() || role >= permissions.RoleScribe
}

// TypesForViewer drops the categories the viewer may not know exist. The DM
// team gets the input slice back as is; anyone else gets a new slice.
func TypesForViewer(types []EntityType, role int) []EntityType {
	if role >= permissions.RoleScribe {
		return types
	}
	out := make([]EntityType, 0, len(types))
	for i := range types {
		if ViewerSeesType(&types[i], role) {
			out = append(out, types[i])
		}
	}
	return out
}

// dmOnlyTypeClause is the SQL fragment that keeps pages of a DM-only category
// away from a viewer below Scribe. It sits in visibilityFilter, the one
// predicate every list, search, count, mention, relation and ancestor read
// shares, so a page of the category cannot leak past a read that remembers to
// apply the page's own visibility but not the category's. It takes one
// argument, the viewer's role, and expects the entities table aliased as e.
const dmOnlyTypeClause = ` AND (? >= 2 OR NOT EXISTS (
		SELECT 1 FROM entity_types dmt
		WHERE dmt.id = e.entity_type_id AND dmt.preset_category = '` + PresetCategoryAdventures + `'
	))`

// withAdventures appends the Adventures category after the last type of a
// seed list, so every genre and the standard set start with it.
func withAdventures(types []EntityType) []EntityType {
	max := 0
	for _, t := range types {
		if t.SortOrder > max {
			max = t.SortOrder
		}
	}
	return append(types, adventuresType(max+1))
}

// adventuresType is the category as seeded for a new campaign or added with
// the one-click button for an existing one.
func adventuresType(sortOrder int) EntityType {
	preset := PresetCategoryAdventures
	return EntityType{
		Slug: SlugAdventures, Name: "Adventure", NamePlural: "Adventures",
		Icon: "fa-dungeon", Color: "#b45309", SortOrder: sortOrder,
		PresetCategory: &preset, IsDefault: true, Enabled: true,
		Fields: []FieldDefinition{
			{Key: "sessions", Label: "Sessions", Type: "text", Section: "Basics"},
			{Key: "party_level", Label: "Party level", Type: "text", Section: "Basics"},
			{Key: "status", Label: "Status", Type: "select", Section: "Basics",
				Options: []string{"Planned", "Running", "Finished", "Shelved"}},
		},
		Layout: AdventureLayout(),
	}
}

// EnsureAdventures adds the Adventures category to a campaign that has none,
// and returns the one it has otherwise. Existing campaigns get it through the
// owner's button in Customize rather than by a migration: it is a data change,
// and the owner decides whether the category appears in their sidebar.
func (s *entityService) EnsureAdventures(ctx context.Context, campaignID string) (*EntityType, bool, error) {
	found, err := s.types.ListByPresetCategory(ctx, campaignID, PresetCategoryAdventures)
	if err != nil {
		return nil, false, apperror.NewInternal(fmt.Errorf("looking for the Adventures category: %w", err))
	}
	if len(found) > 0 {
		return &found[0], false, nil
	}

	// A category the owner made by hand may already hold the slug.
	slug, err := s.generateEntityTypeSlug(ctx, campaignID, "Adventures")
	if err != nil {
		return nil, false, apperror.NewInternal(fmt.Errorf("generating the Adventures slug: %w", err))
	}
	maxOrder, err := s.types.MaxSortOrder(ctx, campaignID)
	if err != nil {
		return nil, false, apperror.NewInternal(fmt.Errorf("querying max sort order: %w", err))
	}
	et := adventuresType(maxOrder + 1)
	et.CampaignID, et.Slug = campaignID, slug

	if err := s.types.Create(ctx, &et); err != nil {
		// Two presses at once: the slug's unique key refuses the second, and
		// the category the first made is the answer.
		if again, lerr := s.types.ListByPresetCategory(ctx, campaignID, PresetCategoryAdventures); lerr == nil && len(again) > 0 {
			return &again[0], false, nil
		}
		return nil, false, apperror.NewInternal(fmt.Errorf("creating the Adventures category: %w", err))
	}
	slog.Info("adventures category added", slog.String("campaign_id", campaignID), slog.Int("entity_type_id", et.ID))

	s.events.PublishEntityTypeEvent("created", campaignID, &et)
	if err := s.sidebarAdder.AddEntityTypeToSidebar(ctx, campaignID, et.ID); err != nil {
		// The category exists; the owner can place its row in the menu by hand.
		slog.Warn("failed to auto-add the Adventures category to the sidebar",
			slog.Int("entity_type_id", et.ID), slog.String("campaign_id", campaignID), slog.Any("error", err))
	}
	return &et, true, nil
}

// AdventureLayout is the trial page: the story on the left with the at-a-glance
// facts beside it, the page's own sub-pages (its trials) under the story, then
// a tile section of the table's tools. The Read-aloud, Steps and Table blocks
// hold text a DM types in place; their ids are the keys that text is stored
// under, so they are fixed here and stay stable for every page of the
// category.
func AdventureLayout() EntityTypeLayout {
	return EntityTypeLayout{
		Rows: []TemplateRow{
			{
				ID: "row-adv-story",
				Columns: []TemplateColumn{
					{
						ID: "col-adv-story", Width: 8,
						Blocks: []TemplateBlock{
							{ID: "blk-title", Type: "title"},
							{ID: "blk-entry", Type: "entry"},
							{ID: "blk-trials", Type: BlockSubPages},
						},
					},
					{
						ID: "col-adv-facts", Width: 4,
						Blocks: []TemplateBlock{
							{ID: "blk-image", Type: "image"},
							{ID: "blk-attrs", Type: "attributes"},
							{ID: "blk-details", Type: "details"},
						},
					},
				},
			},
			{
				ID:      "tiles-adv-table",
				Kind:    tilegrid.KindTiles,
				Columns: []TemplateColumn{},
				Tiles: []tilegrid.Tile{
					{ID: "tile-adv-read", Type: "read_aloud", Size: "2x2"},
					{ID: "tile-adv-steps", Type: "page_steps", Size: "2x2"},
					{ID: "tile-adv-table", Type: "page_table", Size: "4x2"},
				},
			},
		},
	}
}
