package crafting

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// ---------- Spoils fakes ----------

// fakeSpoils keeps drop lists and the tray in memory, with the same
// guarded claim as SQL.
type fakeSpoils struct {
	drops  map[string]DropList
	spoils []Spoil
	lines  map[int64]*SpoilLine
	order  []int64
	next   int64
}

func newFakeSpoils() *fakeSpoils {
	return &fakeSpoils{drops: map[string]DropList{}, lines: map[int64]*SpoilLine{}}
}

func (f *fakeSpoils) GetDrops(_ context.Context, _, pageID string) (DropList, bool, error) {
	d, ok := f.drops[pageID]
	return d, ok, nil
}
func (f *fakeSpoils) ListDrops(_ context.Context, _ string, _ int) ([]DropList, error) {
	out := []DropList{}
	for _, d := range f.drops {
		out = append(out, d)
	}
	return out, nil
}
func (f *fakeSpoils) DropsFor(_ context.Context, _ string, ids []string) (map[string]DropList, error) {
	out := map[string]DropList{}
	for _, id := range ids {
		if d, ok := f.drops[id]; ok {
			out[id] = d
		}
	}
	return out, nil
}
func (f *fakeSpoils) SaveDrops(_ context.Context, _ string, d DropList, _ string, expected int) (bool, error) {
	old, ok := f.drops[d.PageID]
	if (expected == 0 && ok) || (expected != 0 && (!ok || old.Version != expected)) {
		return false, nil
	}
	d.Version = expected + 1
	f.drops[d.PageID] = d
	return true, nil
}
func (f *fakeSpoils) DeleteDrops(_ context.Context, _, pageID string) error {
	delete(f.drops, pageID)
	return nil
}
func (f *fakeSpoils) AddSpoils(_ context.Context, campaignID string, spoils []Spoil) error {
	for _, s := range spoils {
		s.CampaignID = campaignID
		for _, l := range s.Lines {
			f.next++
			l.ID, l.SpoilID, l.Status = f.next, s.ID, SpoilWaiting
			line := l
			f.lines[l.ID] = &line
			f.order = append(f.order, l.ID)
		}
		s.Lines = nil
		f.spoils = append(f.spoils, s)
	}
	return nil
}
func (f *fakeSpoils) CountWaiting(context.Context, string) (int, error) {
	n := 0
	for _, l := range f.lines {
		if l.Status == SpoilWaiting {
			n++
		}
	}
	return n, nil
}
func (f *fakeSpoils) ListTray(context.Context, string) ([]Spoil, error) {
	out := []Spoil{}
	for _, s := range f.spoils {
		for _, id := range f.order {
			if l := f.lines[id]; l.SpoilID == s.ID && l.Status == SpoilWaiting {
				s.Lines = append(s.Lines, *l)
			}
		}
		if len(s.Lines) > 0 {
			out = append(out, s)
		}
	}
	return out, nil
}
func (f *fakeSpoils) GetLine(_ context.Context, _ string, id int64) (SpoilLine, bool, error) {
	l, ok := f.lines[id]
	if !ok {
		return SpoilLine{}, false, nil
	}
	return *l, true, nil
}
func (f *fakeSpoils) ClaimLine(_ context.Context, _ string, id int64, characterID, by string, qty int, rolled *int, at time.Time) (bool, error) {
	l, ok := f.lines[id]
	if !ok || l.Status != SpoilWaiting {
		return false, nil
	}
	l.Status, l.CharacterID, l.TakenBy, l.TakenQty, l.Rolled, l.TakenAt = SpoilTaken, characterID, by, qty, rolled, &at
	return true, nil
}
func (f *fakeSpoils) ReleaseLine(_ context.Context, _ string, id int64) error {
	if l, ok := f.lines[id]; ok && l.Status == SpoilTaken {
		l.Status, l.CharacterID, l.TakenQty, l.Rolled, l.TakenAt = SpoilWaiting, "", 0, nil, nil
	}
	return nil
}
func (f *fakeSpoils) clear(match func(*SpoilLine) bool) int {
	n := 0
	for _, l := range f.lines {
		if l.Status == SpoilWaiting && match(l) {
			l.Status = SpoilCleared
			n++
		}
	}
	return n
}
func (f *fakeSpoils) ClearLine(_ context.Context, _ string, id int64) (int, error) {
	return f.clear(func(l *SpoilLine) bool { return l.ID == id }), nil
}
func (f *fakeSpoils) ClearSpoil(_ context.Context, _, spoilID string) (int, error) {
	return f.clear(func(l *SpoilLine) bool { return l.SpoilID == spoilID }), nil
}
func (f *fakeSpoils) ClearTray(context.Context, string) (int, error) {
	return f.clear(func(*SpoilLine) bool { return true }), nil
}
func (f *fakeSpoils) ListTaken(context.Context, string, int) ([]SpoilLine, error) {
	out := []SpoilLine{}
	for i := len(f.order) - 1; i >= 0; i-- {
		if l := f.lines[f.order[i]]; l.Status == SpoilTaken {
			out = append(out, *l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TakenAt.After(*out[j].TakenAt) })
	return out, nil
}

// fakePages is the campaign's pages for drop lists. Players can't see the
// shrine.
type fakePages struct{ hidden map[string]bool }

var spoilPages = map[string]PageInfo{
	"troll":   {ID: "troll", Name: "Troll", Kind: "Creature"},
	"remnant": {ID: "remnant", Name: "Troll Remnant", Kind: "Creature"},
	"goblin":  {ID: "goblin", Name: "Goblin sneak", Kind: "Creature"},
	"shrine":  {ID: "shrine", Name: "The Weeping Shrine", Kind: "Place"},
}

func (p *fakePages) Page(_ context.Context, _, id string) (*PageInfo, error) {
	if pi, ok := spoilPages[id]; ok {
		return &pi, nil
	}
	return nil, nil
}
func (p *fakePages) CanSee(_ context.Context, _, id string, role int, _ string) (bool, error) {
	_, ok := spoilPages[id]
	return ok && (role >= 3 || !p.hidden[id]), nil
}

type spoilFx struct {
	*projectFx
	sp    *fakeSpoils
	pages *fakePages
}

// newSpoilFx is the projects campaign with spoils: a troll drops two
// Bloodmoss and Spring water to harvest with Reason (1, 2, 3); its Remnant
// also drops what the troll drops, plus the secret item; the shrine drops
// Spring water.
func newSpoilFx() *spoilFx {
	fx := &spoilFx{projectFx: newProjectFx(), sp: newFakeSpoils(), pages: &fakePages{hidden: map[string]bool{"shrine": true}}}
	fx.s.projects.Spoils, fx.s.projects.Pages = fx.sp, fx.pages
	fx.sp.drops["troll"] = DropList{PageID: "troll", Version: 1, Lines: []DropLine{
		{ItemID: "moss", Mode: DropTake, Qty: 2},
		{ItemID: "water", Mode: DropHarvest, Stat: "reason", Yields: []int{1, 2, 3}}}}
	fx.sp.drops["remnant"] = DropList{PageID: "remnant", Also: "troll", Version: 1, Lines: []DropLine{{ItemID: "secret", Mode: DropTake, Qty: 1}}}
	fx.sp.drops["shrine"] = DropList{PageID: "shrine", Version: 1, Lines: []DropLine{{ItemID: "water", Mode: DropTake, Qty: 1}}}
	return fx
}

// fight ends a fight with two trolls down, a remnant down, a goblin with
// no drop list, a row with no page and a remnant still standing.
func (fx *spoilFx) fight(t *testing.T) *FightSpoils {
	t.Helper()
	got, err := fx.s.FightEnded(context.Background(), "c", director, Fight{Name: "Shrine cellar", Creatures: []FightCreature{
		{PageID: "troll", Name: "Troll", Down: 2, Copies: 2},
		{PageID: "remnant", Name: "Remnant the DM renamed", Down: 1, Copies: 1},
		{PageID: "goblin", Name: "Goblin sneak", Down: 1, Copies: 1},
		{Name: "Cellar rats", Down: 2, Copies: 3},
		{PageID: "remnant", Name: "Second remnant", Down: 0, Copies: 1},
	}})
	if err != nil {
		t.Fatalf("fight ended: %v", err)
	}
	return got
}

// lineFor finds a waiting line by item and spoil source.
func (fx *spoilFx) lineFor(t *testing.T, source, item string) int64 {
	t.Helper()
	for _, s := range fx.sp.spoils {
		if s.PageName != source {
			continue
		}
		for _, id := range fx.sp.order {
			if l := fx.sp.lines[id]; l.SpoilID == s.ID && l.ItemID == item && l.Status == SpoilWaiting {
				return id
			}
		}
	}
	t.Fatalf("no waiting %s from %s", item, source)
	return 0
}

// ---------- Tests ----------

func TestSpoilsAreDirectorsOnly(t *testing.T) {
	fx := newSpoilFx()
	ctx := context.Background()
	calls := map[string]func() error{
		"tab": func() error { _, err := fx.s.Spoils(ctx, "c", player); return err },
		"save drops": func() error {
			_, err := fx.s.SaveDrops(ctx, "c", "goblin", player, DropInput{Also: "troll"})
			return err
		},
		"delete": func() error { return fx.s.DeleteDrops(ctx, "c", "troll", player) },
		"put": func() error {
			_, err := fx.s.PutSpoils(ctx, "c", player, PutInput{PageID: "troll", Copies: 1})
			return err
		},
		"clear tray": func() error { return fx.s.ClearTray(ctx, "c", player) },
		"clear one":  func() error { return fx.s.ClearSpoil(ctx, "c", "x", player) },
		"clear line": func() error { return fx.s.ClearSpoilLine(ctx, "c", 1, player) },
		"fight":      func() error { _, err := fx.s.FightEnded(ctx, "c", player, Fight{}); return err },
		"notes":      func() error { _, err := fx.s.DropNotes(ctx, "c", player, []string{"troll"}); return err },
	}
	for name, call := range calls {
		if c := code(call()); c != 403 {
			t.Errorf("%s as a player: %d", name, c)
		}
	}
	if _, ok := fx.sp.drops["goblin"]; ok || len(fx.sp.spoils) != 0 {
		t.Error("a refused call changed something")
	}
}

func TestSaveDrops(t *testing.T) {
	ctx := context.Background()
	take := func(item string, n int) DropLine { return DropLine{ItemID: item, Mode: DropTake, Qty: n} }
	harvest := func(stat string, y ...int) DropLine {
		return DropLine{ItemID: "water", Mode: DropHarvest, Stat: stat, Yields: y}
	}
	cases := []struct {
		name string
		page string
		in   DropInput
		code int
	}{
		{"a take and a harvest", "goblin", DropInput{Lines: []DropLine{take("moss", 2), harvest("reason", 1, 0, 4)}}, 0},
		{"only what another page drops", "goblin", DropInput{Also: "troll"}, 0},
		{"no such page", "nowhere", DropInput{Also: "troll"}, 404},
		{"nothing at all", "goblin", DropInput{}, 422},
		{"not an item", "goblin", DropInput{Lines: []DropLine{take("mira", 1)}}, 422},
		{"no item", "goblin", DropInput{Lines: []DropLine{take("", 1)}}, 422},
		{"none taken", "goblin", DropInput{Lines: []DropLine{take("moss", 0)}}, 422},
		{"too many taken", "goblin", DropInput{Lines: []DropLine{take("moss", maxQty+1)}}, 422},
		{"no mode", "goblin", DropInput{Lines: []DropLine{{ItemID: "moss", Qty: 1}}}, 422},
		{"two tiers", "goblin", DropInput{Lines: []DropLine{harvest("reason", 1, 2)}}, 422},
		{"a poor test pays nothing", "goblin", DropInput{Lines: []DropLine{harvest("reason", 0, 2, 3)}}, 422},
		{"a tier pays too much", "goblin", DropInput{Lines: []DropLine{harvest("reason", 1, 2, maxQty+1)}}, 422},
		{"a characteristic the system lacks", "goblin", DropInput{Lines: []DropLine{harvest("luck", 1, 2, 3)}}, 422},
		{"also drops its own list", "goblin", DropInput{Also: "goblin"}, 422},
		{"also drops a missing page", "goblin", DropInput{Also: "nowhere"}, 422},
		{"a page that has one, as new", "troll", DropInput{Also: "shrine"}, 409},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := newSpoilFx()
			lines := make([]DropLine, 0, maxDropLines+1)
			lines = append(lines, c.in.Lines...)
			c.in.Lines = lines
			got, err := fx.s.SaveDrops(ctx, "c", c.page, director, c.in)
			if code(err) != c.code {
				t.Fatalf("code %d (%v), want %d", code(err), err, c.code)
			}
			if c.code != 0 {
				if _, ok := fx.sp.drops["goblin"]; ok {
					t.Error("a refused save was kept")
				}
				return
			}
			if got.Version != 1 || got.Page != "Goblin sneak" || got.Kind != "Creature" || got.Also != c.in.Also {
				t.Errorf("saved %+v", got)
			}
			if c.in.Also != "" && got.AlsoName != "Troll" {
				t.Errorf("also named %q", got.AlsoName)
			}
		})
	}

	t.Run("too many lines", func(t *testing.T) {
		fx := newSpoilFx()
		in := DropInput{}
		for i := 0; i <= maxDropLines; i++ {
			in.Lines = append(in.Lines, take("moss", 1))
		}
		if _, err := fx.s.SaveDrops(ctx, "c", "goblin", director, in); code(err) != 422 {
			t.Errorf("code %d", code(err))
		}
	})
	t.Run("an edit needs the version it read", func(t *testing.T) {
		fx := newSpoilFx()
		in := DropInput{Lines: []DropLine{take("moss", 3)}, Version: 1}
		got, err := fx.s.SaveDrops(ctx, "c", "troll", director, in)
		if err != nil || got.Version != 2 || got.Lines[0].Qty != 3 {
			t.Fatalf("edit: %+v, %v", got, err)
		}
		if _, err := fx.s.SaveDrops(ctx, "c", "troll", director, in); code(err) != 409 {
			t.Errorf("stale edit: %v", err)
		}
	})
}

