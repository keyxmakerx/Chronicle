package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

// hbCategories is the slice of the entity service a saved field part needs.
type hbCategories interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	UpdateEntityType(ctx context.Context, id int, input entities.UpdateEntityTypeInput) (*entities.EntityType, error)
}

// hbRuleStore is the slice of the field rules service saved parts need.
type hbRuleStore interface {
	RuleNamed(ctx context.Context, campaignID, name string) (*fieldrules.Rule, error)
	SaveRule(ctx context.Context, campaignID string, in fieldrules.RuleInput, v fieldrules.Viewer) (*fieldrules.Rule, error)
	Words(ctx context.Context, campaignID string, v fieldrules.Viewer) ([]fieldrules.LocalWord, error)
	SaveWord(ctx context.Context, campaignID string, in fieldrules.WordInput, v fieldrules.Viewer) (*fieldrules.LocalWord, error)
	Eras(ctx context.Context, campaignID string, v fieldrules.Viewer) map[int]string
}

// hbSaveAs is who a recipe save acts as: the Workshop stores recipes only
// for a Director, so its saves of fields, rules and words are the Director's.
var hbSaveAs = fieldrules.Viewer{Role: permissions.RoleOwner}

// fieldRuleSync saves what a recipe's field, item and words parts describe:
// fields on a category's sheet, each worked-out field's rule, and local
// words. It only adds and updates. A part taken out of a recipe, or a deleted
// recipe, leaves what it made in place for the Director to remove by hand, so
// tidying a recipe never takes anything off a sheet.
type fieldRuleSync struct {
	cats  hbCategories
	rules hbRuleStore
}

// sheetPlan is what saving a set of field or item parts would write: each
// category's whole field list, and the rules to save first, keyed by the
// field (category id and key) that names them.
type sheetPlan struct {
	fields map[int][]entities.FieldDefinition
	order  []int
	rules  map[hbFieldRef]fieldrules.RuleInput
}

type hbFieldRef struct {
	typeID int
	key    string
}

// sheetFields gathers the fields of field and item parts by category.
func sheetFields(kind string, parts []homebrew.Part) ([]string, [][]hbField, error) {
	var cats []string
	var lists [][]hbField
	for _, p := range parts {
		switch kind {
		case "field":
			var fp hbFieldPart
			if err := json.Unmarshal(p.Data, &fp); err != nil {
				return nil, nil, fmt.Errorf("field part %s: %w", p.Key, err)
			}
			cats = append(cats, fp.Category)
			lists = append(lists, []hbField{fp.hbField})
		case "item":
			var ip hbItemPart
			if err := json.Unmarshal(p.Data, &ip); err != nil {
				return nil, nil, fmt.Errorf("item part %s: %w", p.Key, err)
			}
			cats = append(cats, ip.Category)
			lists = append(lists, ip.Fields)
		}
	}
	return cats, lists, nil
}

func (s fieldRuleSync) planSheet(ctx context.Context, campaignID, kind string, parts []homebrew.Part) (*sheetPlan, error) {
	cats, lists, err := sheetFields(kind, parts)
	if err != nil || len(cats) == 0 {
		return &sheetPlan{}, err
	}
	types, err := s.cats.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	plan := &sheetPlan{fields: map[int][]entities.FieldDefinition{}, rules: map[hbFieldRef]fieldrules.RuleInput{}}
	for i, name := range cats {
		et := hbTypeNamed(types, name)
		if et == nil {
			return nil, apperror.NewValidation(fmt.Sprintf("there is no category called %q", name))
		}
		defs, seen := plan.fields[et.ID]
		if !seen {
			defs = append([]entities.FieldDefinition{}, et.Fields...)
			plan.order = append(plan.order, et.ID)
		}
		for _, f := range lists[i] {
			def := entities.FieldDefinition{Key: f.Key, Label: f.Label, Type: f.Type, Section: f.Section, Options: f.Options, GMOnly: f.Director}
			if f.Type == entities.FieldTypeLink {
				to := hbTypeNamed(types, f.LinksTo)
				if to == nil {
					return nil, apperror.NewValidation(fmt.Sprintf("%s links to %q, which is not a category", f.Label, f.LinksTo))
				}
				def.LinkTo = to.ID
			}
			if f.Rule != nil {
				plan.rules[hbFieldRef{et.ID, f.Key}] = fieldrules.RuleInput{Name: f.Label, Follows: f.Rule.Follows, Rows: f.Rule.Rows}
			}
			at := -1
			for j, d := range defs {
				if strings.EqualFold(d.Key, f.Key) {
					at = j
					break
				}
			}
			if at < 0 {
				defs = append(defs, def)
				continue
			}
			// A field the sheet already has keeps its kind: changing one
			// would reinterpret every value already typed into it.
			if defs[at].Type != def.Type {
				return nil, apperror.NewValidation(fmt.Sprintf("%s already has a %s field keyed %s; give this one another key",
					et.Name, defs[at].Type, defs[at].Key))
			}
			def.Key = defs[at].Key
			def.OwnerOnly = defs[at].OwnerOnly
			def.RuleID = defs[at].RuleID
			defs[at] = def
		}
		plan.fields[et.ID] = defs
	}
	return plan, nil
}

