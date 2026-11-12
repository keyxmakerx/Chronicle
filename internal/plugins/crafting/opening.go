package crafting

import "github.com/keyxmakerx/chronicle/internal/templates/layouts"

// craftingIntros are what the page says about itself for each viewer. The
// crafting widget draws the recipes from the API, so whether the book is empty
// isn't known here: the opening is always the one-line size and the widget
// owns its own empty book. The Director's editors and a player's recipe book
// share the page.
var craftingIntros = layouts.AppIntros{
	Player: "The recipes your characters know.",
	CoDM:   "Recipes, the materials they take, and the benches they are made at.",
	Owner:  "Recipes, the materials they take, and the benches they are made at.",
}

// craftingOpening is the Crafting page's opening. It has no main action: new
// recipes are made inside the widget's editors.
func craftingOpening() layouts.AppOpeningData {
	return layouts.AppOpeningData{Name: "Crafting", Intros: craftingIntros}
}