func TestFightEnded_DropsASpoilPerDownCopy(t *testing.T) {
	fx := newSpoilFx()
	got := fx.fight(t)
	want := []string{
		"Troll (2 down) left spoils in the tray: 2 × Bloodmoss, Spring water to harvest.",
		"Remnant the DM renamed left spoils in the tray: 2 × Bloodmoss, Spring water to harvest, 1 × Dragon's tear.",
		"Goblin sneak has no drop list, so it left nothing.",
		"Cellar rats has no page, so it left nothing.",
		"Second remnant wasn’t down, so it left nothing.",
	}
	if !reflect.DeepEqual(got.Lines, want) || got.Spoils != 3 {
		t.Fatalf("report %d spoils:\n%s", got.Spoils, strings.Join(got.Lines, "\n"))
	}
	if len(fx.sp.spoils) != 3 {
		t.Fatalf("spoils %+v", fx.sp.spoils)
	}
	// The trolls are numbered among those that dropped, and the remnant is
	// named as its page is, not as the fight row was.
	for i, w := range []struct {
		name         string
		copy, copies int
	}{{"Troll", 1, 2}, {"Troll", 2, 2}, {"Troll Remnant", 1, 1}} {
		if s := fx.sp.spoils[i]; s.PageName != w.name || s.Copy != w.copy || s.Copies != w.copies || s.Origin != "Shrine cellar" {
			t.Errorf("spoil %d = %+v", i, s)
		}
	}
	// A harvest line is copied with its yields, a take with its count.
	h := fx.sp.lines[fx.lineFor(t, "Troll Remnant", "water")]
	if h.Mode != DropHarvest || h.Stat != "reason" || h.Yields != [3]int{1, 2, 3} || h.ItemName != "Spring water" {
		t.Errorf("harvest line %+v", h)
	}
	// Editing the list later leaves what already dropped alone.
	fx.sp.drops["troll"] = DropList{PageID: "troll", Version: 2, Lines: []DropLine{{ItemID: "potion", Mode: DropTake, Qty: 9}}}
	if l := fx.sp.lines[fx.lineFor(t, "Troll", "moss")]; l.Qty != 2 {
		t.Errorf("line changed with the list: %+v", l)
	}
}

