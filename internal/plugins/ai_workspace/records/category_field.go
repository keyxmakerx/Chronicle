package records

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// CategoriesAPI is the slice of the entity service a category's fields need.
// UpdateEntityType checks link and rule fields before it saves.
type CategoriesAPI interface {
	GetEntityTypes(ctx context.Context, campaignID string) ([]entities.EntityType, error)
	UpdateEntityType(ctx context.Context, id int, input entities.UpdateEntityTypeInput) (*entities.EntityType, error)
}

// fieldTypes are the sheet's field types, as the category editor offers them.
var fieldTypes = map[string]bool{
	"text": true, "number": true, "textarea": true, "url": true, "checkbox": true, "select": true,
	entities.FieldTypeLink: true, entities.FieldTypeRule: true, entities.FieldTypeCalendarDate: true,
}

// CategoryFieldKind adds, changes and removes one field on a category's
// sheet. Every other field of the category is kept as it is.
type CategoryFieldKind struct {
	Types CategoriesAPI
	// Rules finds a rule field's rule by name; nil when homebrew rules are
	// off, and then a rule field can't be made.
	Rules HomebrewAPI
}

func (CategoryFieldKind) Name() string  { return "category-field" }
func (CategoryFieldKind) Label() string { return "Category field" }
func (CategoryFieldKind) Doc() string {
	return "A field on a category's sheet. `name` is the field's label and `category` the category's name; matched by both. `type` is one of text, number, textarea, url, checkbox, select (give `options`, a list), link (a page of another category: give `links_to`, that category's name; a page's sheet then takes the page's name) rule (worked out by a `kind: field-rule`: give `rule`, its name; the rule must already exist or come earlier in the paste) or calendar_date (a day on the campaign's calendar; a page gives it as `year-month-day`, month by number, like 1043-3-22). A calendar_date field takes `ago: true` (say how long ago the date was) and `countdowns`, a list of `name`, `days`, optional `director: true` (only Directors see it) and `longer` (other lengths a page can pick, each `name` and `days`); naming `countdowns` replaces them all. Optional `key` (else made from the name), `section`, `director: true` (only Directors see its value). `action: update` changes only what it names, `rename_to` renames it; `action: delete` takes the field off the sheet.\n\n```\n---\nkind: category-field\ncategory: Items\nname: Source creature\ntype: link\nlinks_to: Creatures\nsection: Remnant\n---\n```\n\n```\n---\nkind: category-field\ncategory: Characters\nname: Guild rank\ntype: rule\nrule: Guild rank\n---\n```"
}

func (k CategoryFieldKind) Title(r Record) string {
	return r.Name + " on " + r.Str("category")
}

var nonKey = regexp.MustCompile(`[^a-z0-9]+`)

// fieldKeyOf makes a field key from a label: lower case, words joined by _.
func fieldKeyOf(label string) string {
	return strings.Trim(nonKey.ReplaceAllString(strings.ToLower(label), "_"), "_")
}

func typeNamed(types []entities.EntityType, name string) *entities.EntityType {
	slug := entities.Slugify(name)
	for i := range types {
		t := &types[i]
		if sameName(t.Name, name) || sameName(t.NamePlural, name) || (slug != "" && t.Slug == slug) {
			return t
		}
	}
	return nil
}

// fieldIndex is the field the record names, by label or key; -1 when none.
func fieldIndex(defs []entities.FieldDefinition, r Record) int {
	key := r.Str("key")
	if key == "" {
		key = fieldKeyOf(r.Name)
	}
	for i, d := range defs {
		if sameName(d.Label, r.Name) || strings.EqualFold(d.Key, key) {
			return i
		}
	}
	return -1
}

