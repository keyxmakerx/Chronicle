package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// How a material ties the parts of crafting together, said once in every
// crafting kind's doc so an AI never invents a link Chronicle doesn't keep.
const craftingTies = "A material is an Armory item with one or more kinds (`kind: material`). A recipe asks for it by the item's name (`2 Troll tooth`) or for any item of a kind (`1 any herb`), which whatever the hero carries of that kind fills. A creature, Remnant, place or magic source gives it through its drop list (`kind: drops`): when a fight ends, each defeated copy drops its list into the party's spoils tray, where heroes take or harvest it. `what: crafting` shows all of it."

// ---- recipe ----

// RecipeKind writes the campaign's crafting recipes, matched by name. A new
// recipe arrives as a draft the Director approves in Crafting; its steps
// are laid by a method, and the rules editor's rules stay the Director's.
type RecipeKind struct{ Svc CraftingAPI }

func (RecipeKind) Name() string  { return "recipe" }
func (RecipeKind) Label() string { return "Recipe" }
func (RecipeKind) Doc() string {
	return "A crafting recipe, matched by `name`. Keys: `makes` (the Armory item it makes, with how many first: `1 Healing potion`), " +
		"`materials` (a list, one line each: `2 Troll tooth` for an item, `1 any herb` for any item of a kind), " +
		"`method` (the steps a hero plays at the bench: `steady`, a brew that ends on the heat, or `volatile`, one that ends on a timing bar; it adds an `any` line for a step that has no material to use), " +
		"`goal` (the project's points, required on create), `characteristic` (what the project roll adds; `what: crafting` lists the system's), `prerequisite` and `source` (the system's words for what the item needs and where its knowledge comes from), " +
		"`cost` (coins on top of the materials, like `25 gp`; a hero who keeps Wealth needs it instead), `who` (`everyone`, the default, or `learned`) and `known_by` (the characters who learned it; naming them makes it `learned`), `rename_to`. " +
		"A new recipe arrives as a draft players don't see until the Director approves it in Crafting. On update, a key you leave out keeps what is there; `materials` and `known_by` replace the whole list, and without `method` the steps stay as they are, each on the same material. " +
		"The \"When… then…\" rules, the bench colours and the exact steps are set in Crafting and an update keeps them. Removing a recipe leaves heroes' projects from it going.\n\n" + craftingTies + "\n\n" +
		"```\n---\nkind: recipe\nname: Troll-blood tonic\nmakes: 1 Troll-blood tonic\nmaterials:\n  - 2 Troll tooth\n  - 1 any liquid\nmethod: steady\ngoal: 30\ncharacteristic: Reason\ncost: 25 gp\nwho: learned\nknown_by: [Mira]\n---\n```"
}