func TestFightEnded_QuietWithoutDropLists(t *testing.T) {
	fx := newSpoilFx()
	got, err := fx.s.FightEnded(context.Background(), "c", director, Fight{Name: "Brawl", Creatures: []FightCreature{
		{PageID: "goblin", Name: "Goblin sneak", Down: 1, Copies: 1}, {Name: "Rats", Down: 1, Copies: 1}}})
	if err != nil || len(got.Lines) != 0 || got.Spoils != 0 {
		t.Errorf("report %+v, %v", got, err)
	}
	// Without spoils set up a fight still ends.
	fx.s.projects.Spoils = nil
	if got, err := fx.s.FightEnded(context.Background(), "c", director, Fight{Creatures: []FightCreature{{PageID: "troll", Down: 1}}}); err != nil || len(got.Lines) != 0 {
		t.Errorf("not set up: %+v, %v", got, err)
	}
}

func TestFightEnded_StopsWhenTheTrayIsFull(t *testing.T) {
	fx := newSpoilFx()
	filler := Spoil{ID: "old", PageName: "Old"}
	for i := 0; i < maxTrayLines-3; i++ {
		filler.Lines = append(filler.Lines, SpoilLine{ItemID: "moss", Mode: DropTake, Qty: 1})
	}
	_ = fx.sp.AddSpoils(context.Background(), "c", []Spoil{filler})
	got, err := fx.s.FightEnded(context.Background(), "c", director, Fight{Creatures: []FightCreature{{PageID: "troll", Name: "Troll", Down: 3, Copies: 3}}})
	if err != nil || got.Spoils != 1 || got.Lines[len(got.Lines)-1] != trayFull {
		t.Fatalf("report %+v, %v", got, err)
	}
	if n, _ := fx.sp.CountWaiting(context.Background(), "c"); n > maxTrayLines {
		t.Errorf("tray holds %d", n)
	}
}

