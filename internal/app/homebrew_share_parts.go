package app

import (
	"encoding/json"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// The Share hooks below are each owner's say over what leaves a campaign in a
// shared recipe (homebrew.PartKind.Share). Every one decodes strictly, so a
// package that smuggles a setting the owner doesn't have is refused rather
// than passed along, and every one leaves data it has nothing to say about
// exactly as it was, because the Store checks that a stripped part strips to
// itself.

// The picks a dropped link asks the Director to make. Each owner's Check
// reads the same string back (homebrew.Waiting) for exactly that field.
const (
	pickPlacePage       = "a place page"
	pickCraftingRecipes = "its crafting recipes"
	pickCreatureRecipe  = "a creature recipe page"
	pickAddonRecipes    = "its add-on recipe pages"
	pickMarkedCreatures = "the creatures it marks"
	pickAncestry        = "an ancestry"
)

// keepWaiting is what an owner does with a marker the sender wrote: it keeps
// it only when the owner would have made it itself (the caller has checked
// that the data really lacks the link), so a marker is always the owner's.
func keepWaiting(waiting []homebrew.Unlinked, pick string) []homebrew.Unlinked {
	for _, w := range waiting {
		if w.Pick == pick {
			return []homebrew.Unlinked{{Pick: pick}}
		}
	}
	return nil
}

// shareFieldPart refuses a field only the Director sees: the field is itself
// the Director's text, so there is nothing of it left to share.
func shareFieldPart(raw json.RawMessage, _ []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error) {
	var p hbFieldPart
	if err := strictJSON(raw, &p); err != nil {
		return nil, nil, err
	}
	if p.Director {
		return nil, nil, apperror.NewValidation("is a field only the Director sees, so it is not shared; make it visible or take it out of the recipe")
	}
	out, err := json.Marshal(p)
	return out, nil, err
}

// shareItemPart drops the fields only the Director sees.
func shareItemPart(raw json.RawMessage, _ []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error) {
	var p hbItemPart
	if err := strictJSON(raw, &p); err != nil {
		return nil, nil, err
	}
	kept := make([]hbField, 0, len(p.Fields))
	for _, f := range p.Fields {
		if !f.Director {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 && len(p.Fields) > 0 {
		return nil, nil, apperror.NewValidation("has only fields the Director sees, so nothing of it is shared")
	}
	p.Fields = kept
	out, err := json.Marshal(p)
	return out, nil, err
}

// shareWordsPart drops the words only the Director sees, and the place: a
// page id means nothing in another campaign.
func shareWordsPart(raw json.RawMessage, waiting []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error) {
	var p hbWordsPart
	if err := strictJSON(raw, &p); err != nil {
		return nil, nil, err
	}
	kept := make([]hbWord, 0, len(p.Words))
	for _, w := range p.Words {
		if !w.Director {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 && len(p.Words) > 0 {
		return nil, nil, apperror.NewValidation("has only words the Director sees, so nothing of it is shared")
	}
	p.Words = kept
	var picks []homebrew.Unlinked
	if p.PlaceID != "" {
		p.PlaceID = ""
		picks = append(picks, homebrew.Unlinked{Pick: pickPlacePage})
	} else if p.Era == "" {
		picks = keepWaiting(waiting, pickPlacePage)
	}
	out, err := json.Marshal(p)
	return out, picks, err
}

// shareCraftingPart drops the crafting recipes, which are held by id.
func shareCraftingPart(raw json.RawMessage, waiting []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error) {
	var d craftingPartData
	if err := strictJSON(raw, &d); err != nil {
		return nil, nil, err
	}
	var picks []homebrew.Unlinked
	if len(d.Recipes) > 0 {
		d.Recipes = nil
		picks = append(picks, homebrew.Unlinked{Pick: pickCraftingRecipes})
	} else {
		picks = keepWaiting(waiting, pickCraftingRecipes)
	}
	out, err := json.Marshal(d)
	return out, picks, err
}

// shareRecipeLink drops the page a creature-recipe part points at.
func shareRecipeLink(raw json.RawMessage, _ []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error) {
	var d recipeLink
	if err := strictJSON(raw, &d); err != nil {
		return nil, nil, err
	}
	// An empty page is never valid, so the link is always waiting to be chosen.
	return json.RawMessage(`{"page":""}`), []homebrew.Unlinked{{Pick: pickCreatureRecipe}}, nil
}

// shareCreatureMark drops every page a mark names; the variants it records
// are the rise action's own work in the sending campaign.
func shareCreatureMark(raw json.RawMessage, waiting []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error) {
	var d creatureMark
	if err := strictJSON(raw, &d); err != nil {
		return nil, nil, err
	}
	// An empty recipe is never valid, so it is always waiting to be chosen;
	// the lists are waiting when they held pages or the sender says so.
	picks := []homebrew.Unlinked{{Pick: pickCreatureRecipe}}
	if len(d.Addons) > 0 {
		picks = append(picks, homebrew.Unlinked{Pick: pickAddonRecipes})
	} else {
		picks = append(picks, keepWaiting(waiting, pickAddonRecipes)...)
	}
	if len(d.Marked) > 0 {
		picks = append(picks, homebrew.Unlinked{Pick: pickMarkedCreatures})
	} else {
		picks = append(picks, keepWaiting(waiting, pickMarkedCreatures)...)
	}
	return json.RawMessage(`{"recipe":""}`), picks, nil
}

// shareAncestry drops the slug: it names an entry of the sending campaign.
func shareAncestry(raw json.RawMessage, _ []homebrew.Unlinked) (json.RawMessage, []homebrew.Unlinked, error) {
	var d struct {
		Slug string `json:"slug"`
	}
	if err := strictJSON(raw, &d); err != nil {
		return nil, nil, err
	}
	// An empty slug is never valid, so the link is always waiting.
	return json.RawMessage(`{"slug":""}`), []homebrew.Unlinked{{Pick: pickAncestry}}, nil
}

// sharePartyEffect refuses an effect only the Director sees: its name and
// text are the Director's, so there is nothing of it to share.
func sharePartyEffect(raw json.RawMessage) (json.RawMessage, error) {
	var a partyEffectArgs
	if err := strictJSON(raw, &a); err != nil {
		return nil, err
	}
	if a.See == effectSeeDirector {
		return nil, apperror.NewValidation("is an effect only the Director sees, so it is not shared; let the party see it or take the step out of the recipe")
	}
	return raw, nil
}
