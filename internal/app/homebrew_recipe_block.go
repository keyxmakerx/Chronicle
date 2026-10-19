package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"sort"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// recipeBlockType is the page block that shows one saved Homebrew Workshop
// recipe. The Customizer offers one palette entry per recipe, carrying its
// id as the block's config.
const recipeBlockType = "homebrew_recipe"

// recipeBlockParts are the kinds of part the block can show on a page; a
// recipe with none of them is not offered in the palette.
var recipeBlockParts = map[string]bool{"tracker": true, "clock": true}

// recipeIDPattern is the shape of a recipe id stored in a block's config,
// which is editable layout JSON and so is checked before it is used.
var recipeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// recipeOwnerPrefix is the owner prefix of every tracker kind a recipe keeps,
// shared by the kind sync and the block so the block shows exactly those.
func recipeOwnerPrefix(recipeID string) string { return "homebrew:" + recipeID + ":" }

var recipeBlockSizes = []string{"1x1", "2x1", "2x2", "4x1"}

// registerRecipeBlock adds the recipe block to page layouts. It shows the
// recipe's trackers and clocks through the trackers widget's owned view, so
// placing it on a category's layout puts them on every page of it, each
// with the "who sees it" its part was given in the Workshop. The bare type
// is never offered on its own: its palette entries come from
// recipeBlockMetas.
func registerRecipeBlock(blockRegistry *entities.BlockRegistry) {
	blockRegistry.Register(entities.BlockMeta{
		Type: recipeBlockType, Label: "Homebrew recipe", Icon: "fa-beer-mug-empty",
		Description: "A homebrew recipe's trackers and clocks",
		Contexts:    []string{"template"},
		TileSizes:   recipeBlockSizes,
		Preset:      map[string]string{"recipe_id": ""},
		PresetOnly:  true,
		Group:       "Homebrew",
	}, func(rc entities.BlockRenderContext) templ.Component {
		id := entities.BlockConfigString(rc.Block.Config, "recipe_id")
		if rc.CC == nil || rc.CC.Campaign == nil || rc.Entity == nil || !recipeIDPattern.MatchString(id) {
			return templ.NopComponent
		}
		return trackers.OwnedMount(rc.CC.Campaign.ID, rc.Entity.ID, rc.CSRFToken, recipeOwnerPrefix(id))
	})
}

// recipeBlockMetas lists one palette entry per saved recipe the block can
// show. The block-types route is the campaign owner's, so the recipes are
// read as a Director.
func recipeBlockMetas(ctx context.Context, svc homebrew.Service, campaignID string) []entities.BlockMeta {
	list, err := svc.List(ctx, campaignID, homebrew.Viewer{Director: true})
	if err != nil {
		slog.Warn("homebrew recipes left out of the widget list", slog.Any("error", err))
		return nil
	}
	var out []entities.BlockMeta
	for _, r := range list {
		shows := false
		for _, k := range r.PartKinds {
			shows = shows || recipeBlockParts[k]
		}
		if !shows {
			continue
		}
		out = append(out, entities.BlockMeta{
			Type: recipeBlockType, Label: r.Name, Icon: "fa-beer-mug-empty",
			Description: "Trackers and clocks from this homebrew recipe",
			Contexts:    []string{"template"},
			TileSizes:   recipeBlockSizes,
			Preset:      map[string]string{"recipe_id": r.ID},
			Group:       "Homebrew",
		})
	}
	return out
}

// recipePlacements implements homebrew.PlacementFinder over the categories'
// page layouts, which the entities plugin owns.
type recipePlacements struct {
	entities entities.EntityService
}

func (p *recipePlacements) Placements(ctx context.Context, campaignID, recipeID string) ([]homebrew.Placement, error) {
	types, err := p.entities.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	out := []homebrew.Placement{}
	for _, et := range types {
		if !layoutPlacesRecipe(et.Layout, recipeID) {
			continue
		}
		name := et.NamePlural
		if name == "" {
			name = et.Name
		}
		out = append(out, homebrew.Placement{Where: "category", Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// layoutPlacesRecipe says whether a layout holds the recipe's block anywhere:
// in a column, a tile, or inside an older container's config.
func layoutPlacesRecipe(layout entities.EntityTypeLayout, recipeID string) bool {
	raw, err := json.Marshal(layout)
	if err != nil {
		return false
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return false
	}
	var walk func(v any) bool
	walk = func(v any) bool {
		switch n := v.(type) {
		case map[string]any:
			if n["type"] == recipeBlockType {
				if cfg, ok := n["config"].(map[string]any); ok && cfg["recipe_id"] == recipeID {
					return true
				}
			}
			for _, c := range n {
				if walk(c) {
					return true
				}
			}
		case []any:
			for _, c := range n {
				if walk(c) {
					return true
				}
			}
		}
		return false
	}
	return walk(tree)
}