func TestTakeSpoil(t *testing.T) {
	ctx := context.Background()
	fx := newSpoilFx()
	fx.fight(t)
	id := fx.lineFor(t, "Troll", "moss")

	if _, err := fx.s.TakeSpoil(ctx, "c", id, player, TakeInput{CharacterID: "tobin"}); code(err) != 404 {
		t.Errorf("another player's hero: %v", err)
	}
	got, err := fx.s.TakeSpoil(ctx, "c", id, player, TakeInput{CharacterID: "mira"})
	if err != nil || got.Got != "2 × Bloodmoss" || got.Qty != 2 || got.Hero != "Mira" {
		t.Fatalf("take: %+v, %v", got, err)
	}
	if !reflect.DeepEqual(fx.arm.spoils, []string{"mira moss 2 false"}) {
		t.Errorf("given %v", fx.arm.spoils)
	}
	if _, err := fx.s.TakeSpoil(ctx, "c", id, director, TakeInput{CharacterID: "tobin"}); code(err) != 409 {
		t.Errorf("taken twice: %v", err)
	}
	if _, err := fx.s.TakeSpoil(ctx, "c", 999, player, TakeInput{CharacterID: "mira"}); code(err) != 404 {
		t.Errorf("no such line: %v", err)
	}
	cleared := fx.lineFor(t, "Troll Remnant", "moss")
	if err := fx.s.ClearSpoilLine(ctx, "c", cleared, director); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := fx.s.TakeSpoil(ctx, "c", cleared, player, TakeInput{CharacterID: "mira"}); code(err) != 404 {
		t.Errorf("a cleared line: %v", err)
	}
	// The Director can take for any hero, and an item the player couldn't
	// see is named once it is theirs.
	secret := fx.lineFor(t, "Troll Remnant", "secret")
	if got, err := fx.s.TakeSpoil(ctx, "c", secret, director, TakeInput{CharacterID: "tobin"}); err != nil || got.Got != "1 × Dragon's tear" {
		t.Errorf("director's take: %+v, %v", got, err)
	}
}

