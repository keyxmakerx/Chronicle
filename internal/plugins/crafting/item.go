package crafting

import (
	"context"
	"sort"
	"strings"
)

// maxItemDrops caps the drop lists read for one item's "Comes from"; a
// campaign with more says so rather than leaving some out silently.
const maxItemDrops = 500

// ItemCrafting is what an Armory item's page shows about crafting: the
// recipes that make it ("How it's made") and, for a material, its kinds,
// the recipes that ask for it and the drop lists that give it ("In
// crafting"). A Director gets everything, drafts included. A player gets
// only the book pages they could already read in their recipe book, and
// never the item's kinds or its drop lists.
type ItemCrafting struct {
	ItemID   string `json:"itemId"`
	Name     string `json:"name"`
	Director bool   `json:"director"`
	// Kinds are the item's crafting tags; Director only.
	Kinds []string `json:"kinds"`
	// MadeBy are the recipes that make the item, as the editor reads them;
	// Director only.
	MadeBy []Recipe `json:"madeBy"`
	// Uses are the recipes that ask for the item by name or by one of its
	// kinds.
	Uses []ItemUse `json:"uses"`
	// Drops are the drop lists that give the item; Director only.
	Drops    []ItemDrop `json:"drops"`
	DropsCut bool       `json:"dropsCut,omitempty"`
	// Book are the pages a player reads for the recipes that make it.
	Book []BookPage `json:"book"`
}

// ItemUse is one recipe that asks for an item.
type ItemUse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Qty  int    `json:"qty"`
	// ByKind is the kind the recipe asks for when it doesn't name the item.
	ByKind string `json:"byKind,omitempty"`
	Draft  bool   `json:"draft,omitempty"`
}

// ItemDrop is one drop list line that gives an item.
type ItemDrop struct {
	PageID string `json:"pageId"`
	Page   string `json:"page"`
	Mode   string `json:"mode"`
	Qty    int    `json:"qty,omitempty"`
}

// ItemCrafting reads what crafting says about one Armory item. A player who
// can't see the item gets a 404, the same as an item that isn't there, so
// the route never says what makes a page they don't know of.
func (s *service) ItemCrafting(ctx context.Context, campaignID, itemID string, v Viewer) (*ItemCrafting, error) {
	e, err := s.dir.Entity(ctx, campaignID, itemID)
	if err != nil {
		return nil, errInternal(err)
	}
	if e == nil || !e.IsItem {
		return nil, errNotFound("item not found")
	}
	if !v.Director {
		if s.projects.Pages == nil {
			return nil, errNotFound("item not found")
		}
		ok, err := s.projects.Pages.CanSee(ctx, campaignID, itemID, v.VisibilityRole, v.UserID)
		if err != nil {
			return nil, errInternal(err)
		}
		if !ok {
			return nil, errNotFound("item not found")
		}
	}
	out := &ItemCrafting{ItemID: e.ID, Name: e.Name, Director: v.Director, Kinds: []string{}, MadeBy: []Recipe{}, Uses: []ItemUse{}, Drops: []ItemDrop{}, Book: []BookPage{}}
	tags, err := s.repo.ListTags(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	kinds := tags[itemID]
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}

	// A player reads only what their recipe book already shows them.
	readable := map[string]BookPage{}
	if !v.Director {
		book, err := s.Book(ctx, campaignID, v)
		if err != nil {
			return nil, err
		}
		for _, p := range book {
			readable[p.ID] = p
		}
	}
	for _, r := range all {
		page, canRead := readable[r.ID]
		if !v.Director && !canRead {
			continue
		}
		if r.Result.ItemID == itemID {
			if v.Director {
				out.MadeBy = append(out.MadeBy, r)
			} else {
				out.Book = append(out.Book, page)
			}
		}
		if u, ok := useOf(r, itemID, kinds); ok {
			out.Uses = append(out.Uses, u)
		}
	}
	if !v.Director {
		return out, nil
	}
	out.Kinds = append(out.Kinds, kinds...)
	sort.Strings(out.Kinds)
	if out.Drops, out.DropsCut, err = s.itemDrops(ctx, campaignID, itemID); err != nil {
		return nil, err
	}
	return out, nil
}

