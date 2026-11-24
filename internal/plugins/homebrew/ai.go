package homebrew

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AI is the Workshop's part of the AI engine (aiplug, ADR-071): the
// `workshop` lookup, which shows the recipes, what a recipe can be built
// from and the Homebrew drawer, and the `workshop-recipe` block an AI asks
// to add, change or remove a recipe with. Both go through the Service, so
// they are the Director's only, as the Workshop is, and a request changes
// nothing until the owner approves it on the review page.
type AI struct {
	svc Service
}

// NewAI builds the Workshop's AI part over its service.
func NewAI(svc Service) *AI { return &AI{svc: svc} }

var (
	_ aiplug.Kind    = (*AI)(nil)
	_ aiplug.Briefed = (*AI)(nil)
	_ aiplug.Lookup  = (*AI)(nil)
)

// aiViewer is the asker as the Workshop sees them. A player's key, or the
// owner's key in Safe privacy, reads as a player and is refused.
func aiViewer(a aiplug.Actor) Viewer {
	return Viewer{UserID: a.UserID, Director: a.CanAuthorDmOnly(), VisibilityRole: a.Role}
}

const aiDirectorsOnly = "the Homebrew Workshop is the Director's, so it is left out in Safe privacy and on a player's key; choose Permitted to include it"

// ---- lookup ----

func (*AI) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{
		What: "workshop", Aliases: []string{"homebrew", "homebrew-workshop", "workshop-recipes"}, Keys: "none",
		Brief: "the Homebrew Workshop: every recipe written as the `workshop-recipe` block that changes it, the parts, events and actions a recipe can use, and the Homebrew drawer's buttons and waiting rules (owner and co-DM only)",
		Tool: aiplug.Tool{
			Name: "homebrew_workshop", Title: "Homebrew Workshop", Did: "Looked at the Homebrew Workshop", Owner: true,
			Desc: "The Homebrew Workshop: each recipe (its parts and When/Then rules) as a block to send back changed with ask_for_changes, the kinds of part, event and action a recipe can use with the args each takes, and the Homebrew drawer's buttons (such as Log a respite) and the rules waiting there.",
		},
	}
}

// Answer reads the recipes, the catalog and the drawer as the Director.
func (k *AI) Answer(ctx context.Context, campaignID string, a aiplug.Actor, _ aiplug.Record) (string, string, error) {
	chip := "Homebrew Workshop"
	if !a.CanAuthorDmOnly() {
		return chip, "", aiplug.BadRequestf(aiDirectorsOnly)
	}
	v := aiViewer(a)
	list, err := k.svc.List(ctx, campaignID, v)
	if err != nil {
		return chip, "", err
	}
	var b strings.Builder
	b.WriteString("## Homebrew Workshop\n\nA recipe is parts (trackers, clocks and other plugins' records) wired by When/Then rules. Send a recipe's block back, changed, to change it; `how_to_write_changes` explains the `workshop-recipe` block.\n\n### Recipes\n\n")
	if len(list) == 0 {
		b.WriteString("None yet.\n\n")
	}
	for _, s := range list {
		r, err := k.svc.Get(ctx, campaignID, s.ID, v)
		if err != nil {
			return chip, "", err
		}
		block, err := recipeBlock(r.Name, r.Doc)
		if err != nil {
			return chip, "", err
		}
		b.WriteString("```\n" + block + "```\n\n")
	}
	cat, err := k.svc.Catalog(v)
	if err != nil {
		return chip, "", err
	}
	writeCatalog(&b, cat)
	if d, err := k.svc.Drawer(ctx, campaignID, v); err == nil && d != nil {
		writeDrawer(&b, d)
	}
	return fmt.Sprintf("Homebrew Workshop · %d recipes", len(list)), b.String(), nil
}