func TestTakeSpoil_HarvestPaysTheTier(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		dice  []int
		stat  map[string]int
		total int
		tier  int
		got   string
		note  string
	}{
		{"11 or less", []int{4, 3}, map[string]int{"reason": 2}, 9, 0, "1 × Spring water", ""},
		{"12 to 16", []int{6, 5}, map[string]int{"reason": 2}, 13, 1, "2 × Spring water", ""},
		{"17 or more", []int{10, 9}, map[string]int{"reason": 2}, 21, 2, "3 × Spring water", ""},
		{"no number on the sheet", []int{6, 5}, map[string]int{}, 11, 0, "1 × Spring water", "Reason isn’t a number on Mira’s sheet, so it added nothing."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := newSpoilFx()
			fx.s.projects.Sheets = fakeSheets(c.stat)
			fx.fight(t)
			fx.dice = c.dice
			id := fx.lineFor(t, "Troll", "water")
			got, err := fx.s.TakeSpoil(ctx, "c", id, player, TakeInput{CharacterID: "mira"})
			if err != nil {
				t.Fatalf("harvest: %v", err)
			}
			if got.Total != c.total || got.Tier != c.tier || got.Got != c.got || len(got.Tiers) != 3 || got.Tiers[1].Label != "12 to 16" {
				t.Errorf("harvest %+v", got)
			}
			if c.note != "" && (len(got.Notes) != 1 || got.Notes[0] != c.note) {
				t.Errorf("notes %v", got.Notes)
			}
			if l := fx.sp.lines[id]; l.Status != SpoilTaken || l.Rolled == nil || *l.Rolled != c.total || l.CharacterID != "mira" {
				t.Errorf("line %+v", l)
			}
		})
	}

	t.Run("a tier that pays nothing gives nothing", func(t *testing.T) {
		fx := newSpoilFx()
		fx.sp.drops["troll"] = DropList{PageID: "troll", Version: 1, Lines: []DropLine{{ItemID: "water", Mode: DropHarvest, Yields: []int{1, 0, 0}}}}
		fx.fight(t)
		fx.dice = []int{8, 8}
		got, err := fx.s.TakeSpoil(ctx, "c", fx.lineFor(t, "Troll", "water"), player, TakeInput{CharacterID: "mira"})
		if err != nil || got.Got != "nothing" || len(fx.arm.spoils) != 0 {
			t.Errorf("harvest %+v, %v, given %v", got, err, fx.arm.spoils)
		}
	})
}