// useOf says how a recipe asks for an item: by its id first, then by the
// first of its kinds a tag slot names.
func useOf(r Recipe, itemID string, kinds []string) (ItemUse, bool) {
	u := ItemUse{ID: r.ID, Name: r.Name, Draft: r.Draft}
	for _, m := range r.Materials {
		if m.ItemID == itemID {
			u.Qty, u.ByKind = m.Qty, ""
			return u, true
		}
	}
	for _, m := range r.Materials {
		if m.Tag != "" && oneOfStr(kinds, m.Tag) {
			u.Qty, u.ByKind = m.Qty, m.Tag
			return u, true
		}
	}
	return u, false
}

// itemDrops lists the drop list lines that give an item, named by the page
// that drops them; a page that is gone is left out.
func (s *service) itemDrops(ctx context.Context, campaignID, itemID string) ([]ItemDrop, bool, error) {
	out := []ItemDrop{}
	if s.projects.Spoils == nil || s.projects.Pages == nil {
		return out, false, nil
	}
	lists, err := s.projects.Spoils.ListDrops(ctx, campaignID, maxItemDrops+1)
	if err != nil {
		return nil, false, errInternal(err)
	}
	cut := len(lists) > maxItemDrops
	if cut {
		lists = lists[:maxItemDrops]
	}
	for _, d := range lists {
		for _, l := range d.Lines {
			if l.ItemID != itemID {
				continue
			}
			p, err := s.projects.Pages.Page(ctx, campaignID, d.PageID)
			if err != nil {
				return nil, false, errInternal(err)
			}
			if p == nil {
				break
			}
			out = append(out, ItemDrop{PageID: d.PageID, Page: p.Name, Mode: l.Mode, Qty: l.Qty})
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Page) < strings.ToLower(out[j].Page) })
	return out, cut, nil
}

// TeachParty teaches a recipe, found by name, to every hero in the party:
// the claimed characters the Director can see. It answers the line the
// Homebrew drawer shows. A recipe everyone knows needs no teaching, and a
// draft is taught but still hidden until it is approved.
func (s *service) TeachParty(ctx context.Context, campaignID string, v Viewer, name string) (string, error) {
	if err := directorOnly(v); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return "", errInternal(err)
	}
	var r *Recipe
	for i := range all {
		if strings.EqualFold(all[i].Name, name) {
			r = &all[i]
			break
		}
	}
	if r == nil {
		return "", errInvalid("there is no crafting recipe called " + quote(name))
	}
	if r.Who != "learned" {
		return "Everyone already knows " + r.Name + ".", nil
	}
	party, err := s.dir.Party(ctx, campaignID, v.VisibilityRole, v.UserID)
	if err != nil {
		return "", errInternal(err)
	}
	if len(party) == 0 {
		return "No hero is claimed yet, so nobody learned " + r.Name + ".", nil
	}
	names := make([]string, 0, len(party))
	for _, h := range party {
		if err := s.repo.Learn(ctx, campaignID, r.ID, h.ID, v.UserID); err != nil {
			return "", errInternal(err)
		}
		names = append(names, h.Name)
	}
	line := "Taught " + r.Name + " to " + joinAnd(names) + "."
	if r.Draft {
		line += " It is still a draft, so players see it once you approve it."
	}
	return line, nil
}

// KnownRecipes keeps the ids that are recipes of this campaign, in order and
// without repeats, so a Workshop part never points at a deleted recipe.
func (s *service) KnownRecipes(ctx context.Context, campaignID string, v Viewer, ids []string) ([]string, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	have := make(map[string]bool, len(all))
	for _, r := range all {
		have[r.ID] = true
	}
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if have[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

func quote(s string) string { return "“" + s + "”" }

// joinAnd lists names as a sentence does: "A", "A and B", "A, B and C".
func joinAnd(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}