// fields works out the category's whole field list after this record, or
// the plain-words reason it can't.
func (k CategoryFieldKind) fields(ctx context.Context, campaignID string, r Record, ruleLater bool) (*entities.EntityType, []entities.FieldDefinition, error) {
	if r.Name == "" {
		return nil, nil, badRequestf("a field needs a name")
	}
	types, err := k.Types.GetEntityTypes(ctx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	et := typeNamed(types, r.Str("category"))
	if et == nil {
		return nil, nil, badRequestf("there is no category called %s", quote(r.Str("category")))
	}
	defs := append([]entities.FieldDefinition{}, et.Fields...)
	i := fieldIndex(defs, r)
	switch r.Action {
	case ActionCreate:
		if i >= 0 {
			return et, nil, badRequestf("%s already has a field called %s; use action: update", et.Name, quote(defs[i].Label))
		}
		f := entities.FieldDefinition{Label: r.Name, Key: r.Str("key"), Type: "text"}
		if f.Key == "" {
			f.Key = fieldKeyOf(r.Name)
		}
		if f.Key == "" {
			return et, nil, badRequestf("the field's name needs a letter or number")
		}
		if err := k.set(ctx, campaignID, types, &f, r, ruleLater); err != nil {
			return et, nil, err
		}
		return et, append(defs, f), nil
	case ActionUpdate:
		if i < 0 {
			return et, nil, badRequestf("%s has no field called %s", et.Name, quote(r.Name))
		}
		if s := r.Str("rename_to"); s != "" {
			defs[i].Label = s
		}
		if err := k.set(ctx, campaignID, types, &defs[i], r, ruleLater); err != nil {
			return et, nil, err
		}
		return et, defs, nil
	default:
		if i < 0 {
			return et, nil, badRequestf("%s has no field called %s", et.Name, quote(r.Name))
		}
		return et, append(defs[:i], defs[i+1:]...), nil
	}
}

// set applies the keys the record names to one field. ruleLater lets a
// review pass a rule that isn't saved yet, since one earlier in the paste
// is saved before this row commits.
func (k CategoryFieldKind) set(ctx context.Context, campaignID string, types []entities.EntityType, f *entities.FieldDefinition, r Record, ruleLater bool) error {
	if r.Has("type") {
		t := strings.ToLower(r.Str("type"))
		if !fieldTypes[t] {
			return badRequestf("type %s is not one of: text, number, textarea, url, checkbox, select, link, rule, calendar_date", quote(t))
		}
		f.Type = t
	}
	if r.Has("section") {
		f.Section = r.Str("section")
	}
	if v, ok := r.Bool("director"); ok {
		f.GMOnly = v
	}
	if r.Has("options") {
		f.Options = nil
		for _, o := range r.List("options") {
			if s := strings.TrimSpace(fmt.Sprint(o)); s != "" {
				f.Options = append(f.Options, s)
			}
		}
	}
	if r.Has("links_to") {
		to := typeNamed(types, r.Str("links_to"))
		if to == nil {
			return badRequestf("there is no category called %s to link to", quote(r.Str("links_to")))
		}
		f.LinkTo = to.ID
	}
	if r.Has("rule") {
		if k.Rules == nil {
			return badRequestf("homebrew rules are not available on this server")
		}
		rule, err := k.Rules.RuleNamed(ctx, campaignID, r.Str("rule"))
		if err != nil {
			return err
		}
		if rule == nil && ruleLater {
			return nil
		}
		if rule == nil {
			return badRequestf("there is no rule called %s (a rule this paste makes must come before the field)", quote(r.Str("rule")))
		}
		f.RuleID = rule.ID
	}
	if v, ok := r.Bool("ago"); ok {
		f.DateAgo = v
	}
	if r.Has("countdowns") {
		cds, err := readCountdowns(r)
		if err != nil {
			return err
		}
		f.Countdowns = cds
	}
	if f.Type == entities.FieldTypeCalendarDate {
		if err := entities.CheckDateSettings(f); err != nil {
			return err
		}
	} else if r.Has("ago") || r.Has("countdowns") {
		return badRequestf("`ago` and `countdowns` belong to a calendar_date field")
	} else {
		f.DateAgo, f.Countdowns = false, nil
	}
	switch {
	case f.Type == "select" && len(f.Options) == 0:
		return badRequestf("a select field needs `options`")
	case f.Type == entities.FieldTypeLink && f.LinkTo == 0:
		return badRequestf("a link field needs `links_to`, the category its pages come from")
	case f.Type == entities.FieldTypeRule && f.RuleID == 0:
		return badRequestf("a rule field needs `rule`, the rule that works it out")
	}
	return nil
}

func (k CategoryFieldKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return notDirectorPlan()
	}
	et, _, err := k.fields(ctx, campaignID, r, true)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	var warn []string
	if r.Action != ActionDelete && r.Has("rule") && k.Rules != nil {
		// A rule this paste makes is saved before this row commits, so a
		// missing one is only a warning here; the commit checks again.
		if rule, err := k.Rules.RuleNamed(ctx, campaignID, r.Str("rule")); err == nil && rule == nil {
			warn = append(warn, "no rule called "+quote(r.Str("rule"))+" yet; it must come earlier in this paste")
		}
	}
	switch r.Action {
	case ActionCreate:
		return Plan{Summary: "adds the field to the " + et.Name + " sheet", Warnings: warn}
	case ActionUpdate:
		return Plan{Summary: "changes the field on the " + et.Name + " sheet", Warnings: warn}
	}
	return Plan{Summary: "takes the field off the " + et.Name + " sheet",
		Warnings: []string{"pages of this category stop showing this field"}}
}

func (k CategoryFieldKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if !a.CanAuthorDmOnly() {
		return apperror.NewBadRequest(notDirectorPlan().Error)
	}
	et, defs, err := k.fields(ctx, campaignID, r, false)
	if err != nil {
		return err
	}
	_, err = k.Types.UpdateEntityType(ctx, et.ID, entities.UpdateEntityTypeInput{Fields: defs})
	return err
}

// Export is empty: every category's fields are already in the prompt.
func (CategoryFieldKind) Export(context.Context, string, Actor) (string, error) { return "", nil }

// readCountdowns reads a calendar date field's `countdowns`: each a name and
// its usual days, with `longer` lengths a page can pick instead.
func readCountdowns(r Record) ([]entities.FieldCountdown, error) {
	items, err := listItems(r, "countdowns", false)
	if err != nil {
		return nil, err
	}
	out := make([]entities.FieldCountdown, 0, len(items))
	for _, it := range items {
		days, ok, err := it.Int("days")
		if err != nil || !ok {
			return nil, badRequestf("countdown %s needs `days`, a whole number", quote(it.Name))
		}
		cd := entities.FieldCountdown{Name: it.Name, Lengths: []entities.CountdownLength{{Days: days}}}
		cd.GMOnly, _ = it.Bool("director")
		longer, err := listItems(it, "longer", false)
		if err != nil {
			return nil, err
		}
		for _, l := range longer {
			n, ok, err := l.Int("days")
			if err != nil || !ok {
				return nil, badRequestf("length %s of countdown %s needs `days`", quote(l.Name), quote(it.Name))
			}
			cd.Lengths = append(cd.Lengths, entities.CountdownLength{Name: l.Name, Days: n})
		}
		out = append(out, cd)
	}
	return out, nil
}
