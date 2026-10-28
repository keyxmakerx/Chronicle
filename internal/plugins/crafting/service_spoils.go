package crafting

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/systems"
)

// What a player reads for something they can't see, until it is theirs.
const (
	unseenItem   = "An item you haven’t seen"
	unseenSource = "Something the party found"
)

func (s *service) spoilsReady() error {
	if s.projects.Spoils == nil || s.projects.Armory == nil || s.projects.Pages == nil {
		return apperror.NewNotFound("crafting spoils are not set up on this server")
	}
	return nil
}

// tierLabels names a tiered test's three outcomes from the system's two
// tier tops: "11 or less", "12 to 16", "17 or more".
func tierLabels(tops []int) []string {
	if len(tops) != 2 {
		tops = systems.DefaultCraftingRules().Tiers
	}
	mid := fmt.Sprintf("%d to %d", tops[0]+1, tops[1])
	if tops[0]+1 == tops[1] {
		mid = strconv.Itoa(tops[1])
	}
	return []string{fmt.Sprintf("%d or less", tops[0]), mid, fmt.Sprintf("%d or more", tops[1]+1)}
}

// tierOf is the outcome a total lands in, 0 to 2.
func tierOf(total int, tops []int) int {
	if len(tops) != 2 {
		tops = systems.DefaultCraftingRules().Tiers
	}
	switch {
	case total <= tops[0]:
		return 0
	case total <= tops[1]:
		return 1
	}
	return 2
}

func tiersOf(yields [3]int, labels []string) []Tier {
	out := make([]Tier, 3)
	for i := range out {
		out[i] = Tier{Label: labels[i], Qty: yields[i]}
	}
	return out
}

// ---------- Drop lists ----------

// cleanDrops checks a drop list against the campaign: every line an Armory
// item, a taken line's count, a harvest's characteristic and yields (the
// first tier pays at least one, so even a poor test gets something), and an
// "also drops" page that exists and isn't the page itself.
func (s *service) cleanDrops(ctx context.Context, campaignID, pageID string, in DropInput, rules systems.CraftingRules) (DropList, error) {
	d := DropList{PageID: pageID, Lines: []DropLine{}}
	if len(in.Lines) > maxDropLines {
		return d, errInvalid(fmt.Sprintf("a drop list has at most %d lines", maxDropLines))
	}
	if also := strings.TrimSpace(in.Also); also != "" {
		if also == pageID {
			return d, errInvalid("a page can’t also drop its own list")
		}
		p, err := s.projects.Pages.Page(ctx, campaignID, also)
		if err != nil {
			return d, errInternal(err)
		}
		if p == nil {
			return d, errInvalid("the page it also drops from was not found")
		}
		d.Also = p.ID
	}
	seen := map[string]bool{}
	for i, l := range in.Lines {
		n := i + 1
		id := strings.TrimSpace(l.ItemID)
		if !seen[id] {
			it, err := s.dir.Entity(ctx, campaignID, id)
			if err != nil {
				return d, errInternal(err)
			}
			if id == "" || it == nil || !it.IsItem {
				return d, errInvalid(fmt.Sprintf("line %d: pick an item from the Armory", n))
			}
			seen[id] = true
		}
		out := DropLine{ItemID: id, Mode: l.Mode}
		switch l.Mode {
		case DropTake:
			if l.Qty < 1 || l.Qty > maxQty {
				return d, errInvalid(fmt.Sprintf("line %d: how many is 1 to %d", n, maxQty))
			}
			out.Qty = l.Qty
		case DropHarvest:
			stat, err := cleanStat(l.Stat, rules)
			if err != nil {
				return d, errInvalid(fmt.Sprintf("line %d: %s", n, err.Error()))
			}
			if len(l.Yields) != 3 {
				return d, errInvalid(fmt.Sprintf("line %d: a harvest pays an amount for each of its three tiers", n))
			}
			for j, y := range l.Yields {
				if y < 0 || y > maxQty || (j == 0 && y < 1) {
					return d, errInvalid(fmt.Sprintf("line %d: each tier pays 0 to %d, and the lowest at least 1", n, maxQty))
				}
			}
			out.Stat, out.Yields = stat, []int{l.Yields[0], l.Yields[1], l.Yields[2]}
		default:
			return d, errInvalid(fmt.Sprintf("line %d: say whether it is taken as it is or harvested", n))
		}
		d.Lines = append(d.Lines, out)
	}
	if len(d.Lines) == 0 && d.Also == "" {
		return d, errInvalid("add a line, or a page it also drops from")
	}
	return d, nil
}

