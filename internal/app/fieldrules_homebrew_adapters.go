package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/plugins/fieldrules"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
)

func init() {
	addWorkshopSource("field rules", func(reg *homebrew.Registry, d workshopDeps) error {
		if d.FieldRules == nil {
			return nil
		}
		return registerFieldRuleParts(reg, d.FieldRules.PlaceName)
	})
}

// Limits on what one Workshop part may describe; a recipe is capped at 16 KB
// anyway, these keep each list readable in the inspector.
const (
	hbMaxItemFields = 40
	hbMaxOptions    = 50
	hbMaxPartWords  = 100
	hbMaxKeyLen     = 64
)

var (
	hbKeyPattern = regexp.MustCompile(`^[a-z0-9_]+$`)
	hbNonKey     = regexp.MustCompile(`[^a-z0-9]+`)
)

// hbFieldTypes are the sheet's field types, as Customize offers them.
var hbFieldTypes = map[string]bool{
	"text": true, "number": true, "textarea": true, "url": true, "checkbox": true, "select": true,
	entities.FieldTypeLink: true, entities.FieldTypeRule: true,
}

// hbRule is a field's worked-out rule inside a recipe. It takes the field's
// label as its name, so a field and its rule can't drift apart.
type hbRule struct {
	Follows string               `json:"follows"`
	Rows    []fieldrules.RuleRow `json:"rows"`
}

// hbField is one sheet field as a recipe describes it. Categories are named,
// not numbered, so a saved layout still works in another campaign.
type hbField struct {
	Label    string   `json:"label"`
	Key      string   `json:"key,omitempty"`
	Type     string   `json:"type,omitempty"`
	Section  string   `json:"section,omitempty"`
	Options  []string `json:"options,omitempty"`
	LinksTo  string   `json:"linksTo,omitempty"`
	Director bool     `json:"director,omitempty"`
	Rule     *hbRule  `json:"rule,omitempty"`
}

type hbFieldPart struct {
	Category string `json:"category"`
	hbField
}

type hbItemPart struct {
	Category string    `json:"category"`
	Fields   []hbField `json:"fields"`
}

type hbWord struct {
	Word     string `json:"word"`
	Local    string `json:"local"`
	Director bool   `json:"director,omitempty"`
}

// hbWordsPart is a list of local words for one place (a page, by id, since a
// place is a page) or one era (by name), or for neither while it is still a
// template.
type hbWordsPart struct {
	PlaceID string   `json:"placeId,omitempty"`
	Era     string   `json:"era,omitempty"`
	Words   []hbWord `json:"words"`
}

// registerFieldRuleParts plugs the field rules into the Homebrew Workshop: a
// field part is one sheet field (with its worked-out rule when it has one),
// an item part an item category's fields, and a words part one place's or
// era's local words. placeName says whether a page belongs to the campaign.
func registerFieldRuleParts(reg *homebrew.Registry, placeName func(ctx context.Context, campaignID, entityID string) (string, bool)) error {
	parts := []homebrew.PartKind{
		{Name: "field", Label: "Field", Check: checkFieldPart},
		{Name: "item", Label: "Item fields", Check: checkItemPart},
		{Name: "words", Label: "Local words", Check: func(ctx context.Context, campaignID string, raw json.RawMessage) (json.RawMessage, error) {
			return checkWordsPart(ctx, campaignID, raw, placeName)
		}},
	}
	for _, p := range parts {
		if err := reg.RegisterPart(p); err != nil {
			return err
		}
	}
	return nil
}

func checkFieldPart(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var p hbFieldPart
	if err := strictJSON(raw, &p); err != nil {
		return nil, err
	}
	cat, err := hbCategory(p.Category)
	if err != nil {
		return nil, err
	}
	f, err := cleanHBField(p.hbField)
	if err != nil {
		return nil, err
	}
	return json.Marshal(hbFieldPart{Category: cat, hbField: f})
}

func checkItemPart(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	var p hbItemPart
	if err := strictJSON(raw, &p); err != nil {
		return nil, err
	}
	cat, err := hbCategory(p.Category)
	if err != nil {
		return nil, err
	}
	switch {
	case len(p.Fields) == 0:
		return nil, apperror.NewValidation("add at least one field")
	case len(p.Fields) > hbMaxItemFields:
		return nil, apperror.NewValidation(fmt.Sprintf("an item can have at most %d fields here", hbMaxItemFields))
	}
	out := hbItemPart{Category: cat}
	seen := map[string]bool{}
	for i, f := range p.Fields {
		c, err := cleanHBField(f)
		if err != nil {
			return nil, apperror.NewValidation(fmt.Sprintf("field %d: %s", i+1, hbMessage(err)))
		}
		if seen[c.Key] {
			return nil, apperror.NewValidation(fmt.Sprintf("field %d has the same key as an earlier one (%s)", i+1, c.Key))
		}
		seen[c.Key] = true
		out.Fields = append(out.Fields, c)
	}
	return json.Marshal(out)
}