func TestTakeSpoil_PutBackWhenThePackFails(t *testing.T) {
	fx := newSpoilFx()
	fx.fight(t)
	fx.arm.failGive = true
	id := fx.lineFor(t, "Troll", "moss")
	if _, err := fx.s.TakeSpoil(context.Background(), "c", id, player, TakeInput{CharacterID: "mira"}); err == nil {
		t.Fatal("no error")
	}
	if l := fx.sp.lines[id]; l.Status != SpoilWaiting || l.CharacterID != "" {
		t.Errorf("line %+v", l)
	}
}

func TestTray_APlayerSeesOnlyWhatTheyMay(t *testing.T) {
	ctx := context.Background()
	fx := newSpoilFx()
	fx.fight(t)
	if _, err := fx.s.PutSpoils(ctx, "c", director, PutInput{PageID: "shrine", Copies: 2}); err != nil {
		t.Fatalf("put: %v", err)
	}
	cc, err := fx.s.Character(ctx, "c", "mira", player)
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	if len(cc.Spoils) != 5 {
		t.Fatalf("tray %+v", cc.Spoils)
	}
	remnant := cc.Spoils[2]
	if remnant.Source != "Troll Remnant" || len(remnant.Lines) != 3 {
		t.Fatalf("remnant %+v", remnant)
	}
	if l := remnant.Lines[2]; !l.Unseen || l.Name != unseenItem || l.ItemID != "" {
		t.Errorf("unseen item shows %+v", l)
	}
	if l := remnant.Lines[1]; l.Roll != "2d10 + Reason" || len(l.Tiers) != 3 || l.Tiers[2] != (Tier{Label: "17 or more", Qty: 3}) || l.Kinds != "liquid" {
		t.Errorf("harvest line %+v", l)
	}
	if remnant.Origin != "Shrine cellar" || remnant.ByHand {
		t.Errorf("a fight's spoils say %q, by hand %v", remnant.Origin, remnant.ByHand)
	}
	// A page the player can't see hides its name, how many dropped and
	// where, even that the Director put it in by hand.
	for _, s := range cc.Spoils[3:] {
		if s.Source != unseenSource || s.Copies != 1 || s.Origin != "" || s.ByHand {
			t.Errorf("hidden source shows %+v", s)
		}
	}
	tab, err := fx.s.Spoils(ctx, "c", director)
	if err != nil {
		t.Fatalf("tab: %v", err)
	}
	if s := tab.Tray[4]; s.Source != "The Weeping Shrine" || s.Copy != 2 || s.Copies != 2 || !s.ByHand {
		t.Errorf("the Director's tray %+v", s)
	}
	if l := tab.Tray[2].Lines[2]; l.Unseen || l.Name != "Dragon's tear" {
		t.Errorf("the Director's secret line %+v", l)
	}
	if !reflect.DeepEqual(tab.Tiers, []string{"11 or less", "12 to 16", "17 or more"}) || tab.Dice != "2d10" || len(tab.Stats) != 1 {
		t.Errorf("tab rules %+v %q %+v", tab.Tiers, tab.Dice, tab.Stats)
	}
}