// SaveDrops writes a page's drop list.
func (s *service) SaveDrops(ctx context.Context, campaignID, pageID string, v Viewer, in DropInput) (*DropView, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.spoilsReady(); err != nil {
		return nil, err
	}
	page, err := s.projects.Pages.Page(ctx, campaignID, strings.TrimSpace(pageID))
	if err != nil {
		return nil, errInternal(err)
	}
	if page == nil {
		return nil, errNotFound("page not found")
	}
	rules, _ := s.systemRules(ctx, campaignID)
	d, err := s.cleanDrops(ctx, campaignID, page.ID, in, rules)
	if err != nil {
		return nil, err
	}
	ok, err := s.projects.Spoils.SaveDrops(ctx, campaignID, d, v.UserID, in.Version)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflict()
	}
	d.Version = in.Version + 1
	return s.dropView(ctx, campaignID, d, page), nil
}

// DeleteDrops removes a page's drop list. Spoils already in the tray stay.
func (s *service) DeleteDrops(ctx context.Context, campaignID, pageID string, v Viewer) error {
	if err := directorOnly(v); err != nil {
		return err
	}
	if err := s.spoilsReady(); err != nil {
		return err
	}
	if err := s.projects.Spoils.DeleteDrops(ctx, campaignID, strings.TrimSpace(pageID)); err != nil {
		return errInternal(err)
	}
	return nil
}

func (s *service) dropView(ctx context.Context, campaignID string, d DropList, page *PageInfo) *DropView {
	out := &DropView{PageID: d.PageID, Page: page.Name, Kind: page.Kind, Also: d.Also, Lines: d.Lines, Version: d.Version}
	if d.Also != "" {
		if p, err := s.projects.Pages.Page(ctx, campaignID, d.Also); err == nil && p != nil {
			out.AlsoName = p.Name
		}
	}
	return out
}

// ---------- The Director's Spoils tab ----------

