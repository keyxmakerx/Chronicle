package crafting

import (
	"context"
	"log/slog"
	"math"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// What a project roll does beyond adding up points: the item effects of a
// recipe's rules (fine, a mark, a quirk, made as something else), mishaps
// mid-brew, discoveries, and the recipe's "If it goes wrong" and "Extras"
// settings that the roll can carry out (botch costs, breakthroughs,
// mastery, substitutes, and what the made item remembers).

// skills reads the maker's skills from their sheet, lowercased; nil when
// the sheet lists none or can't be read, so a rule about a skill stays
// unknown rather than off.
func (s *service) skills(ctx context.Context, campaignID, characterID string) map[string]bool {
	if s.projects.Sheets == nil {
		return nil
	}
	list, ok, err := s.projects.Sheets.Skills(ctx, campaignID, characterID)
	if err != nil {
		slog.Warn("crafting: cannot read a sheet's skills", slog.String("character_id", characterID), slog.Any("error", err))
		return nil
	}
	if !ok {
		return nil
	}
	out := make(map[string]bool, len(list))
	for _, k := range list {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			out[k] = true
		}
	}
	return out
}

// mastered says the hero has brewed this recipe cleanly as often as its
// mastery asks.
func (s *service) mastered(ctx context.Context, campaignID string, p Project, x Extras) bool {
	if x.Mastery == nil || p.RecipeID == "" {
		return false
	}
	n, err := s.projects.Store.CleanRolls(ctx, campaignID, p.CharacterID, p.RecipeID)
	if err != nil {
		slog.Warn("crafting: cannot count clean rolls", slog.String("project_id", p.ID), slog.Any("error", err))
		return false
	}
	return n >= x.Mastery.AfterCleanBrews
}

// rollTable rolls one row of the campaign's table by name with the server's
// dice, weighting each row by its chance. ok is false when there is no such
// table, or it has no rows.
func (s *service) rollTable(ctx context.Context, campaignID, name string) (row int, text string, ok bool) {
	if s.projects.Tables == nil || name == "" {
		return 0, "", false
	}
	rows, found, err := s.projects.Tables.Rows(ctx, campaignID, name)
	if err != nil {
		slog.Warn("crafting: cannot read a rolling table", slog.String("table", name), slog.Any("error", err))
		return 0, "", false
	}
	if !found || len(rows) == 0 {
		return 0, "", false
	}
	// Weights are relative and may be fractions; a hundredth is as fine as
	// the Rolling Tables page lets anyone set one.
	units := make([]int, len(rows))
	total := 0
	for i, r := range rows {
		u := int(math.Round(r.Weight * 100))
		if u < 1 {
			u = 1
		}
		units[i] = u
		total += u
	}
	n := s.roll(total)
	for i, u := range units {
		if n <= u {
			return i, rows[i].Text, true
		}
		n -= u
	}
	return len(rows) - 1, rows[len(rows)-1].Text, true
}

// tableNote names a table that a rule or setting asks for but the campaign
// doesn't have, for the Director only.
func tableNote(name string, director bool, res *RollResult) {
	if director {
		res.Notes = append(res.Notes, "There is no Rolling Table called “"+name+"”, so nothing was rolled on it.")
	}
}

// befall carries out what happens to the work on this roll: the item
// effects of rules that fired (kept on the project and shown when it is
// made), a mishap mid-brew, and what botched work costs. It returns the
// item the project becomes, "" for none.
func (s *service) befall(ctx context.Context, campaignID string, next *Project, fx []fired, x Extras, work string, set Settings, res *RollResult, v Viewer) string {
	becomes := ""
	made := func(note string) {
		if len(next.Details.Made) < maxMadeNotes {
			next.Details.Made = append(next.Details.Made, note)
		}
	}
	item := func() *Kept {
		if next.Details.Item == nil {
			next.Details.Item = &Kept{}
		}
		return next.Details.Item
	}
	for _, f := range fx {
		t := f.rule.Then
		switch t.Kind {
		case "fine":
			if res.Finished {
				made("Fine")
				item().Quality = "fine"
			}
		case "mark":
			if res.Finished && t.Mark != "" {
				made("Marked: " + t.Mark)
				if k := item(); len(k.Marks) < maxKeptMarks {
					k.Marks = append(k.Marks, t.Mark)
				}
			}
		case "becomes":
			if !res.Finished || t.Item == "" {
				continue
			}
			if id := s.itemByName(ctx, campaignID, t.Item); id != "" {
				becomes = id
				made("Made as " + t.Item)
			} else if v.Director {
				res.Notes = append(res.Notes, "A rule says it becomes “"+t.Item+"”, but the Armory has no item by that name, so it was made as written.")
			}
		case "quirk":
			if !res.Finished {
				continue
			}
			if _, text, ok := s.rollTable(ctx, campaignID, t.Table); ok {
				made("A quirk: " + text)
				item().Quirk = text
			} else {
				tableNote(t.Table, v.Director, res)
			}
		case "mishap":
			if res.Mishap != nil {
				continue
			}
			if row, text, ok := s.rollTable(ctx, campaignID, t.Table); ok {
				res.Mishap = &Mishap{Table: t.Table, Row: row, Text: text}
			} else {
				tableNote(t.Table, v.Director, res)
			}
		}
	}
	if x.Mishaps != nil && res.Mishap == nil && !res.Spoiled {
		if n := chanceIn(x.Mishaps.Chance); n <= 1 || s.roll(n) == 1 {
			if row, text, ok := s.rollTable(ctx, campaignID, x.Mishaps.Table); ok {
				res.Mishap = &Mishap{Table: x.Mishaps.Table, Row: row, Text: text}
			} else {
				tableNote(x.Mishaps.Table, v.Director, res)
			}
		}
	}
	if work == WorkBotched && set.Bane && !res.Spoiled {
		switch x.Botch.Costs {
		case BotchQuirk:
			if _, text, ok := s.rollTable(ctx, campaignID, x.Botch.QuirkTable); ok {
				made("A quirk: " + text)
				item().Quirk = text
				res.Notes = append(res.Notes, "The botch left a quirk: "+text)
			} else {
				tableNote(x.Botch.QuirkTable, v.Director, res)
			}
		case BotchIngredients:
			// The block says so from Lost, beside the roll.
			res.Lost = !res.Finished
		}
	}
	return becomes
}