func TestSpoils_TakenLatelyAndDropLists(t *testing.T) {
	ctx := context.Background()
	fx := newSpoilFx()
	fx.fight(t)
	fx.dice = []int{6, 5}
	if _, err := fx.s.TakeSpoil(ctx, "c", fx.lineFor(t, "Troll", "water"), player, TakeInput{CharacterID: "mira"}); err != nil {
		t.Fatal(err)
	}
	fx.s.projects.Now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	if _, err := fx.s.TakeSpoil(ctx, "c", fx.lineFor(t, "Troll", "moss"), director, TakeInput{CharacterID: "tobin"}); err != nil {
		t.Fatal(err)
	}
	tab, err := fx.s.Spoils(ctx, "c", director)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Tobin took 2 × Bloodmoss", "Mira harvested Spring water (rolled 13): 2 × Spring water"}
	if len(tab.Taken) != 2 || tab.Taken[0].Text != want[0] || tab.Taken[1].Text != want[1] {
		t.Errorf("taken %+v", tab.Taken)
	}
	var names []string
	for _, d := range tab.Drops {
		names = append(names, d.Page+"/"+d.AlsoName)
	}
	if strings.Join(names, ", ") != "The Weeping Shrine/, Troll/, Troll Remnant/Troll" {
		t.Errorf("drop lists %v", names)
	}
}

func TestPutAndClearSpoils(t *testing.T) {
	ctx := context.Background()
	fx := newSpoilFx()
	for _, c := range []struct {
		in   PutInput
		code int
	}{
		{PutInput{PageID: "troll", Copies: 0}, 422},
		{PutInput{PageID: "troll", Copies: maxSpoilCopies + 1}, 422},
		{PutInput{PageID: "goblin", Copies: 1}, 422},
		{PutInput{PageID: "nowhere", Copies: 1}, 404},
	} {
		if _, err := fx.s.PutSpoils(ctx, "c", director, c.in); code(err) != c.code {
			t.Errorf("put %+v: %v", c.in, err)
		}
	}
	got, err := fx.s.PutSpoils(ctx, "c", director, PutInput{PageID: "troll", Copies: 2, Origin: "The bridge"})
	if err != nil || got.Spoils != 2 || fx.sp.spoils[1].Origin != "The bridge" {
		t.Fatalf("put: %+v, %v", got, err)
	}
	if err := fx.s.ClearSpoil(ctx, "c", fx.sp.spoils[0].ID, director); err != nil {
		t.Fatal(err)
	}
	if err := fx.s.ClearSpoil(ctx, "c", fx.sp.spoils[0].ID, director); code(err) != 404 {
		t.Errorf("clearing a cleared spoil: %v", err)
	}
	if n, _ := fx.sp.CountWaiting(ctx, "c"); n != 2 {
		t.Errorf("waiting %d", n)
	}
	if err := fx.s.ClearTray(ctx, "c", director); err != nil {
		t.Fatal(err)
	}
	if n, _ := fx.sp.CountWaiting(ctx, "c"); n != 0 {
		t.Errorf("waiting %d after clearing the tray", n)
	}
	// Clearing an empty tray is fine.
	if err := fx.s.ClearTray(ctx, "c", director); err != nil {
		t.Errorf("empty tray: %v", err)
	}
}

func TestDropNotes(t *testing.T) {
	ctx := context.Background()
	fx := newSpoilFx()
	got, err := fx.s.DropNotes(ctx, "c", director, []string{"troll", "goblin"})
	if err != nil || !reflect.DeepEqual(got, map[string]string{"troll": "Has a drop list", "goblin": "No drop list"}) {
		t.Errorf("notes %v, %v", got, err)
	}
	if got, err := fx.s.DropNotes(ctx, "c", director, []string{"goblin"}); err != nil || got != nil {
		t.Errorf("no drop lists among them: %v, %v", got, err)
	}
}

func TestTiers(t *testing.T) {
	if got := TierLabels([]int{11, 12}); !reflect.DeepEqual(got, []string{"11 or less", "12", "13 or more"}) {
		t.Errorf("labels %v", got)
	}
	if got := TierLabels(nil); got[0] != "11 or less" {
		t.Errorf("default labels %v", got)
	}
	for total, want := range map[int]int{-3: 0, 11: 0, 12: 1, 16: 1, 17: 2, 40: 2} {
		if got := tierOf(total, []int{11, 16}); got != want {
			t.Errorf("tier of %d = %d", total, got)
		}
	}
}
