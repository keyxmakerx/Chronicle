package crafting

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// How a recipe reads in a player's recipe book: plain sentences from the
// recipe's parts, in the words the editor's "What players see" preview uses
// (crafting.js mirrors these), so the book and the preview never disagree.

var numberWords = []string{"no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten"}

func numberWord(n int) string {
	if n >= 0 && n < len(numberWords) {
		return numberWords[n]
	}
	return strconv.Itoa(n)
}

func tagLabel(k string) string {
	for _, t := range Tags {
		if t.Key == k {
			return strings.ToLower(t.Label)
		}
	}
	return k
}

func templateName(k string) string {
	for _, t := range Templates {
		if t.Key == k {
			return t.Name
		}
	}
	return "Recipe"
}

// materialName is a material as the book names it: the item's name, "any
// herb" for a tag, or a plain stand-in for an item the viewer cannot see
// (or that was deleted).
func materialName(m Material, names map[string]string) string {
	if m.Tag != "" {
		return "any " + tagLabel(m.Tag)
	}
	if n, ok := names[m.ItemID]; ok {
		return n
	}
	return "an item you haven’t seen"
}

// stepMaterial is how a method line names a step's material.
func stepMaterial(r *Recipe, s Step, names map[string]string) string {
	if s.Material == nil || *s.Material >= len(r.Materials) {
		return "it"
	}
	m := r.Materials[*s.Material]
	if m.Tag != "" {
		return "a " + tagLabel(m.Tag) + " of your choice"
	}
	if n, ok := names[m.ItemID]; ok {
		return "the " + strings.ToLower(n)
	}
	return "the item"
}

// methodLine is one step as a sentence.
func methodLine(r *Recipe, s Step, names map[string]string) string {
	n := stepMaterial(r, s, names)
	switch s.T {
	case "add":
		to := s.To
		if to == "" {
			to = "mortar"
		}
		return "Put " + n + " in the " + to + "."
	case "grind":
		return "Grind it, " + numberWord(s.Turns) + " turns."
	case "tip":
		return "Tip the paste into the flask."
	case "pour":
		return "Pour " + n + " to the line."
	case "simmer":
		if s.Secs >= 10 {
			return "Keep the heat steady for a long while."
		}
		return "Keep the heat steady."
	case "strike":
		if s.N > 1 {
			return "Catch the moment, " + numberWord(s.N) + " times."
		}
		return "Catch the moment."
	}
	return ""
}

// rollWords is the project roll as written: the dice plus the
// characteristic's label.
func rollWords(stat string, rules systems.CraftingRules) string {
	if stat == "" {
		return rules.Dice
	}
	for _, c := range rules.Characteristics {
		if c.Key == stat {
			return rules.Dice + " + " + c.Label
		}
	}
	return rules.Dice + " + " + stat
}

func bookPage(r *Recipe, names map[string]string, rules systems.CraftingRules) BookPage {
	p := BookPage{ID: r.ID, Name: r.Name, Kind: templateName(r.Template), Needs: []BookMaterial{}, Method: []string{}, Lines: []string{},
		MakesQty: r.Result.Qty, Goal: r.Goal, Roll: rollWords(r.Stat, rules), Prerequisite: r.Prerequisite, Source: r.Source}
	if k := benchKind(r.Bench); k != nil {
		p.Bench = k.Name
	}
	for _, m := range r.Materials {
		p.Needs = append(p.Needs, BookMaterial{Qty: m.Qty, Name: materialName(m, names)})
	}
	for _, s := range r.Steps {
		if line := methodLine(r, s, names); line != "" {
			p.Method = append(p.Method, line)
		}
		if k := stepKind(s.T); k != nil && k.Graded {
			p.Graded = true
		}
	}
	if r.Result.ItemID != "" {
		p.Makes = materialName(Material{ItemID: r.Result.ItemID}, names)
	}
	if r.Cost.Amount > 0 {
		c := r.Cost
		p.Cost = &c
	}
	for _, rule := range r.Rules {
		if rule.Draft || rule.Players != "book" {
			continue
		}
		if rule.BookLine != "" {
			p.Lines = append(p.Lines, rule.BookLine)
		} else {
			p.Lines = append(p.Lines, ruleSentence(rule))
		}
	}
	return p
}

// ruleSentence says a rule plainly, for a book rule with no line of its
// own: "When The Lantern is full: the project roll has an edge."
func ruleSentence(r Rule) string {
	var when []string
	for _, c := range r.When {
		if w := condWords(c); w != "" {
			when = append(when, w)
		}
	}
	head := "Always"
	if len(when) > 0 {
		head = "When " + strings.Join(when, " and ")
	}
	s := head + ": " + effectWords(r.Then)
	if r.Chance != "" {
		s += " (" + r.Chance + ")"
	}
	// A spoken line brings its own full stop
	if strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?") {
		return s
	}
	return s + "."
}

func condWords(c Cond) string {
	switch c.Kind {
	case "moon":
		return c.Moon + " is " + c.Phase
	case "event":
		return c.Event + " comes round"
	case "rolled":
		return c.Event + " comes up " + c.Result
	case "weather":
		return "the weather is " + c.Weather
	case "season":
		return "it is " + c.Season
	case "time":
		return "it is " + c.Time
	case "bench":
		return "it is made at " + c.Bench
	case "ingredient":
		return "an ingredient is " + c.Mark
	case "maker":
		return "the maker has " + c.Skill
	}
	return ""
}

func effectWords(e Effect) string {
	switch e.Kind {
	case "only":
		return "it can only be made then"
	case "cannot":
		return "it can’t be made"
	case "edge":
		return "the project roll has an edge"
	case "bane":
		return "the project roll has a bane"
	case "points":
		if e.Amount < 0 {
			return fmt.Sprintf("%d project points", e.Amount)
		}
		return fmt.Sprintf("+%d project points", e.Amount)
	case "extra":
		return "it makes " + numberWord(e.Amount) + " more"
	case "fine":
		return "it comes out fine"
	case "becomes":
		return "it becomes " + e.Item
	case "mark":
		return "it comes out " + e.Mark
	case "quirk":
		return "it may come out with a quirk"
	case "mishap":
		return "something may go wrong"
	case "discover":
		return "you may discover " + e.Recipe
	case "say":
		return e.Text
	}
	return ""
}