func checkWordsPart(ctx context.Context, campaignID string, raw json.RawMessage, placeName func(context.Context, string, string) (string, bool)) (json.RawMessage, error) {
	var p hbWordsPart
	if err := strictJSON(raw, &p); err != nil {
		return nil, err
	}
	p.PlaceID = strings.TrimSpace(p.PlaceID)
	p.Era = strings.TrimSpace(p.Era)
	switch {
	case p.PlaceID != "" && p.Era != "":
		return nil, apperror.NewValidation("the words belong to one place or one era, not both")
	case utf8.RuneCountInString(p.Era) > fieldrules.MaxNameLen:
		return nil, apperror.NewValidation("the era's name is too long")
	case len(p.Words) == 0:
		return nil, apperror.NewValidation("add at least one word")
	case len(p.Words) > hbMaxPartWords:
		return nil, apperror.NewValidation(fmt.Sprintf("a list can have at most %d words", hbMaxPartWords))
	}
	// A page id from another campaign would rename nothing here and say
	// something about that campaign, so only this campaign's pages count.
	if p.PlaceID != "" {
		if _, ok := placeName(ctx, campaignID, p.PlaceID); !ok {
			return nil, apperror.NewValidation("that place is not a page of this campaign")
		}
	}
	out := hbWordsPart{PlaceID: p.PlaceID, Era: p.Era}
	seen := map[string]bool{}
	for i, w := range p.Words {
		w.Word = strings.TrimSpace(w.Word)
		w.Local = strings.TrimSpace(w.Local)
		switch {
		case w.Word == "" || w.Local == "":
			return nil, apperror.NewValidation(fmt.Sprintf("word %d needs the usual word and what it is called", i+1))
		case utf8.RuneCountInString(w.Word) > fieldrules.MaxNameLen || utf8.RuneCountInString(w.Local) > fieldrules.MaxNameLen:
			return nil, apperror.NewValidation(fmt.Sprintf("word %d is too long", i+1))
		case seen[strings.ToLower(w.Word)]:
			return nil, apperror.NewValidation(fmt.Sprintf("word %d (%s) is already in the list", i+1, w.Word))
		}
		seen[strings.ToLower(w.Word)] = true
		out.Words = append(out.Words, w)
	}
	return json.Marshal(out)
}

func hbCategory(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", apperror.NewValidation("say which category the fields go on")
	case utf8.RuneCountInString(name) > fieldrules.MaxNameLen:
		return "", apperror.NewValidation("the category's name is too long")
	}
	return name, nil
}

// cleanHBField checks one field the way Customize and AI Import would, so a
// recipe never holds a field the category editor would refuse.
func cleanHBField(f hbField) (hbField, error) {
	f.Label = strings.TrimSpace(f.Label)
	f.Key = strings.TrimSpace(f.Key)
	f.Type = strings.ToLower(strings.TrimSpace(f.Type))
	f.Section = strings.TrimSpace(f.Section)
	f.LinksTo = strings.TrimSpace(f.LinksTo)
	if f.Type == "" {
		f.Type = "text"
	}
	if f.Key == "" {
		f.Key = strings.Trim(hbNonKey.ReplaceAllString(strings.ToLower(f.Label), "_"), "_")
	}
	switch {
	case f.Label == "":
		return f, apperror.NewValidation("a field needs a label")
	case utf8.RuneCountInString(f.Label) > fieldrules.MaxNameLen || utf8.RuneCountInString(f.Section) > fieldrules.MaxNameLen:
		return f, apperror.NewValidation("the field's label or section is too long")
	case f.Key == "" || len(f.Key) > hbMaxKeyLen || !hbKeyPattern.MatchString(f.Key):
		return f, apperror.NewValidation("the field's key is lower-case letters, numbers and _")
	case !hbFieldTypes[f.Type]:
		return f, apperror.NewValidation(fmt.Sprintf("type %q is not one of: text, number, textarea, url, checkbox, select, link, rule", f.Type))
	case len(f.Options) > hbMaxOptions:
		return f, apperror.NewValidation(fmt.Sprintf("a field can have at most %d options", hbMaxOptions))
	case utf8.RuneCountInString(f.LinksTo) > fieldrules.MaxNameLen:
		return f, apperror.NewValidation("the linked category's name is too long")
	}
	opts := f.Options
	f.Options = nil
	for _, o := range opts {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if utf8.RuneCountInString(o) > fieldrules.MaxNameLen {
			return f, apperror.NewValidation("an option is too long")
		}
		f.Options = append(f.Options, o)
	}
	switch {
	case f.Type == "select" && len(f.Options) == 0:
		return f, apperror.NewValidation("a select field needs its options")
	case f.Type != "select" && len(f.Options) > 0:
		return f, apperror.NewValidation("only a select field has options")
	case f.Type == entities.FieldTypeLink && f.LinksTo == "":
		return f, apperror.NewValidation("a link field needs the category its pages come from")
	case f.Type != entities.FieldTypeLink && f.LinksTo != "":
		return f, apperror.NewValidation("only a link field links to a category")
	case f.Type == entities.FieldTypeRule && f.Rule == nil:
		return f, apperror.NewValidation("a worked-out field needs its rule")
	case f.Type != entities.FieldTypeRule && f.Rule != nil:
		return f, apperror.NewValidation("only a worked-out field has a rule")
	}
	if f.Rule != nil {
		r, err := fieldrules.CleanRule(fieldrules.RuleInput{Name: f.Label, Follows: f.Rule.Follows, Rows: f.Rule.Rows})
		if err != nil {
			return f, err
		}
		f.Rule = &hbRule{Follows: r.Follows, Rows: r.Rows}
	}
	return f, nil
}

// hbMessage is an error's message for the Director, without its wrapping.
func hbMessage(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Message
	}
	return err.Error()
}
