package crafting

import (
	"context"
	"log/slog"

	"github.com/keyxmakerx/chronicle/internal/madeitems"
)

// A made item's picture comes from what went into it: the recipe's
// materials, in order, give the result shape its colours and one material
// gives the picture on its label. The shape comes from the item made. Only
// shapes, colours and picture keys are kept, never which items they came
// from, because what a project took is the Director's.

// ItemLook is a made copy's picture, drawn by static/js/widgets/craft_results.js.
type ItemLook struct {
	Shape  string    `json:"shape"`
	Body   string    `json:"body"`
	Trim   string    `json:"trim"`
	Detail string    `json:"detail"`
	Mark   *LookMark `json:"mark,omitempty"`
}

// LookMark is the material picture a look shows.
type LookMark struct {
	Picture string `json:"picture"`
	Tint    string `json:"tint"`
}

// lookDefaults are the drawn shapes' own colours, for what no material
// gives: body, trim, detail. They match DEFAULTS in craft_results.js; the
// painted items carry their own in internal/madeitems. Looks stored before
// the painted set keep these shapes.
var lookDefaults = map[string][3]string{
	"potion": {"#c3223a", "#9a7650", "#f4ecd6"},
	"vial":   {"#4a9a6a", "#9a7650", "#f4ecd6"},
	"blade":  {"#8c96a3", "#c9a24a", "#7a4a2a"},
	"shield": {"#3f9a84", "#8c96a3", "#a8743e"},
	"ring":   {"#4a7ad8", "#d1ad52", "#d1ad52"},
	"bundle": {"#7a6a9a", "#c9a54a", "#efe6d2"},
}

// shapeByPicture maps the item's own material picture to a painted item,
// when the Director gave the made item one.
var shapeByPicture = map[string]string{
	"potion": "bottle", "flask": "bottle", "jar": "bottle",
	"vial": "bottle", "drop": "bottle",
	"ring": "ring", "gem": "ring",
}

// lookShape is what a made item is drawn as: one of the painted items
// (internal/madeitems), picked by its own picture, else by a word in its
// name, else the bundle, which the painted set draws as a pouch.
func lookShape(own *MaterialPicture, name string) string {
	if own != nil {
		if s, ok := shapeByPicture[own.Picture]; ok {
			return s
		}
	}
	if k := madeitems.FromName(name); k != "" {
		return k
	}
	return "bundle"
}

// composeLook builds a look from a shape and the materials' pictures in
// recipe order: the first colours the body, the second the trim, the third
// the detail, and the second (or else the first) is the mark.
func composeLook(shape string, mats []MaterialPicture) *ItemLook {
	// A painted item's own colours come first: ring and shield are both.
	d, ok := lookDefaults[shape]
	if it, painted := madeitems.Get(shape); painted {
		d, ok = [3]string{it.Body, it.Trim, it.Detail}, true
	}
	if !ok {
		shape, d = "bundle", lookDefaults["bundle"]
	}
	var use []MaterialPicture
	for _, m := range mats {
		if usable(m) {
			use = append(use, m)
		}
	}
	l := &ItemLook{Shape: shape, Body: d[0], Trim: d[1], Detail: d[2]}
	for i, f := range []*string{&l.Body, &l.Trim, &l.Detail} {
		if i < len(use) {
			*f = use[i].Tint
		}
	}
	switch {
	case len(use) > 1:
		l.Mark = &LookMark{Picture: use[1].Picture, Tint: use[1].Tint}
	case len(use) == 1:
		l.Mark = &LookMark{Picture: use[0].Picture, Tint: use[0].Tint}
	}
	return l
}

// madeLook is the look of what project p makes, read from its recipe as it
// stands now. A recipe that was deleted, or a read that fails, gives no
// look, and the made copy shows the plain icon.
func (s *service) madeLook(ctx context.Context, p *Project) *ItemLook {
	if p.RecipeID == "" || p.ResultItemID == "" {
		return nil
	}
	r, ok, err := s.repo.GetRecipe(ctx, p.CampaignID, p.RecipeID)
	if err != nil || !ok {
		if err != nil {
			slog.Error("crafting: cannot read the recipe for a made item's picture", slog.String("project_id", p.ID), slog.Any("error", err))
		}
		return nil
	}
	book, err := s.pictureBook(ctx, p.CampaignID)
	if err != nil {
		return nil
	}
	tags, err := s.repo.ListTags(ctx, p.CampaignID)
	if err != nil {
		return nil
	}
	mats := make([]MaterialPicture, 0, len(r.Materials))
	for _, m := range r.Materials {
		if m.ItemID != "" {
			mats = append(mats, book.item(m.ItemID, knownTags(tags[m.ItemID])))
		} else if m.Tag != "" {
			mats = append(mats, book.kind(m.Tag))
		}
	}
	name := p.RecipeName
	if e, err := s.dir.Entity(ctx, p.CampaignID, p.ResultItemID); err == nil && e != nil {
		name = e.Name
	}
	return composeLook(lookShape(book.own(p.ResultItemID), name), mats)
}