// input reads the block as a whole recipe: the stored one with the written
// keys over it. pending names the items the paste makes.
func (k RecipeKind) input(c *Crafting, r Record, cur *CraftRecipe) (CraftRecipeInput, []string, error) {
	in := CraftRecipeInput{Name: strings.TrimSpace(r.Name), ResultQty: 1, Who: "all"}
	if cur != nil {
		in = CraftRecipeInput{ID: cur.ID, Name: cur.Name, Materials: cur.Materials, ResultID: cur.ResultID, ResultQty: cur.ResultQty,
			Goal: cur.Goal, Stat: cur.Stat, Prerequisite: cur.Prerequisite, Source: cur.Source, Cost: cur.Cost, Currency: cur.Currency,
			Who: cur.Who, Known: cur.Known}
	}
	var pending []string
	note := func(id *string, err error) error {
		if p, ok := err.(errPending); ok {
			pending = append(pending, p.name)
			*id = pendingID(p.name)
			return nil
		}
		return err
	}
	if r.Action == ActionUpdate && r.Has("rename_to") {
		in.Name = r.Str("rename_to")
	}
	if r.Has("makes") {
		in.ResultID, in.ResultQty = "", 1
		if s := r.Str("makes"); s != "" && !isNone(s) {
			q, name := countName(s)
			if q < 1 || q > 99 {
				return in, nil, badRequestf("makes: write 1 to 99 before the item")
			}
			id, err := c.item(name, r.Made)
			if err = note(&id, err); err != nil {
				return in, nil, badRequestf("makes: %s", planError(err))
			}
			in.ResultID, in.ResultQty = id, q
		}
	}
	if r.Has("materials") {
		in.Materials = nil
		for _, raw := range r.List("materials") {
			m, err := c.material(raw, r.Made)
			if err = note(&m.ItemID, err); err != nil {
				return in, nil, err
			}
			in.Materials = append(in.Materials, m)
		}
	}
	if r.Has("method") {
		s := r.Str("method")
		ch, ok := choice(c.Methods, s)
		if !ok {
			return in, nil, badRequestf("method: %q is not a method; the methods are %s", s, choiceKeys(c.Methods))
		}
		in.Method = ch.Key
	}
	if r.Has("goal") {
		n, _, err := r.Int("goal")
		if err != nil {
			return in, nil, err
		}
		if n < 1 || n > 999 {
			return in, nil, badRequestf("goal: 1 to 999 points")
		}
		in.Goal = n
	} else if cur == nil {
		return in, nil, badRequestf("a new recipe needs `goal`: the project's points")
	}
	if r.Has("characteristic") {
		s, err := c.stat("characteristic", r.Str("characteristic"))
		if err != nil {
			return in, nil, err
		}
		in.Stat = s
	}
	for _, key := range []string{"prerequisite", "source"} {
		if !r.Has(key) {
			continue
		}
		s := r.Str(key)
		if isNone(s) {
			s = ""
		}
		if key == "prerequisite" {
			in.Prerequisite = s
		} else {
			in.Source = s
		}
	}
	if r.Has("cost") {
		n, cur, err := readCost(r.Str("cost"))
		if err != nil {
			return in, nil, err
		}
		in.Cost, in.Currency = n, cur
	}
	if r.Has("who") {
		switch w := strings.ToLower(r.Str("who")); w {
		case "everyone", "all", "":
			in.Who = "all"
		case "learned", "must be learned":
			in.Who = "learned"
		default:
			return in, nil, badRequestf("who: %q is everyone or learned", w)
		}
	}
	if r.Has("known_by") {
		in.Known = nil
		for _, raw := range r.List("known_by") {
			id, err := c.character(fmt.Sprint(raw))
			if err != nil {
				return in, nil, badRequestf("known_by: %s", planError(err))
			}
			in.Known = append(in.Known, id)
		}
		if len(in.Known) > 0 && !r.Has("who") {
			in.Who = "learned"
		}
	}
	return in, pending, nil
}

func choiceKeys(list []CraftChoice) string {
	parts := make([]string, len(list))
	for i, c := range list {
		parts[i] = c.Key
	}
	return strings.Join(parts, ", ")
}

