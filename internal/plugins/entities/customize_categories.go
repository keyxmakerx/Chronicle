// customize_categories.go — applies the Customize page's "categories" draft
// section: creating, changing and deleting entity types in one Save.
//
// It is orchestration only. Every write goes through CreateEntityType,
// UpdateEntityType, UpdateEntityTypeLayout and DeleteEntityType, so each
// existing rule (names, colours, one level of nesting, a type with pages
// cannot be deleted) applies here unchanged.

package entities

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// maxCustomizeCategoryItems bounds one Save so a hand-built request cannot
// turn into thousands of sequential writes.
const maxCustomizeCategoryItems = 200

var (
	// customizeNewKeyPattern is the key of a category made in the draft.
	customizeNewKeyPattern = regexp.MustCompile(`^new-[a-z0-9-]{1,40}$`)
	// customizeIDKeyPattern is the key of an existing category: its id.
	customizeIDKeyPattern = regexp.MustCompile(`^[0-9]{1,9}$`)
)

// CustomizeCategoryItem is one category's change in the Customize draft.
// Empty text fields mean "unchanged"; none of them can legitimately be empty.
type CustomizeCategoryItem struct {
	// New marks a category made in the draft. Its key must start with "new-"
	// and Name and NamePlural are required.
	New        bool   `json:"new,omitempty"`
	Name       string `json:"name,omitempty"`
	NamePlural string `json:"name_plural,omitempty"`
	Icon       string `json:"icon,omitempty"`
	Color      string `json:"color,omitempty"`
	Claimable  *bool  `json:"claimable,omitempty"`
	// Parent is the key of the parent category: an existing id or another
	// "new-" key. With ClearParent it is ignored and the category goes top-level.
	Parent      string `json:"parent,omitempty"`
	ClearParent bool   `json:"clear_parent,omitempty"`
	// CopyFrom copies an existing category's layout and fields into a new one.
	CopyFrom int `json:"copy_from,omitempty"`
	// Menu is the players' sidebar-menu setting. The campaigns plugin owns the
	// sidebar, so the caller applies it; this service only carries it.
	Menu *bool `json:"menu,omitempty"`
	// Delete removes an existing category.
	Delete bool `json:"delete,omitempty"`
}

// CustomizeCategoriesPatch is the whole "categories" section: items keyed by
// an existing category id (decimal) or a "new-" key.
type CustomizeCategoriesPatch struct {
	Items map[string]CustomizeCategoryItem
}

// CustomizeCategoriesResult reports what was applied before any failure.
type CustomizeCategoriesResult struct {
	// Created maps the key of each category made to its new id. It also holds
	// a category created whose later step failed, so the caller can address it.
	Created map[string]int
	// Applied lists the keys that were fully applied, in the order done.
	Applied []string
}

// CustomizeCategoryInfo is a category with the numbers the Customize page
// shows beside it.
type CustomizeCategoryInfo struct {
	Type EntityType
	// Claimable is the effective value, after the heuristics for a type whose
	// owner never chose.
	Claimable bool
	Pages     int
	Trashed   int
}

