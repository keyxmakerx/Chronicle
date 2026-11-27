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

// shareFieldPart refuses a field only the Director sees: the field is itself
// the Director's text, so there is nothing of it left to share.
func shareFieldPart(raw json.RawMessage) (json.RawMessage, []homebrew.Unlinked, error) {
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
func shareItemPart(raw json.RawMessage) (json.RawMessage, []homebrew.Unlinked, error) {
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
func shareWordsPart(raw json.RawMessage) (json.RawMessage, []homebrew.Unlinked, error) {
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
		picks = append(picks, homebrew.Unlinked{Pick: "a place page"})
	}
	out, err := json.Marshal(p)
	return out, picks, err
}

// shareCraftingPart drops the crafting recipes, which are held by id.
func shareCraftingPart(raw json.RawMessage) (json.RawMessage, []homebrew.Unlinked, error) {
	var d craftingPartData
	if err := strictJSON(raw, &d); err != nil {
		return nil, nil, err
	}
	var picks []homebrew.Unlinked
	if len(d.Recipes) > 0 {
		d.Recipes = nil
		picks = append(picks, homebrew.Unlinked{Pick: "its crafting recipes"})
	}
	out, err := json.Marshal(d)
	return out, picks, err
}

// shareRecipeLink drops the page a creature-recipe part points at.
func shareRecipeLink(raw json.RawMessage) (json.RawMessage, []homebrew.Unlinked, error) {
	var d recipeLink
	if err := strictJSON(raw, &d); err != nil {
		return nil, nil, err
	}
	var picks []homebrew.Unlinked
	if d.Page != "" {
		picks = append(picks, homebrew.Unlinked{Pick: "a creature recipe page"})
	}
	return json.RawMessage(`{"page":""}`), picks, nil
}

// shareCreatureMark drops every page a mark names; the variants it records
// are the rise action's own work in the sending campaign.
func shareCreatureMark(raw json.RawMessage) (json.RawMessage, []homebrew.Unlinked, error) {
	var d creatureMark
	if err := strictJSON(raw, &d); err != nil {
		return nil, nil, err
	}
	var picks []homebrew.Unlinked
	if d.Recipe != "" {
		picks = append(picks, homebrew.Unlinked{Pick: "a creature recipe page"})
	}
	if len(d.Addons) > 0 {
		picks = append(picks, homebrew.Unlinked{Pick: "its add-on recipe pages"})
	}
	if len(d.Marked) > 0 {
		picks = append(picks, homebrew.Unlinked{Pick: "the creatures it marks"})
	}
	return json.RawMessage(`{"recipe":""}`), picks, nil
}
