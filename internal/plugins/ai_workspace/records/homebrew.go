package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// The homebrew tools AI Import can build: a rule that works a field out
// from another (field-rule), what a place or era calls a word (local-word),
// and the fields on a category's sheet (category-field), which is how a
// worked-out field or a link to a page gets onto a sheet at all. All three
// are the Director's, as in the system book.

// HomebrewAPI is what these kinds need from the homebrew rules plugin; the
// adapter in internal/app maps it onto that plugin's service, which checks
// again that the Actor is a Director.
type HomebrewAPI interface {
	Rules(ctx context.Context, campaignID string, a Actor) ([]HomebrewRule, error)
	// RuleNamed finds a rule by name, ignoring case; nil when none.
	RuleNamed(ctx context.Context, campaignID, name string) (*HomebrewRule, error)
	CheckRule(in HomebrewRule) error
	SaveRule(ctx context.Context, campaignID string, in HomebrewRule, a Actor) error
	DeleteRule(ctx context.Context, campaignID string, id int, a Actor) error

	Words(ctx context.Context, campaignID string, a Actor) ([]HomebrewWord, error)
	CheckWord(ctx context.Context, campaignID string, in HomebrewWord, a Actor) error
	SaveWord(ctx context.Context, campaignID string, in HomebrewWord, a Actor) error
	DeleteWord(ctx context.Context, campaignID string, id int, a Actor) error
	// Eras is every era on the campaign calendar by id.
	Eras(ctx context.Context, campaignID string, a Actor) map[int]string
}

// HomebrewRule is a worked-out field's rule: the first row matching the
// followed field's value gives the result. ID 0 is a new rule.
type HomebrewRule struct {
	ID      int
	Name    string
	Follows string
	Rows    []RuleRow
}

// RuleRow matches an exact value (Is) or a range of numbers (From, To).
type RuleRow struct {
	Is    string
	From  *float64
	To    *float64
	Gives string
}

// HomebrewWord is what one place (PlaceID) or one era (EraID) calls Word.
// ID 0 is a new word.
type HomebrewWord struct {
	ID        int
	Word      string
	Local     string
	PlaceID   string
	PlaceName string
	EraID     int
	EraName   string
	DMOnly    bool
}

func notDirectorPlan() Plan {
	return Plan{Error: "only the campaign's Directors can change its homebrew tools"}
}

// changedUnderfoot is Apply's answer when what Plan saw is gone or new.
func changedUnderfoot() error {
	return apperror.NewBadRequest("it changed while you were reviewing; check it again")
}

// ---- field-rule ----

// FieldRuleKind adds, changes and removes the rules worked-out fields follow.
type FieldRuleKind struct {
	Svc HomebrewAPI
}

func (FieldRuleKind) Name() string  { return "field-rule" }
func (FieldRuleKind) Label() string { return "Worked-out field rule" }
func (FieldRuleKind) Doc() string {
	return "A rule that works one sheet field out from another, as a table: the first row that matches the followed field's value gives the result. `follows` is the key of the field it reads. Each of `rows` has `gives` and either `is` (an exact value) or `from` and/or `to` (a range of numbers, both ends included). Matched by `name`; `action: update` replaces what it names. A field shows the rule's result once a category has a field of `type: rule` naming it (see `kind: category-field`); a value typed into that field by hand wins over the rule.\n\n```\n---\nkind: field-rule\nname: Guild rank\nfollows: echelon\nrows:\n  - is: 1\n    gives: Copper\n  - is: 2\n    gives: Silver\n  - is: 3\n    gives: Gold\n  - from: 4\n    gives: Starsteel\n---\n```"
}

func (k FieldRuleKind) input(cur *HomebrewRule, r Record) (HomebrewRule, error) {
	in := HomebrewRule{Name: r.Name, Follows: r.Str("follows")}
	if cur != nil {
		in.ID = cur.ID
		if in.Follows == "" {
			in.Follows = cur.Follows
		}
		in.Rows = cur.Rows
	}
	if r.Has("rows") {
		rows, err := ruleRows(r.List("rows"))
		if err != nil {
			return in, err
		}
		in.Rows = rows
	}
	return in, nil
}