// discover teaches the maker every recipe a fired "discovers…" rule names.
// A recipe everyone knows, or one already learned, is nothing new; a name
// the campaign has no recipe for is told to the Director only.
func (s *service) discover(ctx context.Context, campaignID string, e *EntityInfo, fx []fired, v Viewer, res *RollResult) {
	var names []string
	for _, f := range fx {
		if f.rule.Then.Kind == "discover" && f.rule.Then.Recipe != "" {
			names = append(names, f.rule.Then.Recipe)
		}
	}
	if len(names) == 0 {
		return
	}
	all, err := s.repo.ListRecipes(ctx, campaignID)
	if err != nil {
		slog.Error("crafting: cannot read recipes for a discovery", slog.Any("error", err))
		return
	}
	for _, name := range names {
		var found *Recipe
		for i := range all {
			if strings.EqualFold(all[i].Name, name) {
				found = &all[i]
			}
		}
		switch {
		case found == nil:
			if v.Director {
				res.Notes = append(res.Notes, "A rule discovers “"+name+"”, but there is no recipe by that name.")
			}
		case found.Draft:
			if v.Director {
				res.Notes = append(res.Notes, "A rule discovers “"+name+"”, which is still a draft, so nobody learned it.")
			}
		case found.Who != "learned":
			res.Notes = append(res.Notes, e.Name+" found the way to make "+found.Name+", which everyone already knows.")
		default:
			if err := s.repo.Learn(ctx, campaignID, found.ID, e.ID, v.UserID); err != nil {
				slog.Error("crafting: a discovery was not recorded", slog.String("recipe_id", found.ID), slog.Any("error", err))
				continue
			}
			res.Notes = append(res.Notes, e.Name+" discovered how to make "+found.Name+". It’s in their recipe book now.")
		}
	}
}

// itemByName finds an Armory item by its name, as the Director sees the
// Armory, "" when there is none.
func (s *service) itemByName(ctx context.Context, campaignID, name string) string {
	items, _, err := s.dir.ListItems(ctx, campaignID, permissions.RoleOwner, "")
	if err != nil {
		slog.Error("crafting: cannot read the Armory's items", slog.Any("error", err))
		return ""
	}
	for _, it := range items {
		if strings.EqualFold(strings.TrimSpace(it.Name), strings.TrimSpace(name)) {
			return it.ID
		}
	}
	return ""
}

// maxKeptMarks caps the marks one made item carries, as the Armory does.
const maxKeptMarks = 6

// ingredientsOf sums what the remembered copies that went into a project
// carried. A project always gets one, empty when every ingredient was
// plain, so its ingredient rules are known to be off rather than unknown.
func ingredientsOf(used []Kept) *Ingredients {
	out := &Ingredients{}
	seen := map[string]bool{}
	for _, k := range used {
		for _, m := range k.Marks {
			if l := strings.ToLower(m); !seen[l] {
				seen[l] = true
				out.Marks = append(out.Marks, m)
			}
		}
		switch k.Quality {
		case "fine":
			out.Fine += k.Qty
		case "poor":
			out.Poor += k.Qty
		}
	}
	return out
}

// madeKept is what the made item remembers: what the rolls gave it (fine,
// marks, a quirk), the maker's mark, and when it goes off, by the recipe's
// extras and the calendar's today.
func (s *service) madeKept(ctx context.Context, p *Project, e *EntityInfo, x Extras, day *Day) Kept {
	var k Kept
	if p.Details.Item != nil {
		k = *p.Details.Item
	}
	k.Look = s.madeLook(ctx, p)
	if x.MakersMark {
		k.How, k.By = "made", e.Name
		if day != nil {
			k.On = day.Label
		}
	}
	if x.Keeps != nil && x.Keeps.Days > 0 && day != nil && day.After != nil {
		k.OffDay, k.OffOn = day.After(x.Keeps.Days)
		if x.Keeps.ThenBecomes != "" {
			k.Becomes = s.itemByName(ctx, p.CampaignID, x.Keeps.ThenBecomes)
			k.BecomesName = x.Keeps.ThenBecomes
		}
	}
	return k
}

// harvestKept is what harvested parts remember: a top-tier cut gives fine
// parts and the lowest tier poor ones, with who cut them and when. The
// middle tier's parts are plain and join the plain stack.
func (s *service) harvestKept(ctx context.Context, campaignID string, e *EntityInfo, tier int) Kept {
	k := Kept{How: "harvested", By: e.Name}
	switch tier {
	case 2:
		k.Quality = "fine"
	case 0:
		k.Quality = "poor"
	default:
		return Kept{}
	}
	if d := s.today(ctx, campaignID); d != nil {
		k.On = d.Label
	}
	return k
}
