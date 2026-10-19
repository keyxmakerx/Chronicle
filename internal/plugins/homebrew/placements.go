package homebrew

import "context"

// Placement is one place a recipe shows up. Where is "category" for a
// category whose every page shows it, placed with the Customizer.
type Placement struct {
	Where string `json:"where"`
	Name  string `json:"name"`
}

// PlacementFinder reads where page layouts place a recipe. It is implemented
// in internal/app over the entities plugin, which owns the layouts.
type PlacementFinder interface {
	Placements(ctx context.Context, campaignID, recipeID string) ([]Placement, error)
}