// ruleRows reads the YAML rows; the service checks what they say.
func ruleRows(list []any) ([]RuleRow, error) {
	out := make([]RuleRow, 0, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, badRequestf("rows: row %d needs `gives` and `is` or `from`/`to`", i+1)
		}
		row := Record{Fields: lowerKeys(m)}
		rr := RuleRow{Is: row.Str("is"), Gives: row.Str("gives")}
		for _, end := range []string{"from", "to"} {
			f, ok, err := row.Float(end)
			if err != nil {
				return nil, badRequestf("rows: row %d: %s", i+1, planError(err))
			}
			if ok {
				v := f
				if end == "from" {
					rr.From = &v
				} else {
					rr.To = &v
				}
			}
		}
		out = append(out, rr)
	}
	return out, nil
}

func lowerKeys(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return out
}

func (k FieldRuleKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return notDirectorPlan()
	}
	if r.Name == "" {
		return Plan{Error: "a rule needs a name"}
	}
	cur, err := k.Svc.RuleNamed(ctx, campaignID, r.Name)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		if cur != nil {
			return Plan{Error: "there is already a rule called " + quote(r.Name) + "; use action: update"}
		}
	case ActionUpdate, ActionDelete:
		if cur == nil {
			return Plan{Error: "no rule called " + quote(r.Name)}
		}
	}
	if r.Action == ActionDelete {
		return Plan{
			Summary:  "removes the rule",
			Warnings: []string{"fields that follow it keep any value typed by hand and otherwise show nothing"},
		}
	}
	in, err := k.input(cur, r)
	if err == nil {
		err = k.Svc.CheckRule(in)
	}
	if err != nil {
		return Plan{Error: planError(err)}
	}
	verb := "new rule"
	if cur != nil {
		verb = "changes the rule"
	}
	return Plan{Summary: verb + ": reads " + in.Follows + ", " + plural(len(in.Rows), "row", "rows")}
}

func (k FieldRuleKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	cur, err := k.Svc.RuleNamed(ctx, campaignID, r.Name)
	if err != nil {
		return err
	}
	if (r.Action == ActionCreate) != (cur == nil) {
		return changedUnderfoot()
	}
	if r.Action == ActionDelete {
		return k.Svc.DeleteRule(ctx, campaignID, cur.ID, a)
	}
	in, err := k.input(cur, r)
	if err != nil {
		return err
	}
	err = k.Svc.SaveRule(ctx, campaignID, in, a)
	return err
}