func (s fieldRuleSync) applySheet(ctx context.Context, campaignID string, plan *sheetPlan) error {
	for ref, in := range plan.rules {
		if old, err := s.rules.RuleNamed(ctx, campaignID, in.Name); err != nil {
			return err
		} else if old != nil {
			in.ID = old.ID
		}
		saved, err := s.rules.SaveRule(ctx, campaignID, in, hbSaveAs)
		if err != nil {
			return err
		}
		defs := plan.fields[ref.typeID]
		for i := range defs {
			if strings.EqualFold(defs[i].Key, ref.key) {
				defs[i].RuleID = saved.ID
			}
		}
	}
	for _, id := range plan.order {
		if _, err := s.cats.UpdateEntityType(ctx, id, entities.UpdateEntityTypeInput{Fields: plan.fields[id]}); err != nil {
			return err
		}
	}
	return nil
}

// planWords turns words parts into the local words to save, each with the id
// of the word it replaces when that place or era already names it. A words
// part with neither place nor era is a template and saves nothing.
func (s fieldRuleSync) planWords(ctx context.Context, campaignID string, parts []homebrew.Part) ([]fieldrules.WordInput, error) {
	var eras map[int]string
	var have []fieldrules.LocalWord
	var out []fieldrules.WordInput
	for _, p := range parts {
		var wp hbWordsPart
		if err := json.Unmarshal(p.Data, &wp); err != nil {
			return nil, fmt.Errorf("words part %s: %w", p.Key, err)
		}
		if wp.PlaceID == "" && wp.Era == "" {
			continue
		}
		if have == nil {
			var err error
			if have, err = s.rules.Words(ctx, campaignID, hbSaveAs); err != nil {
				return nil, err
			}
			eras = s.rules.Eras(ctx, campaignID, hbSaveAs)
		}
		eraID := 0
		if wp.Era != "" {
			for id, name := range eras {
				if strings.EqualFold(name, wp.Era) {
					eraID = id
				}
			}
			if eraID == 0 {
				return nil, apperror.NewValidation(fmt.Sprintf("there is no era called %q on this campaign's calendar", wp.Era))
			}
		}
		for _, w := range wp.Words {
			in := fieldrules.WordInput{Word: w.Word, Local: w.Local, PlaceID: wp.PlaceID, EraID: eraID, DMOnly: w.Director}
			for _, h := range have {
				if strings.EqualFold(h.Word, w.Word) && h.PlaceID == in.PlaceID && h.EraID == in.EraID {
					in.ID = h.ID
				}
			}
			out = append(out, in)
		}
	}
	return out, nil
}

// syncFor is the PartSync of one of the three kinds; nil when the field rules
// are degraded, so the parts can still be drafted in a recipe.
func (s fieldRuleSync) syncFor(kind string) *homebrew.PartSync {
	if s.cats == nil || s.rules == nil {
		return nil
	}
	if kind == "words" {
		return &homebrew.PartSync{
			Check: func(ctx context.Context, campaignID, _ string, parts []homebrew.Part) error {
				_, err := s.planWords(ctx, campaignID, parts)
				return err
			},
			Apply: func(ctx context.Context, campaignID, _ string, parts []homebrew.Part) error {
				words, err := s.planWords(ctx, campaignID, parts)
				if err != nil {
					return err
				}
				for _, w := range words {
					if _, err := s.rules.SaveWord(ctx, campaignID, w, hbSaveAs); err != nil {
						return err
					}
				}
				return nil
			},
		}
	}
	return &homebrew.PartSync{
		Check: func(ctx context.Context, campaignID, _ string, parts []homebrew.Part) error {
			_, err := s.planSheet(ctx, campaignID, kind, parts)
			return err
		},
		Apply: func(ctx context.Context, campaignID, _ string, parts []homebrew.Part) error {
			plan, err := s.planSheet(ctx, campaignID, kind, parts)
			if err != nil {
				return err
			}
			return s.applySheet(ctx, campaignID, plan)
		},
	}
}

// hbTypeNamed finds a category by its name, plural name or slug, the way the
// Director wrote it.
func hbTypeNamed(types []entities.EntityType, name string) *entities.EntityType {
	name = strings.TrimSpace(name)
	slug := entities.Slugify(name)
	for i := range types {
		t := &types[i]
		if strings.EqualFold(t.Name, name) || strings.EqualFold(t.NamePlural, name) || (slug != "" && t.Slug == slug) {
			return t
		}
	}
	return nil
}