// CustomizeCategories lists a campaign's entity types with their effective
// claimable flag and live and trashed page counts.
func (s *entityService) CustomizeCategories(ctx context.Context, campaignID string) ([]CustomizeCategoryInfo, error) {
	types, err := s.types.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing entity types: %w", err))
	}
	// Owner role (3) counts every page whatever its visibility.
	counts, err := s.entities.CountByType(ctx, campaignID, 3, "")
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("counting entities by type: %w", err))
	}
	out := make([]CustomizeCategoryInfo, 0, len(types))
	for i := range types {
		t := types[i]
		info := CustomizeCategoryInfo{Type: t, Claimable: isClaimableType(&t), Pages: counts[t.ID]}
		if s.safety != nil {
			if info.Trashed, err = s.safety.CountTrashedByType(ctx, t.ID); err != nil {
				return nil, apperror.NewInternal(err)
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// categoryPlan is the validated shape of a patch: every reference resolved
// and every order decided before the first write.
type categoryPlan struct {
	creates []string // new keys, parents before children
	updates []string // existing-id keys with a change to make
	deletes []string // existing-id keys, children before parents
}

// ApplyCustomizeCategories applies a patch: creates first (parents before
// children), then updates of existing categories, then deletes (children
// before parents). The whole patch is checked before anything is written, then
// it stops at the first item that fails; what was done stays done and is
// reported in the result so the caller can keep the rest.
func (s *entityService) ApplyCustomizeCategories(ctx context.Context, campaignID string, in CustomizeCategoriesPatch) (CustomizeCategoriesResult, error) {
	res := CustomizeCategoriesResult{Created: map[string]int{}, Applied: []string{}}

	types, err := s.types.ListByCampaign(ctx, campaignID)
	if err != nil {
		return res, apperror.NewInternal(fmt.Errorf("listing entity types: %w", err))
	}
	byID := make(map[int]*EntityType, len(types))
	for i := range types {
		byID[types[i].ID] = &types[i]
	}

	plan, err := planCustomizeCategories(in.Items, byID)
	if err != nil {
		return res, err
	}

	// resolve turns a parent key into an id: an existing id as is, a new key
	// through what has been created so far.
	resolve := func(key string) (int, error) {
		if id, ok := res.Created[key]; ok {
			return id, nil
		}
		id, convErr := strconv.Atoi(key)
		if convErr != nil {
			return 0, apperror.NewBadRequest("the parent category is not created yet")
		}
		return id, nil
	}

	for _, key := range plan.creates {
		item := in.Items[key]
		input := CreateEntityTypeInput{
			Name: item.Name, NamePlural: item.NamePlural, Icon: item.Icon, Color: item.Color,
			Claimable: item.Claimable,
		}
		var src *EntityType
		if item.CopyFrom > 0 {
			src = byID[item.CopyFrom]
			input.Fields = append([]FieldDefinition(nil), src.Fields...)
		}
		if item.Parent != "" {
			pid, err := resolve(item.Parent)
			if err != nil {
				return res, err
			}
			input.ParentTypeID = &pid
		}
		created, err := s.CreateEntityType(ctx, campaignID, input)
		if err != nil {
			return res, err
		}
		res.Created[key] = created.ID
		if src != nil {
			if err := s.UpdateEntityTypeLayout(ctx, created.ID, src.Layout); err != nil {
				// A category left without the layout the owner asked for would
				// look done; removing the empty one lets the item be retried.
				if delErr := s.DeleteEntityType(ctx, created.ID); delErr == nil {
					delete(res.Created, key)
				}
				return res, err
			}
		}
		res.Applied = append(res.Applied, key)
	}

	for _, key := range plan.updates {
		item := in.Items[key]
		id, _ := strconv.Atoi(key)
		upd := UpdateEntityTypeInput{Claimable: item.Claimable}
		if item.Name != "" {
			upd.Name = patch.Of(item.Name)
		}
		if item.NamePlural != "" {
			upd.NamePlural = patch.Of(item.NamePlural)
		}
		if item.Icon != "" {
			upd.Icon = patch.Of(item.Icon)
		}
		if item.Color != "" {
			upd.Color = patch.Of(item.Color)
		}
		switch {
		case item.ClearParent:
			upd.ClearParent = true
		case item.Parent != "":
			pid, err := resolve(item.Parent)
			if err != nil {
				return res, err
			}
			upd.ParentTypeID = &pid
		}
		if customizeItemChangesType(item) {
			if _, err := s.UpdateEntityType(ctx, id, upd); err != nil {
				return res, err
			}
		}
		res.Applied = append(res.Applied, key)
	}

	for _, key := range plan.deletes {
		id, _ := strconv.Atoi(key)
		if err := s.DeleteEntityType(ctx, id); err != nil {
			return res, err
		}
		res.Applied = append(res.Applied, key)
	}
	return res, nil
}

// customizeItemChangesType reports whether an item edits the type itself. An
// item that only sets Menu has nothing for the entity service to do.
func customizeItemChangesType(it CustomizeCategoryItem) bool {
	return it.Name != "" || it.NamePlural != "" || it.Icon != "" || it.Color != "" ||
		it.Claimable != nil || it.ClearParent || it.Parent != ""
}

// planCustomizeCategories validates a patch against the campaign's own types
// and decides the order to apply it in. Every existing id must belong to the
// campaign: an id from anywhere else is reported as not found, so the patch
// cannot probe or touch another campaign's categories.
func planCustomizeCategories(items map[string]CustomizeCategoryItem, byID map[int]*EntityType) (categoryPlan, error) {
	var plan categoryPlan
	if len(items) > maxCustomizeCategoryItems {
		return plan, apperror.NewBadRequest("too many category changes at once")
	}

	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	// Sorted so a failure stops at the same item every time.
	sort.Slice(keys, func(i, j int) bool {
		ai, aerr := strconv.Atoi(keys[i])
		bi, berr := strconv.Atoi(keys[j])
		if aerr == nil && berr == nil {
			return ai < bi
		}
		return keys[i] < keys[j]
	})

	deleting := map[string]bool{}
	for _, k := range keys {
		it := items[k]
		switch {
		case customizeNewKeyPattern.MatchString(k):
			if !it.New {
				return plan, apperror.NewBadRequest("a new category must be marked new")
			}
			if it.Delete {
				return plan, apperror.NewBadRequest("a category made in the draft cannot be deleted")
			}
			if it.Name == "" || it.NamePlural == "" {
				return plan, apperror.NewBadRequest("a new category needs a name and a plural name")
			}
			if it.CopyFrom != 0 {
				if _, ok := byID[it.CopyFrom]; !ok {
					return plan, apperror.NewNotFound("the category to copy was not found")
				}
			}
			plan.creates = append(plan.creates, k)
		case customizeIDKeyPattern.MatchString(k):
			id, _ := strconv.Atoi(k)
			if _, ok := byID[id]; !ok {
				return plan, apperror.NewNotFound("category not found")
			}
			if it.New {
				return plan, apperror.NewBadRequest("an existing category cannot be marked new")
			}
			if it.CopyFrom != 0 {
				return plan, apperror.NewBadRequest("only a new category can copy another")
			}
			if it.Delete {
				deleting[k] = true
				plan.deletes = append(plan.deletes, k)
			} else if customizeItemChangesType(it) || it.Menu != nil {
				plan.updates = append(plan.updates, k)
			}
		default:
			return plan, apperror.NewBadRequest("invalid category key")
		}
	}

	// parentOf is where each category sits once the patch is applied.
	parentOf := map[string]string{}
	for id, t := range byID {
		if t.ParentTypeID != nil {
			parentOf[strconv.Itoa(id)] = strconv.Itoa(*t.ParentTypeID)
		}
	}
	for _, k := range keys {
		it := items[k]
		if it.Delete {
			continue
		}
		switch {
		case it.ClearParent && !it.New:
			delete(parentOf, k)
		case it.Parent != "":
			if err := checkCustomizeParentKey(it.Parent, items, byID, deleting); err != nil {
				return plan, err
			}
			parentOf[k] = it.Parent
		}
	}

	// A walk up from any category that does not end at the top is a cycle.
	for k := range parentOf {
		cur, steps := k, 0
		for cur != "" {
			steps++
			if steps > len(parentOf)+1 {
				return plan, apperror.NewBadRequest("categories cannot be their own ancestors")
			}
			cur = parentOf[cur]
		}
	}

	// A category being deleted cannot leave children behind.
	for k := range deleting {
		for child, parent := range parentOf {
			if parent == k && !deleting[child] {
				return plan, apperror.NewBadRequest("move or delete a category's sub-categories before deleting it")
			}
		}
	}

	depth := func(k string) int {
		d := 0
		for cur := parentOf[k]; cur != ""; cur = parentOf[cur] {
			d++
		}
		return d
	}
	sort.SliceStable(plan.creates, func(i, j int) bool { return depth(plan.creates[i]) < depth(plan.creates[j]) })
	sort.SliceStable(plan.deletes, func(i, j int) bool { return depth(plan.deletes[i]) > depth(plan.deletes[j]) })
	return plan, nil
}

// checkCustomizeParentKey accepts a parent that is an existing category of the
// campaign or another new category in the patch, and not one being deleted.
func checkCustomizeParentKey(parent string, items map[string]CustomizeCategoryItem, byID map[int]*EntityType, deleting map[string]bool) error {
	switch {
	case customizeNewKeyPattern.MatchString(parent):
		if it, ok := items[parent]; !ok || !it.New {
			return apperror.NewBadRequest("the parent category was not found")
		}
	case customizeIDKeyPattern.MatchString(parent):
		id, _ := strconv.Atoi(parent)
		if _, ok := byID[id]; !ok {
			return apperror.NewBadRequest("the parent category was not found")
		}
		if deleting[parent] {
			return apperror.NewBadRequest("a category being deleted cannot be a parent")
		}
	default:
		return apperror.NewBadRequest("invalid parent category")
	}
	return nil
}