func (k FieldRuleKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	rules, err := k.Svc.Rules(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, rule := range rules {
		b.WriteString("- " + quote(rule.Name) + " follows `" + rule.Follows + "`:")
		for _, row := range rule.Rows {
			b.WriteString(" " + rowText(row) + " → " + row.Gives + ";")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func rowText(row RuleRow) string {
	switch {
	case row.Is != "":
		return row.Is
	case row.From != nil && row.To != nil:
		return fmt.Sprintf("%g–%g", *row.From, *row.To)
	case row.From != nil:
		return fmt.Sprintf("%g or more", *row.From)
	case row.To != nil:
		return fmt.Sprintf("up to %g", *row.To)
	}
	return ""
}

// ---- local-word ----

// LocalWordKind adds, changes and removes what a place or an era calls a
// word: a coin, a rank, anything a field or a shop shows.
type LocalWordKind struct {
	Svc      HomebrewAPI
	Entities EntityLookup
}

func (LocalWordKind) Name() string  { return "local-word" }
func (LocalWordKind) Label() string { return "Local word" }
func (LocalWordKind) Doc() string {
	return "What one place or one era calls a word, such as a coin or a rank. `name` is the usual word, `local` what it is called there, and exactly one of `place` (a page name: a region, a city; pages inside it use the word too, the nearest place winning) or `era` (an era on the campaign calendar, used while it is the current era; a place beats an era). Shops show their coins by the local name, and sheet fields their values. Optional `director: true` keeps it to Directors. Matched by `name` with its `place` or `era`.\n\n```\n---\nkind: local-word\nname: Mark\nlocal: Grain\nplace: Saltreach\n---\n```"
}

func (k LocalWordKind) Title(r Record) string {
	where := r.Str("place")
	if where == "" {
		where = r.Str("era")
	}
	return r.Name + " → " + r.Str("local") + " in " + where
}

// target is where the word applies: a place's page id or an era's id.
// pending is a place this paste makes, which has no id until it commits.
func (k LocalWordKind) target(ctx context.Context, campaignID string, a Actor, r Record) (placeID string, eraID int, pending bool, err error) {
	place, era := r.Str("place"), r.Str("era")
	if (place == "") == (era == "") {
		return "", 0, false, badRequestf("say where the word is used: `place` or `era`, not both")
	}
	if place != "" {
		e, _ := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(place))
		if e != nil && e.CampaignID == campaignID {
			return e.ID, 0, false, nil
		}
		if r.Made[entities.Slugify(place)] {
			return "", 0, true, nil
		}
		return "", 0, false, badRequestf("there is no page called %s", quote(place))
	}
	for id, name := range k.Svc.Eras(ctx, campaignID, a) {
		if sameName(name, era) {
			return "", id, false, nil
		}
	}
	return "", 0, false, badRequestf("the calendar has no era called %s", quote(era))
}

func (k LocalWordKind) find(ctx context.Context, campaignID string, a Actor, r Record, placeID string, eraID int) (*HomebrewWord, error) {
	words, err := k.Svc.Words(ctx, campaignID, a)
	if err != nil {
		return nil, err
	}
	for i := range words {
		w := words[i]
		if sameName(w.Word, r.Name) && w.PlaceID == placeID && w.EraID == eraID {
			return &w, nil
		}
	}
	return nil, nil
}

func (k LocalWordKind) input(cur *HomebrewWord, r Record, placeID string, eraID int) HomebrewWord {
	in := HomebrewWord{Word: r.Name, Local: r.Str("local"), PlaceID: placeID, EraID: eraID}
	if cur != nil {
		in.ID = cur.ID
		if in.Local == "" {
			in.Local = cur.Local
		}
		in.DMOnly = cur.DMOnly
	}
	if v, ok := r.Bool("director"); ok {
		in.DMOnly = v
	}
	return in
}

func (k LocalWordKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return notDirectorPlan()
	}
	if r.Name == "" {
		return Plan{Error: "a local word needs `name`, the usual word"}
	}
	placeID, eraID, pending, err := k.target(ctx, campaignID, a, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	summary := "calls " + r.Name + " " + quote(r.Str("local"))
	if pending {
		if r.Action != ActionCreate {
			return Plan{Error: "no local word for " + quote(r.Name) + " there"}
		}
		return Plan{Summary: summary + " in a place this paste makes"}
	}
	cur, err := k.find(ctx, campaignID, a, r, placeID, eraID)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	switch r.Action {
	case ActionCreate:
		if cur != nil {
			return Plan{Error: quote(r.Name) + " already has a local name there; use action: update"}
		}
	default:
		if cur == nil {
			return Plan{Error: "no local word for " + quote(r.Name) + " there"}
		}
	}
	if r.Action == ActionDelete {
		return Plan{Summary: "goes back to the usual word " + quote(r.Name)}
	}
	if err := k.Svc.CheckWord(ctx, campaignID, k.input(cur, r, placeID, eraID), a); err != nil {
		return Plan{Error: planError(err)}
	}
	return Plan{Summary: summary}
}

func (k LocalWordKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	placeID, eraID, pending, err := k.target(ctx, campaignID, a, r)
	if err != nil {
		return err
	}
	if pending {
		return badRequestf("the page %s was not made", quote(r.Str("place")))
	}
	cur, err := k.find(ctx, campaignID, a, r, placeID, eraID)
	if err != nil {
		return err
	}
	if (r.Action == ActionCreate) != (cur == nil) {
		return changedUnderfoot()
	}
	if r.Action == ActionDelete {
		return k.Svc.DeleteWord(ctx, campaignID, cur.ID, a)
	}
	err = k.Svc.SaveWord(ctx, campaignID, k.input(cur, r, placeID, eraID), a)
	return err
}

func (k LocalWordKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	words, err := k.Svc.Words(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, w := range words {
		where := "place: " + quote(w.PlaceName)
		if w.EraID != 0 {
			where = "era: " + quote(w.EraName)
		}
		b.WriteString("- " + quote(w.Word) + " is " + quote(w.Local) + " (" + where + ")")
		if w.DMOnly {
			b.WriteString(" (Directors only)")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}