// Spoils is the Director's tab: the tray, what was taken lately, the drop
// lists, and the items, heroes and characteristics the tab picks from.
func (s *service) Spoils(ctx context.Context, campaignID string, v Viewer) (*SpoilsPage, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.spoilsReady(); err != nil {
		return nil, err
	}
	rules, _ := s.systemRules(ctx, campaignID)
	out := &SpoilsPage{Taken: []TakenView{}, Drops: []DropView{}, Stats: rules.Characteristics, Tiers: tierLabels(rules.Tiers), Dice: rules.Dice}
	if out.Stats == nil {
		out.Stats = []systems.CraftingStat{}
	}
	cat, err := s.Catalog(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	out.Items, out.Heroes, out.Truncated = cat.Items, cat.Characters, cat.Truncated
	names := make(map[string]string, len(cat.Items))
	tags := make(map[string][]string, len(cat.Items))
	for _, it := range cat.Items {
		names[it.ID], tags[it.ID] = it.Name, it.Tags
	}
	if out.Tray, err = s.trayViews(ctx, campaignID, v, names, tags, rules); err != nil {
		return nil, err
	}
	taken, err := s.projects.Spoils.ListTaken(ctx, campaignID, recentTaken)
	if err != nil {
		return nil, errInternal(err)
	}
	heroes := map[string]string{}
	for _, h := range cat.Characters {
		heroes[h.ID] = h.Name
	}
	for _, l := range taken {
		hero, ok := heroes[l.CharacterID]
		if !ok {
			hero = "A deleted character"
		}
		item := names[l.ItemID]
		if item == "" {
			item = l.ItemName
		}
		text := fmt.Sprintf("%s took %d × %s", hero, l.TakenQty, item)
		if l.Mode == DropHarvest && l.Rolled != nil {
			text = fmt.Sprintf("%s harvested %s (rolled %d): %d × %s", hero, item, *l.Rolled, l.TakenQty, item)
		}
		tv := TakenView{Text: text}
		if l.TakenAt != nil {
			tv.At = *l.TakenAt
		}
		out.Taken = append(out.Taken, tv)
	}
	drops, err := s.projects.Spoils.ListDrops(ctx, campaignID, maxDropLists)
	if err != nil {
		return nil, errInternal(err)
	}
	for _, d := range drops {
		page, err := s.projects.Pages.Page(ctx, campaignID, d.PageID)
		if err != nil {
			return nil, errInternal(err)
		}
		if page == nil {
			continue
		}
		out.Drops = append(out.Drops, *s.dropView(ctx, campaignID, d, page))
	}
	sort.SliceStable(out.Drops, func(i, j int) bool { return strings.ToLower(out.Drops[i].Page) < strings.ToLower(out.Drops[j].Page) })
	return out, nil
}

// trayViews builds the tray as the viewer may see it. names and tags are
// the items the viewer can see; any other item reads as unseen, and a
// player who can't see where a spoil dropped reads "Something the party
// found".
func (s *service) trayViews(ctx context.Context, campaignID string, v Viewer, names map[string]string, tags map[string][]string, rules systems.CraftingRules) ([]SpoilView, error) {
	tray, err := s.projects.Spoils.ListTray(ctx, campaignID)
	if err != nil {
		return nil, errInternal(err)
	}
	labels := tierLabels(rules.Tiers)
	seen := map[string]bool{}
	sees := func(pageID string) (bool, error) {
		if v.Director {
			return true, nil
		}
		if pageID == "" {
			return false, nil
		}
		if ok, done := seen[pageID]; done {
			return ok, nil
		}
		ok, err := s.projects.Pages.CanSee(ctx, campaignID, pageID, v.VisibilityRole, v.UserID)
		if err != nil {
			return false, errInternal(err)
		}
		seen[pageID] = ok
		return ok, nil
	}
	out := make([]SpoilView, 0, len(tray))
	for _, sp := range tray {
		sv := SpoilView{ID: sp.ID, Source: sp.PageName, Copy: sp.Copy, Copies: sp.Copies, Origin: sp.Origin, At: sp.CreatedAt, Lines: []SpoilLineView{}}
		ok, err := sees(sp.PageID)
		if err != nil {
			return nil, err
		}
		if !ok {
			// The copy count would tell a player how many of a hidden
			// thing there were, so it goes with the name.
			sv.Source, sv.Copy, sv.Copies, sv.Origin = unseenSource, 1, 1, ""
		}
		for _, l := range sp.Lines {
			lv := SpoilLineView{ID: l.ID, Mode: l.Mode, Tags: []string{}}
			if n, ok := names[l.ItemID]; ok {
				lv.ItemID, lv.Name, lv.Tags = l.ItemID, n, knownTags(tags[l.ItemID])
			} else if v.Director {
				lv.ItemID, lv.Name = l.ItemID, l.ItemName
			} else {
				lv.Name, lv.Unseen = unseenItem, true
			}
			if l.Mode == DropHarvest {
				lv.Roll, lv.Tiers = rollWords(l.Stat, rules), tiersOf(l.Yields, labels)
			} else {
				lv.Qty = l.Qty
			}
			sv.Lines = append(sv.Lines, lv)
		}
		out = append(out, sv)
	}
	return out, nil
}

// ---------- Filling the tray ----------

// dropsFor resolves what each page drops: its "also" page's lines first,
// then its own, one step deep. Pages without a list are left out.
func (s *service) dropsFor(ctx context.Context, campaignID string, pageIDs []string) (map[string][]DropLine, error) {
	own, err := s.projects.Spoils.DropsFor(ctx, campaignID, pageIDs)
	if err != nil {
		return nil, errInternal(err)
	}
	var more []string
	for _, d := range own {
		if d.Also != "" {
			if _, ok := own[d.Also]; !ok {
				more = append(more, d.Also)
			}
		}
	}
	also := own
	if len(more) > 0 {
		extra, err := s.projects.Spoils.DropsFor(ctx, campaignID, more)
		if err != nil {
			return nil, errInternal(err)
		}
		also = make(map[string]DropList, len(own)+len(extra))
		for k, d := range own {
			also[k] = d
		}
		for k, d := range extra {
			also[k] = d
		}
	}
	out := make(map[string][]DropLine, len(own))
	for id, d := range own {
		var lines []DropLine
		if d.Also != "" {
			lines = append(lines, also[d.Also].Lines...)
		}
		out[id] = append(lines, d.Lines...)
	}
	return out, nil
}

// spoilLines copies drop lines into a spoil's lines, leaving out items
// that were deleted since the list was written.
func (s *service) spoilLines(ctx context.Context, campaignID string, lines []DropLine, items map[string]*EntityInfo) ([]SpoilLine, error) {
	out := []SpoilLine{}
	for _, l := range lines {
		it, ok := items[l.ItemID]
		if !ok {
			var err error
			if it, err = s.dir.Entity(ctx, campaignID, l.ItemID); err != nil {
				return nil, errInternal(err)
			}
			items[l.ItemID] = it
		}
		if it == nil || !it.IsItem {
			continue
		}
		sl := SpoilLine{ItemID: it.ID, ItemName: it.Name, Mode: l.Mode, Qty: l.Qty, Stat: l.Stat}
		if l.Mode == DropHarvest && len(l.Yields) == 3 {
			sl.Qty = 0
			sl.Yields = [3]int{l.Yields[0], l.Yields[1], l.Yields[2]}
		}
		out = append(out, sl)
	}
	return out, nil
}

func dropSummary(lines []SpoilLine) string {
	parts := make([]string, 0, len(lines))
	for _, l := range lines {
		if l.Mode == DropHarvest {
			parts = append(parts, l.ItemName+" to harvest")
		} else {
			parts = append(parts, fmt.Sprintf("%d × %s", l.Qty, l.ItemName))
		}
	}
	return strings.Join(parts, ", ")
}

// room is how many more lines the tray takes before it is full.
func (s *service) room(ctx context.Context, campaignID string) (int, error) {
	n, err := s.projects.Spoils.CountWaiting(ctx, campaignID)
	if err != nil {
		return 0, errInternal(err)
	}
	return maxTrayLines - n, nil
}

var trayFull = fmt.Sprintf("The tray is full (%d things waiting), so nothing more dropped. Clear some first.", maxTrayLines)

// FightEnded puts in the tray what every creature down at the end of a
// fight drops, a spoil per copy, and says for each creature row what it
// left. A fight with no drop lists at all says nothing.
func (s *service) FightEnded(ctx context.Context, campaignID string, v Viewer, f Fight) (*FightSpoils, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	out := &FightSpoils{Lines: []string{}}
	if s.spoilsReady() != nil {
		return out, nil
	}
	var ids []string
	for _, c := range f.Creatures {
		if c.PageID != "" {
			ids = append(ids, c.PageID)
		}
	}
	drops, err := s.dropsFor(ctx, campaignID, ids)
	if err != nil {
		return nil, err
	}
	if len(drops) == 0 {
		return out, nil
	}
	room, err := s.room(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	items := map[string]*EntityInfo{}
	now := s.now()
	var spoils []Spoil
	full := false
	for _, c := range f.Creatures {
		lines, has := drops[c.PageID]
		switch {
		case c.PageID == "":
			out.Lines = append(out.Lines, c.Name+" has no page, so it left nothing.")
			continue
		case !has:
			out.Lines = append(out.Lines, c.Name+" has no drop list, so it left nothing.")
			continue
		case c.Down < 1:
			out.Lines = append(out.Lines, c.Name+" wasn’t down, so it left nothing.")
			continue
		}
		copied, err := s.spoilLines(ctx, campaignID, lines, items)
		if err != nil {
			return nil, err
		}
		if len(copied) == 0 {
			out.Lines = append(out.Lines, c.Name+"’s drop list has no items left, so it left nothing.")
			continue
		}
		n := min(c.Down, maxSpoilCopies)
		if room < n*len(copied) {
			full = true
			n = room / len(copied)
		}
		// The tray names the page, not the fight row, whose name the
		// Director may have written only for themselves.
		name := c.Name
		if p, err := s.projects.Pages.Page(ctx, campaignID, c.PageID); err != nil {
			return nil, errInternal(err)
		} else if p != nil {
			name = p.Name
		}
		for i := 0; i < n; i++ {
			spoils = append(spoils, Spoil{ID: s.newID(), PageID: c.PageID, PageName: name, Copy: i + 1, Copies: n,
				Origin: f.Name, CreatedBy: v.UserID, CreatedAt: now, Lines: copied})
		}
		room -= n * len(copied)
		if n == 0 {
			continue
		}
		who := c.Name
		if n > 1 {
			who = fmt.Sprintf("%s (%d down)", c.Name, n)
		}
		out.Lines = append(out.Lines, fmt.Sprintf("%s left spoils in the tray: %s.", who, dropSummary(copied)))
	}
	if full {
		out.Lines = append(out.Lines, trayFull)
	}
	if len(spoils) > 0 {
		if err := s.projects.Spoils.AddSpoils(ctx, campaignID, spoils); err != nil {
			return nil, errInternal(err)
		}
	}
	out.Spoils = len(spoils)
	return out, nil
}

// DropNotes says, for each page asked about, whether it has a drop list,
// for the Fight tab. A campaign with no drop lists on any of them gets no
// notes, so a table that doesn't use spoils sees nothing new.
func (s *service) DropNotes(ctx context.Context, campaignID string, v Viewer, pageIDs []string) (map[string]string, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if s.spoilsReady() != nil || len(pageIDs) == 0 {
		return nil, nil
	}
	drops, err := s.projects.Spoils.DropsFor(ctx, campaignID, pageIDs)
	if err != nil {
		return nil, errInternal(err)
	}
	if len(drops) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pageIDs))
	for _, id := range pageIDs {
		if _, ok := drops[id]; ok {
			out[id] = "Has a drop list"
		} else {
			out[id] = "No drop list"
		}
	}
	return out, nil
}

