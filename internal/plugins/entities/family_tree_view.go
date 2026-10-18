package entities

import (
	"context"
	"fmt"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// Presentation helpers for the Family tree block (family_tree_block.templ).
// The wire shape below is what static/js/family_tree.js reads; it is built
// from a FamilyTree the service already filtered for the viewer, so nothing
// here makes a permission decision.

// familyWireParent is a person's link up to a parent on the wire.
type familyWireParent struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	DM   bool   `json:"dm,omitempty"`
}

// familyWirePerson is one relative on the wire.
type familyWirePerson struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Sub      string             `json:"sub"`
	Img      string             `json:"img,omitempty"`
	URL      string             `json:"url"`
	Parents  []familyWireParent `json:"parents"`
	Partners []string           `json:"partners"`
	Read     bool               `json:"read"`
	DM       bool               `json:"dm,omitempty"`
}

// familyWire is the whole block's data on the wire.
type familyWire struct {
	Page           string             `json:"page"`
	People         []familyWirePerson `json:"people"`
	CutGenerations bool               `json:"cutGenerations"`
	CutPeople      bool               `json:"cutPeople"`
	Generations    int                `json:"generations"`
	MaxPeople      int                `json:"maxPeople"`
}

// familyWireOf turns a tree into the block's JSON. A picture becomes a signed
// media URL here, where the request context that signs it is at hand.
func familyWireOf(ctx context.Context, campaignID string, t *FamilyTree) familyWire {
	w := familyWire{
		Page:           t.PageID,
		People:         make([]familyWirePerson, 0, len(t.People)),
		CutGenerations: t.CutGenerations,
		CutPeople:      t.CutPeople,
		Generations:    MaxFamilyGenerations,
		MaxPeople:      MaxFamilyPeople,
	}
	for _, p := range t.People {
		wp := familyWirePerson{
			ID: p.ID, Name: p.Name, Sub: p.Type,
			URL:      fmt.Sprintf("/campaigns/%s/entities/%s", campaignID, p.ID),
			Parents:  make([]familyWireParent, 0, len(p.Parents)),
			Partners: append([]string{}, p.Partners...),
			Read:     p.Read, DM: p.DMOnly,
		}
		if p.Image != "" {
			wp.Img = layouts.MediaURL(ctx, p.Image)
		}
		for _, par := range p.Parents {
			wp.Parents = append(wp.Parents, familyWireParent{ID: par.ID, Kind: par.Kind, DM: par.DMOnly})
		}
		w.People = append(w.People, wp)
	}
	return w
}

// familyCountLabel is the resting strip's count: "1 relative", "12 relatives",
// or "At least 60 relatives" when a bound left some out.
func familyCountLabel(t *FamilyTree) string {
	n := t.Relatives()
	switch {
	case t.Cut():
		return fmt.Sprintf("At least %d relatives", n)
	case n == 1:
		return "1 relative"
	}
	return fmt.Sprintf("%d relatives", n)
}

// familyCutNote says which bound kept relatives out, in words a player can
// act on (open a relative's page to follow their side). Empty when none did.
func familyCutNote(t *FamilyTree) string {
	switch {
	case t.CutPeople && t.CutGenerations:
		return fmt.Sprintf("Showing the nearest %d relatives and %d generations each way; the family goes on past that.", MaxFamilyPeople, MaxFamilyGenerations)
	case t.CutPeople:
		return fmt.Sprintf("Showing the nearest %d relatives; the rest are on their own pages.", MaxFamilyPeople)
	case t.CutGenerations:
		return fmt.Sprintf("Showing %d generations each way; the family goes on past that.", MaxFamilyGenerations)
	}
	return ""
}

// familyHasDMOnly reports whether the tree holds anything only the Director
// sees (the service sends such things to no one else).
func familyHasDMOnly(t *FamilyTree) bool {
	for _, p := range t.People {
		if p.DMOnly {
			return true
		}
		for _, par := range p.Parents {
			if par.DMOnly {
				return true
			}
		}
	}
	return false
}

// familyCloseKin lists the page's own parents, partners and children by id, in
// that order, for the resting strip's no-script fallback (the script draws the
// real strip from the same data).
func familyCloseKin(t *FamilyTree) []FamilyPerson {
	byID := map[string]FamilyPerson{}
	for _, p := range t.People {
		byID[p.ID] = p
	}
	var out []FamilyPerson
	seen := map[string]bool{t.PageID: true}
	add := func(id string) {
		if p, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			out = append(out, p)
		}
	}
	if len(t.People) == 0 {
		return nil
	}
	for _, par := range t.People[0].Parents {
		add(par.ID)
	}
	for _, id := range t.People[0].Partners {
		add(id)
	}
	for _, p := range t.People {
		for _, par := range p.Parents {
			if par.ID == t.PageID {
				add(p.ID)
			}
		}
	}
	return out
}

// familyEditURL is the Edit tab's endpoint for a page; empty for anyone who is
// not the Director, so the script has nothing to call.
func familyEditURL(dm bool, campaignID, entityID, suffix string) string {
	if !dm {
		return ""
	}
	if suffix != "" {
		return fmt.Sprintf("/campaigns/%s/entities/%s/family%s", campaignID, entityID, suffix)
	}
	return fmt.Sprintf("/campaigns/%s/entities/%s/family", campaignID, entityID)
}

// familyPagesURL is the prefix a page's own address starts with, so a page the
// Director just added can link to itself before the tree is read again.
func familyPagesURL(dm bool, campaignID string) string {
	if !dm {
		return ""
	}
	return fmt.Sprintf("/campaigns/%s/entities/", campaignID)
}

func familyDigit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// familyCall is one of the block's handlers in static/js/family_tree.js,
// written as an inline IIFE so it works after an HTMX swap of the page (a
// templ script helper or a delegated listener may never run there). fn is
// always one of that file's fixed names, never input.
func familyCall(fn string) templ.ComponentScript {
	return templ.ComponentScript{
		Name:     "entities_family_" + fn,
		Function: "",
		Call: `(function(el,e){` +
			`if(window.ChronicleFamilyTree)window.ChronicleFamilyTree.` + fn + `(el,e);` +
			`})(this,event)`,
	}
}

// familySheetID is the id the open button's aria-controls points at; it
// carries the entity so a page opened in a side peek beside another never
// shares it.
func familySheetID(entityID string) string { return "fm-sheet-" + entityID }