// writeCatalog lists what a recipe can be built from, with the args each
// event and action takes, so a new recipe is written right the first time.
func writeCatalog(b *strings.Builder, c Catalog) {
	b.WriteString("### What a recipe can use\n\nParts (`kind`):\n")
	for _, p := range c.Parts {
		line := "- `" + p.Name + "`: " + p.Label
		if p.ForceSee != "" {
			line += "; always `see: " + p.ForceSee + "`"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nEvents (`when.event`):\n")
	for _, e := range c.Events {
		b.WriteString("- `" + e.Name + "`: " + e.Label + catalogTail(e) + "\n")
	}
	b.WriteString("\nActions (`then[].action`):\n")
	for _, e := range c.Actions {
		line := "- `" + e.Name + "`: " + e.Label + catalogTail(e)
		switch {
		case e.ClientRun:
			line += "; runs in the Director's browser when they press Run"
		case e.Waits:
			line += "; always waits in the drawer for the Director"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
}

// catalogTail is the part kinds and args an event or action takes.
func catalogTail(e CatalogEntry) string {
	var out string
	if len(e.PartKinds) > 0 {
		out += "; needs `part`, a part of kind " + strings.Join(e.PartKinds, " or ")
	} else {
		out += "; no `part`"
	}
	if len(e.Fields) > 0 {
		var args []string
		for _, f := range e.Fields {
			kind := "text"
			if f.Type == FieldWhole {
				kind = "a whole number"
			}
			args = append(args, "`"+f.Name+"` ("+f.Label+", "+kind+")")
		}
		out += "; args " + strings.Join(args, ", ")
	}
	return out
}

// writeDrawer is the drawer as the Director sees it. Pressing a button or
// running a waiting rule is the Director's own act at the table, so it is
// described, never offered as a change.
func writeDrawer(b *strings.Builder, d *Drawer) {
	b.WriteString("### The Homebrew drawer\n\nThe Director presses these in the drawer at the table; they are not something to ask for.\n\nButtons:\n")
	if len(d.Presses) == 0 {
		b.WriteString("- none\n")
	}
	for _, p := range d.Presses {
		fmt.Fprintf(b, "- %s (`%s`): starts %d saved rules\n", p.Label, p.Event, p.Rules)
	}
	b.WriteString("\nWaiting for the Director:\n")
	if len(d.Waiting) == 0 {
		b.WriteString("- none\n")
	}
	for _, r := range d.Waiting {
		line := "- " + r.Recipe + ": " + r.What
		if r.Waits != "" {
			line += " (" + r.Waits + ")"
		}
		b.WriteString(line + "\n")
	}
	if len(d.Recent) > 0 {
		b.WriteString("\nLately:\n")
		for _, r := range d.Recent {
			b.WriteString("- " + r.Recipe + ": " + r.What + ", " + r.Status + "\n")
		}
	}
}

// recipeBlock writes a recipe as the block that sends it back. Each part
// and rule is one JSON line, which YAML reads as it is, so owner data and
// args come back exactly as they were. The canvas layout is left out.
func recipeBlock(name string, doc Doc) (string, error) {
	var b strings.Builder
	b.WriteString("---\nkind: workshop-recipe\nname: " + strconv.Quote(name) + "\n")
	b.WriteString("parts:")
	if len(doc.Parts) == 0 {
		b.WriteString(" []")
	}
	b.WriteString("\n")
	for _, p := range doc.Parts {
		line, err := json.Marshal(p)
		if err != nil {
			return "", err
		}
		b.WriteString("  - " + string(line) + "\n")
	}
	b.WriteString("rules:")
	if len(doc.Rules) == 0 {
		b.WriteString(" []")
	}
	b.WriteString("\n")
	for _, r := range doc.Rules {
		line, err := json.Marshal(r)
		if err != nil {
			return "", err
		}
		b.WriteString("  - " + string(line) + "\n")
	}
	b.WriteString("---\n")
	return b.String(), nil
}

// ---- block ----

func (*AI) Name() string  { return "workshop-recipe" }
func (*AI) Label() string { return "Homebrew recipe" }
func (*AI) Brief() string {
	return "Homebrew Workshop recipes: trackers, clocks and other parts wired by When/Then rules, such as a button press ticking a counter that raises another every third time"
}

func (*AI) Doc() string {
	return "A Homebrew Workshop recipe: parts wired by When/Then rules. Matched by `name`. Keys: `parts`, a list of `{key, kind, label, see, data}` (`key` is short, lower-case letters, digits and dashes, unique in the recipe; `kind` is a part kind the `homebrew_workshop` tool lists; `see` is `party`, `name` (players see it exists, not its details) or `director`; `data` is the kind's own settings: a `tracker` takes `{shape: count, min, max}`, a `clock` `{max}`), and `rules`, a list of `{when: {event, part, args, test}, timing, then: [{action, part, args}]}` (`event` and `action` from the tool's lists; `part` names a part of this recipe, given exactly when the event or action needs one; `args` holds the fields it lists; `timing` is `now`, `encounter_end` or `held`, which waits in the Homebrew drawer). An update gives the whole recipe: both lists replace what is there, so send back every part and rule you keep. A new name with `rename`. `action: delete` removes the recipe; trackers it kept stay as hand-made trackers. Read the recipes and the lists first with the `homebrew_workshop` tool (`what: workshop`). Only Directors change recipes.\n\n```\n---\nkind: workshop-recipe\nname: The Hunger\nparts:\n  - {key: respites, kind: tracker, label: Respites, see: director, data: {shape: count, min: 0, max: 3}}\n  - {key: hunger, kind: tracker, label: The Hunger, see: party, data: {shape: count, min: 0, max: 5}}\nrules:\n  - when: {event: respite.logged}\n    timing: now\n    then:\n      - {action: step, part: respites, args: {by: 1}}\n  - when: {event: tracker.reached, part: respites, args: {value: 3}}\n    timing: now\n    then:\n      - {action: step, part: hunger, args: {by: 1}}\n      - {action: set, part: respites, args: {to: 0}}\n---\n```"
}

// findRecipe is the campaign's recipe of that name, or nil.
func (k *AI) findRecipe(ctx context.Context, campaignID string, v Viewer, name string) (*Recipe, error) {
	list, err := k.svc.List(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		if aiplug.SameName(s.Name, name) {
			return k.svc.Get(ctx, campaignID, s.ID, v)
		}
	}
	return nil, nil
}

// aiDraft is a record read into the recipe it asks for. has says which of
// parts and rules the record gave.
type aiDraft struct {
	name      string
	doc       Doc
	hasParts  bool
	hasRules  bool
	newName   string
	hasRename bool
}

// readDraft reads a record's keys. Front matter arrives as loose YAML, so
// each list goes through JSON into the Workshop's own types, which refuse a
// shape the Workshop would not save.
func readDraft(r aiplug.Record) (aiDraft, error) {
	d := aiDraft{name: strings.TrimSpace(r.Name)}
	if d.name == "" {
		return d, aiplug.BadRequestf("needs `name`, the recipe's name")
	}
	if raw, ok := r.Fields["parts"]; ok {
		d.hasParts = true
		if err := viaJSON(raw, &d.doc.Parts); err != nil {
			return d, aiplug.BadRequestf("`parts` must be a list of {key, kind, label, see, data}: %v", err)
		}
	}
	if raw, ok := r.Fields["rules"]; ok {
		d.hasRules = true
		if err := viaJSON(raw, &d.doc.Rules); err != nil {
			return d, aiplug.BadRequestf("`rules` must be a list of {when, timing, then}: %v", err)
		}
	}
	if r.Has("rename") {
		d.hasRename = true
		d.newName = strings.TrimSpace(r.Str("rename"))
	}
	if d.doc.Parts == nil {
		d.doc.Parts = []Part{}
	}
	if d.doc.Rules == nil {
		d.doc.Rules = []Rule{}
	}
	return d, nil
}

// viaJSON copies a loosely typed YAML value into out, refusing unknown keys.
func viaJSON(v any, out any) error {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

// input is what the record asks the service to save: for an update, the
// stored recipe with the record's lists and name laid over it.
func (d aiDraft) input(cur *Recipe) RecipeInput {
	if cur == nil {
		in := RecipeInput{Name: d.name, Doc: d.doc}
		if d.hasRename && d.newName != "" {
			in.Name = d.newName
		}
		return in
	}
	in := RecipeInput{Name: cur.Name, Doc: cur.Doc, Version: cur.Version}
	if d.hasParts {
		in.Doc.Parts = d.doc.Parts
	}
	if d.hasRules {
		in.Doc.Rules = d.doc.Rules
	}
	if d.hasRename && d.newName != "" {
		in.Name = d.newName
	}
	return in
}

// seen is the stored state a row was planned against: Apply goes ahead only
// on the same version, so a Workshop edit made during review is not lost.
func seen(cur *Recipe) string {
	if cur == nil {
		return "new"
	}
	return "v" + strconv.Itoa(cur.Version)
}

func (k *AI) Plan(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) aiplug.Plan {
	if !a.CanAuthorDmOnly() {
		return aiplug.Plan{Error: "only the campaign's Directors can change Homebrew Workshop recipes"}
	}
	v := aiViewer(a)
	d, err := readDraft(r)
	if err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	cur, err := k.findRecipe(ctx, campaignID, v, d.name)
	if err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	switch r.Action {
	case aiplug.ActionDelete:
		if cur == nil {
			return aiplug.Plan{Error: fmt.Sprintf("there is no recipe called %q", d.name)}
		}
		return aiplug.Plan{Summary: "Remove the recipe " + strconv.Quote(cur.Name) + "; trackers it kept stay as hand-made trackers", Seen: seen(cur)}
	case aiplug.ActionCreate, aiplug.ActionUpdate, "":
	default:
		return aiplug.Plan{Error: "action must be create, update or delete"}
	}
	if r.Action == aiplug.ActionCreate && cur != nil {
		return aiplug.Plan{Error: fmt.Sprintf("there is already a recipe called %q; use action: update", cur.Name)}
	}
	if r.Action == aiplug.ActionUpdate && cur == nil {
		return aiplug.Plan{Error: fmt.Sprintf("there is no recipe called %q to update", d.name)}
	}
	if cur == nil && !d.hasParts && !d.hasRules {
		return aiplug.Plan{Error: "a new recipe needs `parts` or `rules`"}
	}
	in := d.input(cur)
	id := ""
	if cur != nil {
		id = cur.ID
	}
	if err := k.svc.Check(ctx, campaignID, id, v, in); err != nil {
		return aiplug.Plan{Error: aiplug.PlanError(err)}
	}
	summary := fmt.Sprintf("Add the recipe %q: %s", in.Name, docCounts(in.Doc))
	if cur != nil {
		summary = fmt.Sprintf("Change the recipe %q: %s", cur.Name, docCounts(in.Doc))
		if in.Name != cur.Name {
			summary += ", renamed " + strconv.Quote(in.Name)
		}
	}
	return aiplug.Plan{Summary: summary, Warnings: planWarnings(cur, d, in.Doc), Seen: seen(cur)}
}

func docCounts(doc Doc) string {
	return fmt.Sprintf("%d parts, %d rules", len(doc.Parts), len(doc.Rules))
}

// planWarnings says what an update drops, since its lists replace the
// stored ones whole.
func planWarnings(cur *Recipe, d aiDraft, doc Doc) []string {
	if cur == nil {
		return nil
	}
	var out []string
	if d.hasParts {
		kept := map[string]bool{}
		for _, p := range doc.Parts {
			kept[p.Key] = true
		}
		for _, p := range cur.Doc.Parts {
			if !kept[p.Key] {
				out = append(out, "removes the part "+strconv.Quote(p.Label)+" ("+p.Key+")")
			}
		}
	}
	if d.hasRules && len(doc.Rules) < len(cur.Doc.Rules) {
		out = append(out, fmt.Sprintf("leaves %d of its %d rules", len(doc.Rules), len(cur.Doc.Rules)))
	}
	return out
}

func (k *AI) Apply(ctx context.Context, campaignID string, a aiplug.Actor, r aiplug.Record) error {
	p := k.Plan(ctx, campaignID, a, r)
	if p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	if r.Seen != "" && r.Seen != p.Seen {
		return apperror.NewBadRequest("the recipe changed in the Workshop while you were reviewing; check it again")
	}
	v := aiViewer(a)
	d, err := readDraft(r)
	if err != nil {
		return err
	}
	cur, err := k.findRecipe(ctx, campaignID, v, d.name)
	if err != nil {
		return err
	}
	if r.Action == aiplug.ActionDelete {
		if cur == nil {
			return apperror.NewBadRequest(fmt.Sprintf("there is no recipe called %q", d.name))
		}
		return k.svc.Delete(ctx, campaignID, cur.ID, v)
	}
	in := d.input(cur)
	if cur == nil {
		_, err = k.svc.Create(ctx, campaignID, v, in)
		return err
	}
	_, err = k.svc.Update(ctx, campaignID, cur.ID, v, in)
	return err
}

// Export lists the recipes in the block's own format.
func (k *AI) Export(ctx context.Context, campaignID string, a aiplug.Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	v := aiViewer(a)
	list, err := k.svc.List(ctx, campaignID, v)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range list {
		r, err := k.svc.Get(ctx, campaignID, s.ID, v)
		if err != nil {
			return "", err
		}
		block, err := recipeBlock(r.Name, r.Doc)
		if err != nil {
			return "", err
		}
		b.WriteString(block)
	}
	return b.String(), nil
}