// PutSpoils puts a page's drop list in the tray by hand, for a place the
// party searched or a fight run elsewhere.
func (s *service) PutSpoils(ctx context.Context, campaignID string, v Viewer, in PutInput) (*PutResult, error) {
	if err := directorOnly(v); err != nil {
		return nil, err
	}
	if err := s.spoilsReady(); err != nil {
		return nil, err
	}
	if in.Copies < 1 || in.Copies > maxSpoilCopies {
		return nil, errInvalid(fmt.Sprintf("how many is 1 to %d", maxSpoilCopies))
	}
	page, err := s.projects.Pages.Page(ctx, campaignID, strings.TrimSpace(in.PageID))
	if err != nil {
		return nil, errInternal(err)
	}
	if page == nil {
		return nil, errNotFound("page not found")
	}
	drops, err := s.dropsFor(ctx, campaignID, []string{page.ID})
	if err != nil {
		return nil, err
	}
	lines, ok := drops[page.ID]
	if !ok {
		return nil, errInvalid(page.Name + " has no drop list yet")
	}
	copied, err := s.spoilLines(ctx, campaignID, lines, map[string]*EntityInfo{})
	if err != nil {
		return nil, err
	}
	if len(copied) == 0 {
		return nil, errInvalid(page.Name + "’s drop list has no items left")
	}
	room, err := s.room(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if room < in.Copies*len(copied) {
		return nil, errInvalid(trayFull)
	}
	now := s.now()
	spoils := make([]Spoil, 0, in.Copies)
	for i := 0; i < in.Copies; i++ {
		spoils = append(spoils, Spoil{ID: s.newID(), PageID: page.ID, PageName: page.Name, Copy: i + 1, Copies: in.Copies,
			Origin: strings.TrimSpace(in.Origin), CreatedBy: v.UserID, CreatedAt: now, Lines: copied})
	}
	if err := s.projects.Spoils.AddSpoils(ctx, campaignID, spoils); err != nil {
		return nil, errInternal(err)
	}
	return &PutResult{Spoils: len(spoils)}, nil
}

// ---------- Clearing ----------

// ClearTray clears every waiting line.
func (s *service) ClearTray(ctx context.Context, campaignID string, v Viewer) error {
	return s.clearing(v, func() (int, error) { return s.projects.Spoils.ClearTray(ctx, campaignID) }, false)
}

// ClearSpoil clears what is left of one spoil.
func (s *service) ClearSpoil(ctx context.Context, campaignID, id string, v Viewer) error {
	return s.clearing(v, func() (int, error) { return s.projects.Spoils.ClearSpoil(ctx, campaignID, strings.TrimSpace(id)) }, true)
}

// ClearSpoilLine clears one waiting line.
func (s *service) ClearSpoilLine(ctx context.Context, campaignID string, id int64, v Viewer) error {
	return s.clearing(v, func() (int, error) { return s.projects.Spoils.ClearLine(ctx, campaignID, id) }, true)
}

// clearing runs a clear for a Director; one that should have found a
// waiting line and didn't says it is gone.
func (s *service) clearing(v Viewer, clear func() (int, error), one bool) error {
	if err := directorOnly(v); err != nil {
		return err
	}
	if err := s.spoilsReady(); err != nil {
		return err
	}
	n, err := clear()
	if err != nil {
		return errInternal(err)
	}
	if one && n == 0 {
		return errNotFound("those spoils are gone; someone took them or they were cleared")
	}
	return nil
}

// ---------- Taking ----------

// TakeSpoil takes a waiting line for a hero: a player for a hero they
// claimed, a Director for anyone. A harvest line rolls the test on the
// server and pays the tier it lands in. The line is claimed before the
// item moves, so two takers can't both have it, and goes back in the tray
// if the item can't reach the pack.
func (s *service) TakeSpoil(ctx context.Context, campaignID string, lineID int64, v Viewer, in TakeInput) (*TakeResult, error) {
	if err := s.spoilsReady(); err != nil {
		return nil, err
	}
	e, err := s.hero(ctx, campaignID, in.CharacterID, v)
	if err != nil {
		return nil, err
	}
	l, ok, err := s.projects.Spoils.GetLine(ctx, campaignID, lineID)
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok || l.Status == SpoilCleared {
		return nil, errNotFound("those spoils are gone")
	}
	if l.Status != SpoilWaiting {
		return nil, errConflictMsg("someone took that first")
	}
	item, err := s.dir.Entity(ctx, campaignID, l.ItemID)
	if err != nil {
		return nil, errInternal(err)
	}
	if item == nil || !item.IsItem {
		return nil, errNotFound("those spoils are gone")
	}
	res := &TakeResult{Hero: e.Name, Notes: []string{}}
	var rolled *int
	harvested := l.Mode == DropHarvest
	if harvested {
		rules, _ := s.systemRules(ctx, campaignID)
		s.harvest(ctx, campaignID, e, l, rules, res)
		total := res.Total
		rolled = &total
	} else {
		res.Qty = l.Qty
	}
	ok, err = s.projects.Spoils.ClaimLine(ctx, campaignID, l.ID, e.ID, v.UserID, res.Qty, rolled, s.now())
	if err != nil {
		return nil, errInternal(err)
	}
	if !ok {
		return nil, errConflictMsg("someone took that first")
	}
	res.Got = fmt.Sprintf("%d × %s", res.Qty, item.Name)
	if res.Qty < 1 {
		res.Got = "nothing"
		res.Notes = append(res.Notes, "That tier pays nothing for this one.")
		return res, nil
	}
	if err := s.projects.Armory.GiveSpoil(ctx, campaignID, v, e.ID, item.ID, res.Qty, harvested); err != nil {
		if rerr := s.projects.Spoils.ReleaseLine(ctx, campaignID, l.ID); rerr != nil {
			slog.Error("crafting: A SPOIL WAS CLAIMED BUT NEITHER GIVEN NOR PUT BACK",
				slog.Int64("line_id", l.ID), slog.String("character_id", e.ID), slog.Any("error", rerr))
		}
		return nil, err
	}
	return res, nil
}

// harvest rolls a harvest's test: the system's dice and the line's
// characteristic from the hero's sheet, paying the tier the total lands in.
func (s *service) harvest(ctx context.Context, campaignID string, e *EntityInfo, l SpoilLine, rules systems.CraftingRules, res *TakeResult) {
	count, size, ok := systems.ParseDice(rules.Dice)
	if !ok {
		count, size = 2, 10
	}
	sum := 0
	for i := 0; i < count; i++ {
		d := s.roll(size)
		res.Dice = append(res.Dice, d)
		sum += d
	}
	res.Sides = size
	res.Terms = append(res.Terms, Term{Text: fmt.Sprintf("%dd%d", count, size), N: sum})
	if l.Stat != "" {
		label := statLabel(l.Stat, rules)
		if n := s.sheetStat(ctx, campaignID, e.ID, l.Stat); n != nil {
			res.Terms = append(res.Terms, Term{Text: label + ", from " + e.Name + "’s sheet", N: *n})
		} else {
			res.Notes = append(res.Notes, label+" isn’t a number on "+e.Name+"’s sheet, so it added nothing.")
		}
	}
	for _, t := range res.Terms {
		res.Total += t.N
	}
	res.Tier = tierOf(res.Total, rules.Tiers)
	res.Tiers = tiersOf(l.Yields, tierLabels(rules.Tiers))
	res.Qty = l.Yields[res.Tier]
}