// recipeWords says what a recipe is in one line: "makes 1 × Healing potion
// from 2 Bloodmoss and 1 any liquid; goal 30; 2d10 + Reason".
func recipeWords(c *Crafting, in CraftRecipeInput, pending []string) string {
	name := c.itemWords
	var parts []string
	what := "makes nothing yet"
	if in.ResultID != "" {
		what = fmt.Sprintf("makes %d × %s", in.ResultQty, name(in.ResultID))
	}
	if len(in.Materials) > 0 {
		var ms []string
		for _, m := range in.Materials {
			if m.Tag != "" {
				ms = append(ms, fmt.Sprintf("%d any %s", m.Qty, strings.ToLower(c.tagLabel(m.Tag))))
			} else {
				ms = append(ms, fmt.Sprintf("%d %s", m.Qty, name(m.ItemID)))
			}
		}
		what += " from " + joinAnd(ms)
	}
	parts = append(parts, what)
	parts = append(parts, fmt.Sprintf("goal %d", in.Goal))
	roll := c.Dice
	if in.Stat != "" {
		roll += " + " + c.statLabel(in.Stat)
	}
	if roll != "" {
		parts = append(parts, roll)
	}
	if in.Method != "" {
		parts = append(parts, "a "+strings.ToLower(choiceLabel(c.Methods, in.Method)))
	}
	if in.Cost > 0 {
		parts = append(parts, "costs "+money(in.Cost, in.Currency))
	}
	if in.Who == "learned" {
		var who []string
		for _, id := range in.Known {
			who = append(who, c.characterName(id))
		}
		s := "must be learned"
		if len(who) > 0 {
			s += " (known by " + joinAnd(who) + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// joinAnd lists words as people write them: "a, b and c".
func joinAnd(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// directorsOnly is the crafting kinds' gate: the app's own rule, Owner or
// co-Director.
func directorsOnly(a Actor, what string) string {
	if a.CanAuthorDmOnly() {
		return ""
	}
	return "only the campaign's Directors can change its " + what
}

func pendingWarnings(names []string) []string {
	var out []string
	for _, n := range names {
		out = append(out, quote(n)+" is made in this paste, so this is checked again when it saves")
	}
	return out
}

func (k RecipeKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, _, _, _ := k.plan(ctx, campaignID, a, r)
	return p
}

// plan also returns the items the paste makes that it names; Apply runs
// after the paste's pages are saved, so one still pending there was not
// made.
func (k RecipeKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (Plan, *CraftRecipe, CraftRecipeInput, []string) {
	if e := directorsOnly(a, "recipes"); e != "" {
		return Plan{Error: e}, nil, CraftRecipeInput{}, nil
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return Plan{Error: "a recipe needs `name`"}, nil, CraftRecipeInput{}, nil
	}
	c, err := k.Svc.Read(ctx, campaignID, a)
	if err != nil {
		return Plan{Error: planError(err)}, nil, CraftRecipeInput{}, nil
	}
	cur := c.recipe(name)
	switch r.Action {
	case ActionCreate:
		if cur != nil {
			return Plan{Error: "there is already a recipe called " + quote(cur.Name) + "; use action: update"}, nil, CraftRecipeInput{}, nil
		}
	default:
		if cur == nil {
			return Plan{Error: "there is no recipe called " + quote(name)}, nil, CraftRecipeInput{}, nil
		}
	}
	if r.Action == ActionDelete {
		return Plan{Summary: "removes the recipe; heroes' projects from it carry on"}, cur, CraftRecipeInput{}, nil
	}
	in, pending, err := k.input(c, r, cur)
	if err != nil {
		return Plan{Error: planError(err)}, cur, in, nil
	}
	p := Plan{Warnings: pendingWarnings(pending)}
	if len(pending) == 0 {
		if err := k.Svc.SaveRecipe(ctx, campaignID, a, in, true); err != nil {
			return Plan{Error: planError(err)}, cur, in, nil
		}
	}
	p.Summary = recipeWords(c, in, pending)
	if cur == nil {
		p.Summary = "a draft players see once you approve it in Crafting: " + p.Summary
		if in.Method == "" {
			p.Warnings = append(p.Warnings, "No `method`, so it has no steps yet: pick them in Crafting, or write method: "+choiceKeys(c.Methods))
		}
	} else {
		if in.Name != cur.Name {
			p.Summary = "renamed " + quote(in.Name) + "; " + p.Summary
		}
		p.Summary = "now " + p.Summary
	}
	return p, cur, in, pending
}

func (k RecipeKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, cur, in, pending := k.plan(ctx, campaignID, a, r)
	if p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	if len(pending) > 0 {
		return badRequestf("%s was not made, so the recipe can't name it", quote(pending[0]))
	}
	if r.Action == ActionDelete {
		return k.Svc.DeleteRecipe(ctx, campaignID, cur.ID, a)
	}
	return k.Svc.SaveRecipe(ctx, campaignID, a, in, false)
}

// Export lists every recipe as a recipe block, for a Director.
func (k RecipeKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	c, err := k.Svc.Read(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, rec := range c.Recipes {
		b.WriteString(recipeBlock(c, rec) + "\n")
	}
	return b.String(), nil
}

// recipeBlock writes a recipe in the recipe block's own keys, so the AI can
// send it back changed.
func recipeBlock(c *Crafting, rec CraftRecipe) string {
	var b strings.Builder
	b.WriteString("---\nkind: recipe\nname: " + quote(rec.Name) + "\n")
	if rec.ResultID != "" {
		if n := c.itemName(rec.ResultID); n != "" {
			b.WriteString("makes: " + quote(fmt.Sprintf("%d %s", rec.ResultQty, n)) + "\n")
		}
	}
	var mats []string
	for _, m := range rec.Materials {
		switch {
		case m.Tag != "":
			mats = append(mats, fmt.Sprintf("%d any %s", m.Qty, strings.ToLower(c.tagLabel(m.Tag))))
		case c.itemName(m.ItemID) != "":
			mats = append(mats, fmt.Sprintf("%d %s", m.Qty, c.itemName(m.ItemID)))
		}
	}
	if len(mats) > 0 {
		b.WriteString("materials:\n")
		for _, m := range mats {
			b.WriteString("  - " + quote(m) + "\n")
		}
	}
	if rec.Method != "" {
		b.WriteString("method: " + rec.Method + "\n")
	}
	fmt.Fprintf(&b, "goal: %d\n", rec.Goal)
	if rec.Stat != "" {
		b.WriteString("characteristic: " + quote(c.statLabel(rec.Stat)) + "\n")
	}
	if rec.Prerequisite != "" {
		b.WriteString("prerequisite: " + quote(rec.Prerequisite) + "\n")
	}
	if rec.Source != "" {
		b.WriteString("source: " + quote(rec.Source) + "\n")
	}
	if rec.Cost > 0 {
		b.WriteString("cost: " + quote(money(rec.Cost, rec.Currency)) + "\n")
	}
	if rec.Who == "learned" {
		b.WriteString("who: learned\n")
		var who []string
		for _, id := range rec.Known {
			if n := c.characterName(id); n != "" {
				who = append(who, quote(n))
			}
		}
		if len(who) > 0 {
			b.WriteString("known_by: [" + strings.Join(who, ", ") + "]\n")
		}
	}
	b.WriteString("---\n")
	return b.String()
}

// recipeNotes are the lines said before a recipe's block: what the block
// can't carry, and what an update keeps.
func recipeNotes(c *Crafting, rec CraftRecipe) []string {
	var out []string
	if rec.Draft {
		out = append(out, "a draft, which players don't see until a Director approves it in Crafting")
	}
	if rec.ByHand {
		out = append(out, "its steps were changed by hand in Crafting, and an update without `method` keeps them")
	} else if rec.Method == "" && rec.Steps > 0 {
		out = append(out, fmt.Sprintf("its %s were set by hand in Crafting, and an update without `method` keeps them", plural(rec.Steps, "step", "steps")))
	}
	if rec.Rules > 0 {
		out = append(out, fmt.Sprintf("%s in Crafting's rules editor, which an update keeps", plural(rec.Rules, "rule", "rules")))
	}
	gone := 0
	for _, m := range rec.Materials {
		if m.ItemID != "" && c.itemName(m.ItemID) == "" {
			gone++
		}
	}
	if rec.ResultID != "" && c.itemName(rec.ResultID) == "" {
		gone++
	}
	if gone > 0 {
		out = append(out, fmt.Sprintf("%s it names were deleted from the Armory, so the block leaves them out", plural(gone, "item", "items")))
	}
	return out
}

// ---- material ----

// MaterialKind sets an Armory item's material kinds, matched by `item`.
type MaterialKind struct{ Svc CraftingAPI }

func (MaterialKind) Name() string  { return "material" }
func (MaterialKind) Label() string { return "Material" }
func (MaterialKind) Doc() string {
	return "Makes an Armory item a crafting material, matched by `item` (the item's page name, which must already be an Armory item or be made in the paste). Key: `kinds`, the kinds of material it is (`what: crafting` lists them; for example `[bone, remnant part]`). An update replaces the whole list. Removing it leaves the item but makes it no material: recipes that ask for any item of its kinds stop taking it.\n\n" + craftingTies + "\n\n" +
		"```\n---\nkind: material\nitem: Troll tooth\nkinds: [bone, remnant part]\n---\n```"
}

// Title names the review row: "Troll tooth as a material".
func (MaterialKind) Title(r Record) string {
	if n := materialItem(r); n != "" {
		return n + " as a material"
	}
	return ""
}

func materialItem(r Record) string { return firstNonEmpty(r.Str("item"), r.Name) }

func (k MaterialKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, _, _ := k.plan(ctx, campaignID, a, r)
	return p
}

func (k MaterialKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (Plan, string, []string) {
	if e := directorsOnly(a, "materials"); e != "" {
		return Plan{Error: e}, "", nil
	}
	name := materialItem(r)
	if name == "" {
		return Plan{Error: "a material needs `item`: the Armory item it is"}, "", nil
	}
	c, err := k.Svc.Read(ctx, campaignID, a)
	if err != nil {
		return Plan{Error: planError(err)}, "", nil
	}
	var tags []string
	if r.Action != ActionDelete {
		raw := r.List("kinds")
		if len(raw) == 0 {
			raw = r.List("tags")
		}
		seen := map[string]bool{}
		for _, x := range raw {
			ch, ok := choice(c.Tags, fmt.Sprint(x))
			if !ok {
				return Plan{Error: fmt.Sprintf("kinds: %q is not a kind of material; the kinds are %s", fmt.Sprint(x), choiceWords(c.Tags))}, "", nil
			}
			if !seen[ch.Key] {
				seen[ch.Key] = true
				tags = append(tags, ch.Key)
			}
		}
		if len(tags) == 0 {
			return Plan{Error: "it needs `kinds`, the kinds of material it is; to make it no material, use action: delete"}, "", nil
		}
	}
	id, err := c.item(name, r.Made)
	if p, ok := err.(errPending); ok && r.Action == ActionCreate {
		return Plan{Summary: "makes the new item a material: " + c.tagLabels(tags), Warnings: pendingWarnings([]string{p.name})}, "", tags
	}
	if err != nil {
		return Plan{Error: planError(err)}, "", nil
	}
	have := c.itemTags(id)
	switch r.Action {
	case ActionCreate:
		if len(have) > 0 {
			return Plan{Error: fmt.Sprintf("%s is already a material (%s); use action: update", c.itemName(id), c.tagLabels(have))}, "", nil
		}
	default:
		if len(have) == 0 {
			return Plan{Error: c.itemName(id) + " is not a material yet; use action: create"}, "", nil
		}
	}
	if r.Action == ActionDelete {
		s := c.itemName(id) + " is no longer a material"
		if used := c.usedBy(id, have); len(used) > 0 {
			s += "; " + joinAnd(used) + " ask for it or its kind"
		}
		return Plan{Summary: s}, id, nil
	}
	s := c.tagLabels(tags)
	if r.Action == ActionUpdate {
		s = "now " + s
	}
	p := Plan{Summary: s}
	return p, id, tags
}

func (k MaterialKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, id, tags := k.plan(ctx, campaignID, a, r)
	if p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	if id == "" {
		return badRequestf("no Armory item called %s", quote(materialItem(r)))
	}
	return k.Svc.SetTags(ctx, campaignID, id, a, tags)
}

// Export lists every material as a material block, for a Director.
func (k MaterialKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	c, err := k.Svc.Read(ctx, campaignID, a)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, it := range c.Items {
		if len(it.Tags) > 0 {
			b.WriteString(materialBlock(c, it) + "\n")
		}
	}
	return b.String(), nil
}

func materialBlock(c *Crafting, it CraftItem) string {
	kinds := make([]string, len(it.Tags))
	for i, t := range it.Tags {
		kinds[i] = strings.ToLower(c.tagLabel(t))
	}
	return "---\nkind: material\nitem: " + quote(it.Name) + "\nkinds: [" + strings.Join(kinds, ", ") + "]\n---\n"
}

// ---- drops ----

// DropsKind writes a page's drop list: what a creature, Remnant, place or
// magic source leaves in the spoils tray. Matched by `page`.
type DropsKind struct {
	Svc      CraftingAPI
	Entities EntityLookup
}

func (DropsKind) Name() string  { return "drops" }
func (DropsKind) Label() string { return "Drop list" }
func (DropsKind) Doc() string {
	return "What a page leaves when it is defeated, harvested or searched: a creature, a Remnant, a place, a magic source. Matched by `page` (the page's name). Keys: `lines` (a list, one line each, every item an Armory item: `2 Troll tooth` is taken as it is; `harvest Troll hide with Might: 1/2/4` is a test of the system's dice plus that characteristic, and the three numbers are what its three outcomes pay, lowest first, the lowest at least 1; leave out `with …` for a test that adds nothing), " +
		"`also` (another page whose list drops first, such as the creature a Remnant came from; `none` takes it away). At most 20 lines. On update, `lines` replaces the whole list and a key you leave out keeps what is there. Removing it takes the list away; spoils already in the tray stay.\n\n" + craftingTies + "\n\n" +
		"```\n---\nkind: drops\npage: Troll\nlines:\n  - 2 Troll tooth\n  - harvest Troll hide with Might: 1/2/4\n---\n```\n\n" +
		"```\n---\nkind: drops\npage: Troll Remnant\nalso: Troll\nlines:\n  - 1 Remnant ash\n---\n```"
}

// Title names the review row: "Drops of Troll".
func (DropsKind) Title(r Record) string {
	if p := dropsPage(r); p != "" {
		return "Drops of " + p
	}
	return ""
}

func dropsPage(r Record) string { return firstNonEmpty(r.Str("page"), r.Name) }

func (k DropsKind) page(ctx context.Context, campaignID, name string) *entities.Entity {
	e, err := k.Entities.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID || !sameName(e.Name, name) {
		return nil
	}
	return e
}

func (k DropsKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	p, _, _ := k.plan(ctx, campaignID, a, r)
	return p
}

func (k DropsKind) plan(ctx context.Context, campaignID string, a Actor, r Record) (Plan, string, CraftDropsInput) {
	var none CraftDropsInput
	if e := directorsOnly(a, "drop lists"); e != "" {
		return Plan{Error: e}, "", none
	}
	name := dropsPage(r)
	if name == "" {
		return Plan{Error: "a drop list needs `page`: the page that drops it"}, "", none
	}
	c, err := k.Svc.Read(ctx, campaignID, a)
	if err != nil {
		return Plan{Error: planError(err)}, "", none
	}
	if c.DropsOff {
		return Plan{Error: "drop lists can't be changed on this server right now"}, "", none
	}
	e := k.page(ctx, campaignID, name)
	if e == nil && !(r.Action == ActionCreate && r.Made[entities.Slugify(name)]) {
		return Plan{Error: "no page called " + quote(name)}, "", none
	}
	var cur *CraftDrops
	if e != nil {
		cur = c.dropsOf(e.ID)
	}
	switch r.Action {
	case ActionCreate:
		if cur != nil {
			return Plan{Error: e.Name + " already has a drop list; use action: update"}, "", none
		}
	default:
		if cur == nil {
			return Plan{Error: name + " has no drop list"}, "", none
		}
	}
	if r.Action == ActionDelete {
		return Plan{Summary: "takes the drop list away; spoils already in the tray stay"}, e.ID, none
	}
	in := CraftDropsInput{}
	if cur != nil {
		in = CraftDropsInput{AlsoID: cur.AlsoID, Lines: cur.Lines, Version: cur.Version}
	}
	var pending []string
	if e == nil {
		pending = append(pending, name)
	}
	if r.Has("lines") {
		in.Lines = nil
		for _, raw := range r.List("lines") {
			l, err := c.dropLine(raw, r.Made)
			if p, ok := err.(errPending); ok {
				pending = append(pending, p.name)
				l.ItemID = pendingID(p.name)
			} else if err != nil {
				return Plan{Error: planError(err)}, "", none
			}
			in.Lines = append(in.Lines, l)
		}
	}
	also := ""
	if r.Has("also") {
		in.AlsoID = ""
		if s := r.Str("also"); s != "" && !isNone(s) {
			ap := k.page(ctx, campaignID, s)
			if ap == nil {
				return Plan{Error: "also: no page called " + quote(s)}, "", none
			}
			if e != nil && ap.ID == e.ID {
				return Plan{Error: "also: a page can't also drop its own list"}, "", none
			}
			in.AlsoID, also = ap.ID, ap.Name
		}
	} else if cur != nil {
		also = cur.Also
	}
	if len(in.Lines) == 0 && in.AlsoID == "" {
		return Plan{Error: "it needs `lines`, or `also`: a page whose list it drops"}, "", none
	}
	p := Plan{Warnings: pendingWarnings(pending)}
	if len(pending) == 0 {
		if err := k.Svc.SaveDrops(ctx, campaignID, e.ID, a, in, true); err != nil {
			return Plan{Error: planError(err)}, "", none
		}
	}
	p.Summary = dropWords(c, in.Lines, also)
	if r.Action == ActionUpdate {
		p.Summary = "now " + p.Summary
	}
	id := ""
	if e != nil {
		id = e.ID
	}
	return p, id, in
}

func (k DropsKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	p, id, in := k.plan(ctx, campaignID, a, r)
	if p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	if id == "" {
		return badRequestf("no page called %s", quote(dropsPage(r)))
	}
	if r.Action == ActionDelete {
		return k.Svc.DeleteDrops(ctx, campaignID, id, a)
	}
	// The paste's pages are saved by now, so an item still pending was not
	// made.
	for _, l := range in.Lines {
		if strings.HasPrefix(l.ItemID, pendingPrefix) {
			return badRequestf("%s was not made, so the drop list can't name it", quote(l.ItemID[len(pendingPrefix):]))
		}
	}
	return k.Svc.SaveDrops(ctx, campaignID, id, a, in, false)
}

// dropWords says a drop list in one line: "drops 2 × Troll tooth; harvest
// Troll hide (Might: 1, 2 or 4); and what Troll drops".
func dropWords(c *Crafting, lines []CraftDropLine, also string) string {
	var parts []string
	for _, l := range lines {
		parts = append(parts, dropLineText(c, l, false))
	}
	s := "drops " + strings.Join(parts, "; ")
	if len(parts) == 0 {
		s = "drops nothing of its own"
	}
	if also != "" {
		s += "; and what " + also + " drops"
	}
	return s
}

// dropLineText writes one line as the block reads it (block) or in words.
func dropLineText(c *Crafting, l CraftDropLine, block bool) string {
	name := c.itemWords(l.ItemID)
	if !l.Harvest {
		if block {
			return fmt.Sprintf("%d %s", l.Qty, name)
		}
		return fmt.Sprintf("%d × %s", l.Qty, name)
	}
	if block {
		s := "harvest " + name
		if l.Stat != "" {
			s += " with " + c.statLabel(l.Stat)
		}
		return fmt.Sprintf("%s: %d/%d/%d", s, l.Yields[0], l.Yields[1], l.Yields[2])
	}
	test := c.Dice
	if l.Stat != "" {
		test += " + " + c.statLabel(l.Stat)
	}
	return fmt.Sprintf("harvest %s (%s: %d, %d or %d)", name, strings.TrimSpace(test), l.Yields[0], l.Yields[1], l.Yields[2])
}

// Export lists every drop list as a drops block, for a Director.
func (k DropsKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	c, err := k.Svc.Read(ctx, campaignID, a)
	if err != nil || c.DropsOff {
		return "", err
	}
	var b strings.Builder
	for _, d := range c.Drops {
		b.WriteString(dropsBlock(c, d) + "\n")
	}
	return b.String(), nil
}

func dropsBlock(c *Crafting, d CraftDrops) string {
	var b strings.Builder
	b.WriteString("---\nkind: drops\npage: " + quote(d.Page) + "\n")
	if d.Also != "" {
		b.WriteString("also: " + quote(d.Also) + "\n")
	}
	var lines []string
	for _, l := range d.Lines {
		if c.itemName(l.ItemID) != "" {
			lines = append(lines, dropLineText(c, l, true))
		}
	}
	if len(lines) > 0 {
		b.WriteString("lines:\n")
		for _, l := range lines {
			b.WriteString("  - " + quote(l) + "\n")
		}
	}
	b.WriteString("---\n")
	return b.String()
}
