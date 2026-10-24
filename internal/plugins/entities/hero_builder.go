package entities

import (
	"context"
	"encoding/json"
	"slices"
)

// hero_builder.go shapes what the Characters page's hero builder needs: the
// viewer's own heroes with their current picks, and whether they may start a
// new one. The builder reads its choices from the creator plan
// (HeroPlanAPI) and saves through the fields door in identity_edit.go, so
// nothing here grants a write; it only decides what the page shows.

// HeroBuilderView is the builder's mount data on the Characters page. nil
// hides the builder.
type HeroBuilderView struct {
	CampaignID string
	CSRFToken  string
	// CanCreate shows the sentence for a new hero.
	CanCreate bool
	// TypeName and ForSelf describe a new hero: the page type it becomes and
	// whether it will be the viewer's own.
	TypeName string
	ForSelf  bool
	Heroes   []BuilderHero
}

// BuilderHero is one of the viewer's heroes as the builder sees it.
type BuilderHero struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Values holds the hero's text fields, so each blank shows what is
	// already chosen. Hidden carries the keys the viewer may not see or
	// change (fields marked GM-only); their values are left out.
	Values map[string]string `json:"values"`
	Hidden []string          `json:"hidden"`
	// Sheet is true when the game system draws this hero's page as its
	// sheet, so the link can promise one; otherwise it opens the plain page.
	Sheet bool `json:"sheet"`
}

// builderHeroes picks the viewer's heroes the builder can fill in: their own
// pages of a type the owner lists as characters. For a viewer below Scribe,
// GM-only fields are left out of Values and listed in Hidden, since the
// fields door refuses them and the owner may not read them.
func builderHeroes(ctx context.Context, mine []CastMember, charTypeIDs []int, types []EntityType, staff bool) []BuilderHero {
	byType := make(map[int]*EntityType, len(types))
	for i := range types {
		byType[types[i].ID] = &types[i]
	}
	out := []BuilderHero{}
	for _, m := range mine {
		e := m.Entity
		if !slices.Contains(charTypeIDs, e.EntityTypeID) {
			continue
		}
		hidden := map[string]bool{}
		if !staff {
			var typeFields []FieldDefinition
			if t := byType[e.EntityTypeID]; t != nil {
				typeFields = t.Fields
			}
			for _, f := range MergeFields(typeFields, e.FieldOverrides) {
				if f.GMOnly {
					hidden[f.Key] = true
				}
			}
		}
		h := BuilderHero{ID: e.ID, Name: e.Name, Values: map[string]string{}, Hidden: []string{}, Sheet: hasEntityShowRenderer(ctx, byType[e.EntityTypeID])}
		for k, v := range e.FieldsData {
			if hidden[k] {
				continue
			}
			if s, ok := v.(string); ok {
				h.Values[k] = s
			}
		}
		for k := range hidden {
			h.Hidden = append(h.Hidden, k)
		}
		slices.Sort(h.Hidden)
		out = append(out, h)
	}
	return out
}

// HeroesJSON is the heroes as the mount's data attribute carries them.
func (v *HeroBuilderView) HeroesJSON() string {
	b, err := json.Marshal(v.Heroes)
	if err != nil {
		return "[]"
	}
	return string(b)
}
