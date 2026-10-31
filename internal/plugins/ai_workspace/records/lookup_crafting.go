package records

import (
	"fmt"
	"strings"
)

// Caps that keep the crafting answer inside one lookup's size: the lines
// of each list in the overview, and the item looks shown at once.
const (
	maxCraftLines = 40
	maxLooksShown = 30
)

// craftingParts are the parts `what: crafting` takes, each written as the
// blocks that change it.
var craftingParts = []string{"recipes", "materials", "drops", "looks"}

// crafting answers `what: crafting`. With no `part` it is the overview:
// the rules, and for a Director every recipe, material, drop list, the
// tray, projects and benches in a line each. A part writes that part as
// blocks the AI can send back changed; `name` narrows it to one. A viewer
// who is no Director reads the recipe book as players do.
func (run *lookupRun) crafting(part, name string) (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Crafting"}
	part = strings.ToLower(strings.TrimSpace(part))
	switch part {
	case "", "recipes", "materials", "drops", "looks":
	case "recipe":
		part = "recipes"
	case "material":
		part = "materials"
	case "drop-lists", "drop lists", "drop":
		part = "drops"
	case "item-looks", "look":
		part = "looks"
	default:
		return ans, badRequestf("part: %q is not a part of crafting; the parts are %s", part, strings.Join(craftingParts, ", "))
	}
	if part == "looks" {
		return run.craftLooks()
	}
	if run.l.Crafting == nil {
		ans.Text = "## Crafting\n\nCrafting can't be looked up on this server right now.\n"
		return ans, nil
	}
	c, err := run.l.Crafting.Read(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	if !c.Director {
		if part != "" {
			return ans, badRequestf("this connection sees what players see, so it reads only the recipe book; the blocks are the Director's")
		}
		return run.craftBook(c)
	}
	switch part {
	case "recipes":
		return craftRecipes(c, name)
	case "materials":
		return craftMaterials(c, name)
	case "drops":
		return craftDrops(c, name)
	}
	if name != "" {
		if c.recipe(name) != nil {
			return craftRecipes(c, name)
		}
		for _, d := range c.Drops {
			if sameName(d.Page, name) {
				return craftDrops(c, name)
			}
		}
		return ans, badRequestf("no recipe or drop list called %s; leave out `name` to see them all", quote(name))
	}
	return run.craftOverview(c)
}

// craftRules writes the system's rules and the campaign's defaults.
func craftRules(b *strings.Builder, c *Crafting, director bool) {
	b.WriteString("### The rules\n\n")
	roll := "Rolls use " + c.Dice
	if len(c.Stats) > 0 {
		labels := make([]string, len(c.Stats))
		for i, s := range c.Stats {
			labels[i] = s.Label
		}
		roll += " plus one characteristic: " + strings.Join(labels, ", ")
	}
	if c.System != "" {
		roll += " (from " + c.System + ")"
	}
	b.WriteString(roll + ".")
	if len(c.Tiers) == 3 {
		b.WriteString(" A harvest's three outcomes: " + strings.Join(c.Tiers, ", ") + ".")
	}
	fmt.Fprintf(b, " The system calls a project's points its %s, what an item needs its %s, and where its knowledge comes from its %s.\n",
		quote(c.Words.Goal), quote(c.Words.Prerequisite), quote(c.Words.Source))
	if director {
		s := c.Settings
		look, timing := "desk", "standard"
		if s.Look == "journal" {
			look = "journal"
		}
		if s.Difficulty == "easy" || s.Difficulty == "hard" {
			timing = s.Difficulty
		}
		bane, skip := "gives no bane", "may not"
		if s.Bane {
			bane = "gives a bane"
		}
		if s.Skip {
			skip = "may"
		}
		fmt.Fprintf(b, "\nDefaults: players start in the %s look; the timing is %s; botched work %s; players %s skip the steps and just roll.\n", look, timing, bane, skip)
	}
	b.WriteString("\nKinds of material: " + choiceWords(c.Tags) + ".\n")
	if len(c.Methods) > 0 {
		var ms []string
		for _, m := range c.Methods {
			ms = append(ms, "`"+m.Key+"` ("+m.Label+")")
		}
		b.WriteString("Methods a recipe's steps follow: " + strings.Join(ms, ", ") + ".\n")
	}
}

// listLines writes up to maxCraftLines lines, then says what to ask for
// the rest.
func listLines(b *strings.Builder, lines []string, rest string) {
	for i, l := range lines {
		if i >= maxCraftLines {
			fmt.Fprintf(b, "- …and %d more; %s\n", len(lines)-i, rest)
			return
		}
		b.WriteString("- " + l + "\n")
	}
}

func (run *lookupRun) craftOverview(c *Crafting) (LookupAnswer, error) {
	var b strings.Builder
	b.WriteString("## Crafting\n\n")
	if c.Off {
		b.WriteString("Crafting is switched off in Game & features, so players can't see it and fights drop nothing; what it keeps is below.\n\n")
	}
	craftRules(&b, c, true)

	fmt.Fprintf(&b, "\n### Recipes (%d)\n\n", len(c.Recipes))
	if len(c.Recipes) == 0 {
		b.WriteString("No recipes yet.\n")
	}
	var lines []string
	for _, rec := range c.Recipes {
		in := CraftRecipeInput{Materials: rec.Materials, ResultID: rec.ResultID, ResultQty: rec.ResultQty, Goal: rec.Goal, Stat: rec.Stat,
			Method: rec.Method, Cost: rec.Cost, Currency: rec.Currency, Who: rec.Who, Known: rec.Known}
		l := rec.Name
		if rec.Draft {
			l += " (a draft)"
		}
		lines = append(lines, l+": "+recipeWords(c, in, nil))
	}
	listLines(&b, lines, "ask `part: recipes`")

	var mats []CraftItem
	for _, it := range c.Items {
		if len(it.Tags) > 0 {
			mats = append(mats, it)
		}
	}
	fmt.Fprintf(&b, "\n### Materials (%s)\n\n", plural(len(mats), "item", "items"))
	if len(mats) == 0 {
		b.WriteString("No Armory item is a material yet.\n")
	}
	lines = nil
	for _, it := range mats {
		lines = append(lines, materialLine(c, it))
	}
	listLines(&b, lines, "ask `part: materials`")
	if c.ItemsCut {
		b.WriteString("(The campaign has more items than were read.)\n")
	}

	if c.DropsOff {
		b.WriteString("\n### Drop lists and the spoils tray\n\nThey can't be read on this server right now.\n")
	} else {
		fmt.Fprintf(&b, "\n### Drop lists (%d)\n\n", len(c.Drops))
		if len(c.Drops) == 0 {
			b.WriteString("No page has a drop list yet.\n")
		}
		lines = nil
		for _, d := range c.Drops {
			lines = append(lines, d.Page+": "+dropWords(c, d.Lines, d.Also))
		}
		listLines(&b, lines, "ask `part: drops`")

		waiting := 0
		for _, s := range c.Tray {
			waiting += len(s.Lines)
		}
		fmt.Fprintf(&b, "\n### The spoils tray (%s waiting)\n\n", plural(waiting, "thing", "things"))
		if waiting == 0 {
			b.WriteString("Nothing is waiting.\n")
		}
		lines = nil
		for _, s := range c.Tray {
			l := s.Source
			if s.Copies > 1 {
				l += fmt.Sprintf(" (%d of %d)", s.Copy, s.Copies)
			}
			if s.Origin != "" {
				l += ", from " + s.Origin
			}
			lines = append(lines, l+": "+strings.Join(s.Lines, "; "))
		}
		listLines(&b, lines, "the rest are on Crafting's Spoils tab")
	}

	fmt.Fprintf(&b, "\n### Projects in progress (%d)\n\n", len(c.Projects))
	if len(c.Projects) == 0 {
		b.WriteString("No hero is making anything.\n")
	}
	lines = nil
	for _, p := range c.Projects {
		l := fmt.Sprintf("%s: %s, %d of %d points", p.Hero, p.Name, p.Points, p.Goal)
		if p.Rolls > 0 {
			l += ", " + plural(p.Rolls, "roll", "rolls") + " ready"
		}
		if p.Waits != "" {
			l += " (" + p.Waits + ")"
		}
		lines = append(lines, l)
	}
	listLines(&b, lines, "the rest are on Crafting's Projects tab")

	fmt.Fprintf(&b, "\n### Benches (%d)\n\n", len(c.Benches))
	if len(c.Benches) == 0 {
		b.WriteString("No benches yet.\n")
	}
	lines = nil
	for _, be := range c.Benches {
		lines = append(lines, be.Name+" ("+be.Kind+")")
	}
	listLines(&b, lines, "the rest are on Crafting's Benches tab")

	b.WriteString("\nAsk again with `part: recipes`, `part: materials`, `part: drops` or `part: looks` (and `name` for one) for blocks you can send back changed. " +
		"The tray, projects, benches and each recipe's \"When… then…\" rules are changed in Crafting, not by blocks.\n")
	ans := LookupAnswer{Chip: "Crafting · " + plural(len(c.Recipes), "recipe", "recipes"), Text: b.String()}
	return ans, nil
}

// materialLine says what a material is, where it comes from and what uses
// it: "Troll tooth: Bone, Remnant part; dropped by Troll; used by Tonic".
func materialLine(c *Crafting, it CraftItem) string {
	l := it.Name + ": " + c.tagLabels(it.Tags)
	if from := c.droppedBy(it.ID); len(from) > 0 {
		l += "; dropped by " + joinAnd(from)
	}
	if used := c.usedBy(it.ID, it.Tags); len(used) > 0 {
		l += "; used by " + joinAnd(used)
	}
	return l
}

func craftRecipes(c *Crafting, name string) (LookupAnswer, error) {
	var b strings.Builder
	b.WriteString("## Recipes\n\nSend a block back changed (`action: update`) to change a recipe; a key you leave out keeps what is there.\n")
	shown := 0
	for _, rec := range c.Recipes {
		if name != "" && !sameName(rec.Name, name) {
			continue
		}
		shown++
		b.WriteString("\n")
		if notes := recipeNotes(c, rec); len(notes) > 0 {
			b.WriteString(rec.Name + ": " + strings.Join(notes, "; ") + ".\n\n")
		}
		b.WriteString("```\n" + recipeBlock(c, rec) + "```\n")
	}
	if name != "" && shown == 0 {
		return LookupAnswer{Chip: "Recipes"}, badRequestf("no recipe called %s", quote(name))
	}
	if shown == 0 {
		b.WriteString("\nNo recipes yet.\n")
	}
	return LookupAnswer{Chip: "Recipes · " + plural(shown, "recipe", "recipes"), Text: b.String()}, nil
}

func craftMaterials(c *Crafting, name string) (LookupAnswer, error) {
	var b strings.Builder
	b.WriteString("## Materials\n\nEach Armory item that is a material, where it comes from and what uses it, then its block.\n\n")
	var blocks []string
	for _, it := range c.Items {
		if len(it.Tags) == 0 || (name != "" && !sameName(it.Name, name)) {
			continue
		}
		b.WriteString("- " + materialLine(c, it) + "\n")
		blocks = append(blocks, materialBlock(c, it))
	}
	if len(blocks) == 0 {
		if name != "" {
			return LookupAnswer{Chip: "Materials"}, badRequestf("%s is not a material, or no Armory item has that name", quote(name))
		}
		b.WriteString("No Armory item is a material yet.\n")
	} else {
		b.WriteString("\n```\n" + strings.Join(blocks, "") + "```\n")
	}
	if c.ItemsCut {
		b.WriteString("\n(The campaign has more items than were read.)\n")
	}
	return LookupAnswer{Chip: "Materials · " + plural(len(blocks), "item", "items"), Text: b.String()}, nil
}

func craftDrops(c *Crafting, name string) (LookupAnswer, error) {
	if c.DropsOff {
		return LookupAnswer{Chip: "Drop lists"}, badRequestf("drop lists can't be read on this server right now")
	}
	var b strings.Builder
	b.WriteString("## Drop lists\n\nWhat each page leaves in the spoils tray when it is defeated or searched.\n\n")
	var blocks []string
	for _, d := range c.Drops {
		if name != "" && !sameName(d.Page, name) {
			continue
		}
		blocks = append(blocks, dropsBlock(c, d))
	}
	if len(blocks) == 0 {
		if name != "" {
			return LookupAnswer{Chip: "Drop lists"}, badRequestf("%s has no drop list", quote(name))
		}
		b.WriteString("No page has a drop list yet.\n")
	} else {
		b.WriteString("```\n" + strings.Join(blocks, "") + "```\n")
	}
	return LookupAnswer{Chip: "Drop lists · " + plural(len(blocks), "page", "pages"), Text: b.String()}, nil
}

// craftBook is the recipe book as a player reads it, for a viewer who is no
// Director.
func (run *lookupRun) craftBook(c *Crafting) (LookupAnswer, error) {
	var b strings.Builder
	b.WriteString("## Crafting\n\nThis connection sees what players see: the recipe book as players read it. Drafts, the full recipes, materials, drop lists, the spoils tray and projects are the Director's.\n\n")
	craftRules(&b, c, false)
	fmt.Fprintf(&b, "\n### The recipe book (%d)\n\n", len(c.Book))
	if len(c.Book) == 0 {
		b.WriteString("No recipe is in the book for everyone yet.\n")
	}
	var lines []string
	for _, p := range c.Book {
		l := p.Name
		if p.Kind != "" {
			l += " (" + p.Kind + ")"
		}
		if p.Makes != "" {
			l += fmt.Sprintf(": makes %d × %s", p.MakesQty, p.Makes)
		}
		if len(p.Needs) > 0 {
			l += ". Needs " + joinAnd(p.Needs)
		}
		l += fmt.Sprintf(". %s %d; roll %s", c.Words.Goal, p.Goal, p.Roll)
		if p.Cost != "" {
			l += "; costs " + p.Cost
		}
		if p.Prerequisite != "" {
			l += ". " + c.Words.Prerequisite + ": " + p.Prerequisite
		}
		if p.Source != "" {
			l += ". " + c.Words.Source + ": " + p.Source
		}
		if len(p.Lines) > 0 {
			l += ". " + strings.Join(p.Lines, " ")
		}
		lines = append(lines, l)
	}
	listLines(&b, lines, "the rest are in the recipe book")
	return LookupAnswer{Chip: "Crafting · " + plural(len(c.Book), "recipe", "recipes"), Text: b.String()}, nil
}

// craftLooks lists every item look the Actor can see, as item-look blocks.
func (run *lookupRun) craftLooks() (LookupAnswer, error) {
	ans := LookupAnswer{Chip: "Item looks"}
	if run.l.Looks == nil {
		ans.Text = "## Item looks\n\nItem looks can't be looked up right now.\n"
		return ans, nil
	}
	list, more, err := run.l.Looks.List(run.ctx, run.campaignID, run.a)
	if err != nil {
		return ans, err
	}
	var b strings.Builder
	b.WriteString("## Item looks\n\nHow each item shows its magic when it is used. The campaign's owner can send a block back, changed, to update it (the item-look block format explains the keys).\n\n")
	if len(list) == 0 {
		b.WriteString("No page has a look yet.\n")
	} else {
		shown := list
		if len(shown) > maxLooksShown {
			shown = shown[:maxLooksShown]
		}
		b.WriteString("```\n")
		for _, l := range shown {
			b.WriteString(itemLookBlock(l))
		}
		b.WriteString("```\n")
		if n := len(list) - len(shown); n > 0 {
			fmt.Fprintf(&b, "\n(%d more not shown; `read_page` shows one page's look.)\n", n)
		}
	}
	if more {
		b.WriteString("\n(The campaign has more looks than were read.)\n")
	}
	ans.Chip = "Item looks · " + plural(len(list), "look", "looks")
	ans.Text = b.String()
	return ans, nil
}
