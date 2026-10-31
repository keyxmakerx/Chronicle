package crafting

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// AIRecipe is a recipe as the AI connection writes it, by ids: the
// connection resolves the AI's names first. Method is a template key that
// lays the steps over the materials, as the editor's template picker does;
// "" keeps a stored recipe's steps (each still pointing at the same
// material) and gives a new one none. Rules, looks and colours are the
// Director's, so an AI save keeps them.
type AIRecipe struct {
	// ID is the recipe to change; "" makes a new one.
	ID           string
	Name         string
	Method       string
	Materials    []Material
	Result       Result
	Goal         int
	Stat         string
	Prerequisite string
	Source       string
	Cost         Cost
	Who          string
	Known        []string
}

// SaveAIRecipe checks a recipe an AI wrote and, unless check is set, saves
// it. A new recipe arrives as a draft, so players never see it until a
// Director reads it in the editor and approves it; changing a recipe keeps
// it approved or not. check runs every check a save runs, the name included,
// so the AI review's plan and the save refuse the same things.
func (s *service) SaveAIRecipe(ctx context.Context, campaignID string, v Viewer, in AIRecipe, check bool) (*Recipe, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	var stored *Recipe
	if in.ID != "" {
		r, err := s.load(ctx, campaignID, in.ID)
		if err != nil {
			return nil, err
		}
		stored = r
	}
	ri := RecipeInput{Name: in.Name, Materials: in.Materials, Result: in.Result, Goal: in.Goal, Stat: in.Stat,
		Prerequisite: in.Prerequisite, Source: in.Source, Cost: in.Cost, Who: in.Who, Known: in.Known}
	if stored != nil {
		ri.Template, ri.Bench, ri.Colors, ri.Version = stored.Template, stored.Bench, stored.Colors, stored.Version
		ri.Steps = KeepStepMaterials(stored.Materials, stored.Steps, ri.Materials)
	}
	if in.Method != "" {
		tags, err := s.repo.ListTags(ctx, campaignID)
		if err != nil {
			return nil, errInternal(err)
		}
		var ok bool
		if ri, ok = LayTemplate(ri, in.Method, tags); !ok {
			return nil, errInvalid(fmt.Sprintf("there is no method %q", in.Method))
		}
	}
	f, err := s.facts(ctx, campaignID, ri, stored)
	if err != nil {
		return nil, err
	}
	r, err := cleanRecipe(ri, f)
	if err != nil {
		return nil, err
	}
	if check {
		return nil, s.nameFree(ctx, campaignID, in.ID, r.Name)
	}
	if stored == nil {
		r.ID, r.Draft = s.newID(), true
		r.Rules, r.Looks = []Rule{}, map[string]*Look{}
		if err := s.repo.CreateRecipe(ctx, campaignID, v.UserID, r); err != nil {
			if errors.Is(err, errDuplicate) {
				return nil, errNameTaken("recipe")
			}
			return nil, errInternal(err)
		}
		return s.load(ctx, campaignID, r.ID)
	}
	r.ID = stored.ID
	ok, err := s.repo.UpdateRecipe(ctx, campaignID, v.UserID, r, stored.Version)
	if err != nil {
		if errors.Is(err, errDuplicate) {
			return nil, errNameTaken("recipe")
		}
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	return s.load(ctx, campaignID, stored.ID)
}

// nameFree refuses a name another recipe already has, ignoring case as the
// table's unique key does.
func (s *service) nameFree(ctx context.Context, campaignID, id, name string) error {
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		return errInternal(err)
	}
	for _, r := range all {
		if r.ID != id && strings.EqualFold(r.Name, name) {
			return errNameTaken("recipe")
		}
	}
	return nil
}

// CheckDrops runs SaveDrops' checks without saving.
func (s *service) CheckDrops(ctx context.Context, campaignID, pageID string, v Viewer, in DropInput) error {
	if err := directorOnly(v); err != nil {
		return err
	}
	if err := s.spoilsReady(); err != nil {
		return err
	}
	page, err := s.projects.Pages.Page(ctx, campaignID, strings.TrimSpace(pageID))
	if err != nil {
		return errInternal(err)
	}
	if page == nil {
		return errNotFound("page not found")
	}
	rules, _ := s.systemRules(ctx, campaignID)
	_, err = s.cleanDrops(ctx, campaignID, page.ID, in, rules)
	return err
}

// LayTemplate lays a template's steps over a recipe, as the editor's
// applyTemplate does: each step that wants a material takes the first one
// whose tags fit, or adds an "any" line for it. tags are each item's
// crafting tags. ok is false for a template the library doesn't have.
func LayTemplate(in RecipeInput, key string, tags map[string][]string) (RecipeInput, bool) {
	var t *Template
	for i := range Templates {
		if Templates[i].Key == key {
			t = &Templates[i]
		}
	}
	if t == nil {
		return in, false
	}
	out := in
	out.Materials = append([]Material{}, in.Materials...)
	out.Steps = make([]Step, 0, len(t.Steps))
	for _, ts := range t.Steps {
		s := Step{T: ts.T, To: ts.To, Turns: ts.Turns, Secs: ts.Secs, N: ts.N}
		if len(ts.Want) > 0 {
			idx := -1
			for i, m := range out.Materials {
				if materialFits(m, ts.Want, tags) {
					idx = i
					break
				}
			}
			if idx < 0 {
				out.Materials = append(out.Materials, Material{Tag: ts.Want[0], Qty: 1})
				idx = len(out.Materials) - 1
			}
			s.Material = &idx
		}
		out.Steps = append(out.Steps, s)
	}
	out.Template, out.Bench = t.Key, t.Bench
	return out, true
}

// materialFits reports whether a material carries one of the wanted tags:
// an "any" line its own tag, an item the tags it has.
func materialFits(m Material, want []string, tags map[string][]string) bool {
	have := tags[m.ItemID]
	if m.Tag != "" {
		have = []string{m.Tag}
	}
	for _, h := range have {
		if oneOfStr(want, h) {
			return true
		}
	}
	return false
}

// KeepStepMaterials points each step at the same material in a recipe's new
// list of materials: the same item, or the same "any" tag. A step whose
// material is gone loses it, and the editor asks for one.
func KeepStepMaterials(old []Material, steps []Step, materials []Material) []Step {
	out := make([]Step, len(steps))
	for i, s := range steps {
		out[i] = s
		out[i].Material = nil
		if s.Material == nil || *s.Material < 0 || *s.Material >= len(old) {
			continue
		}
		was := old[*s.Material]
		for j, m := range materials {
			if (was.ItemID != "" && m.ItemID == was.ItemID) || (was.Tag != "" && m.Tag == was.Tag) {
				n := j
				out[i].Material = &n
				break
			}
		}
	}
	return out
}
