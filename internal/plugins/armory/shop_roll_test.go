package armory

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// fakeRollRepo keeps rolls and notes in memory with the table's unique key.
type fakeRollRepo struct {
	mu      sync.Mutex
	secrets map[int]GoodSecret
	rows    []ShopRoll
	next    int64
}

func (f *fakeRollRepo) Secrets(context.Context, string, string) (map[int]GoodSecret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]GoodSecret{}
	for k, v := range f.secrets {
		out[k] = v
	}
	return out, nil
}

func (f *fakeRollRepo) SaveSecret(_ context.Context, _, _ string, rel int, s GoodSecret) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s.Note == "" && s.Extra == "" {
		delete(f.secrets, rel)
		return nil
	}
	f.secrets[rel] = s
	return nil
}

func (f *fakeRollRepo) InsertRoll(_ context.Context, r ShopRoll) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.rows {
		if x.RelationID == r.RelationID && x.CharacterID == r.CharacterID && x.Action == r.Action && x.ShelfDay == r.ShelfDay {
			return 0, errAlreadyTried
		}
	}
	f.next++
	r.ID = f.next
	f.rows = append(f.rows, r)
	return r.ID, nil
}

func (f *fakeRollRepo) CharacterRolls(_ context.Context, _, _, char string) ([]ShopRoll, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ShopRoll
	for _, x := range f.rows {
		if x.CharacterID == char {
			out = append(out, x)
		}
	}
	return out, nil
}

func (f *fakeRollRepo) Haggles(_ context.Context, _, _, char string, day int64) (map[int]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[int]int{}
	for _, x := range f.rows {
		if x.CharacterID == char && x.Action == RollHaggle && x.ShelfDay == day {
			out[x.RelationID] = x.Pct
		}
	}
	return out, nil
}

type rollRooms struct{ layout string }

func (r *rollRooms) Get(context.Context, string, string) (json.RawMessage, error) {
	if r.layout == "" {
		return nil, nil
	}
	return json.RawMessage(r.layout), nil
}
func (r *rollRooms) Upsert(context.Context, string, string, string, json.RawMessage) error {
	return nil
}

// rollFakeToday is a shelf whose day the test sets and whose goods can be
// taken off it.
type rollFakeToday struct {
	ShopTodayService
	day int64
	off map[int]bool
}

func (f *rollFakeToday) OnShelf(_ context.Context, _, _ string, rel int, _ shopMeta) (bool, error) {
	return !f.off[rel], nil
}
func (f *rollFakeToday) ShelfDayIndex(context.Context, string) (int64, error) { return f.day, nil }

type fakeSheetStats map[string]map[string]int

func (f fakeSheetStats) Stat(_ context.Context, _, char, key string) (int, bool, error) {
	v, ok := f[char][key]
	return v, ok, nil
}

type fakeRollRules struct{ r ShopTestRules }

func (f fakeRollRules) ShopTestRules(context.Context, string) ShopTestRules { return f.r }

type fakeRollLog struct{ entries []ShopRollLogEntry }

func (f *fakeRollLog) Rolled(_ context.Context, _ string, e ShopRollLogEntry) string {
	f.entries = append(f.entries, e)
	return "roll-1"
}

const rollsOn = `{"rolls":{"haggle":true,"look":true,"stubborn":"usual","haggleStat":"","lookStat":"","great":20,"middling":10,"poor":10}}`

func dcRules() ShopTestRules {
	return ShopTestRules{Dice: "1d20", Style: "dc", Haggle: "cha", Look: "int", Tiers: []int{11, 16}, Edge: 2, Bane: -2, DCEasy: 10, DCUsual: 15, DCHard: 20,
		Stats: []ShopTestStat{{Key: "cha", Label: "Charisma", Score: true}, {Key: "int", Label: "Intelligence", Score: true}}}
}

func tierRules() ShopTestRules {
	return ShopTestRules{Dice: "2d10", Style: "tiers", Haggle: "presence", Look: "reason", Tiers: []int{11, 16}, Edge: 2, Bane: -2, DCEasy: 10, DCUsual: 15, DCHard: 20,
		Stats: []ShopTestStat{{Key: "presence", Label: "Presence"}, {Key: "reason", Label: "Reason"}}}
}

type rollFx struct {
	*buyFx
	repo   *fakeRollRepo
	rooms  *rollRooms
	today  *rollFakeToday
	sheets fakeSheetStats
	log    *fakeRollLog
	svc    ShopRollService
	dice   []int
}

// newRollFx builds the counter over the buy fixture: shop-1 sells relation 1
// (Potion, 10 gp) and 2 (Secret, 2.5 gp); Thorin (c1) is u1's, Mira (c2) is u2's.
func newRollFx(rules ShopTestRules, layout string) *rollFx {
	repo := &fakeRollRepo{secrets: map[int]GoodSecret{}}
	today := &rollFakeToday{day: 100, off: map[int]bool{}}
	b := newBuyFxWith(today, WithHaggles(repo))
	f := &rollFx{buyFx: b, repo: repo, rooms: &rollRooms{layout: layout}, today: today,
		sheets: fakeSheetStats{"c1": {"cha": 14, "int": 8, "presence": 2, "reason": 1}, "c2": {}}, log: &fakeRollLog{}}
	f.svc = NewShopRollService(ShopRollDeps{
		Repo: repo, Rooms: f.rooms, Stash: b.fx.svc.(*stashService), Shops: buyShops{}, Rels: b.listings,
		Listings: &fakeListings{}, Today: today, Rules: fakeRollRules{rules}, Sheets: f.sheets, Log: f.log,
	})
	f.svc.(*shopRollService).roll = func(int) int {
		v := f.dice[0]
		f.dice = f.dice[1:]
		return v
	}
	return f
}

// fakeListings is defined by the shelf tests; the roll service lists goods
// only to show a character's state, so an empty list is enough here unless a
// test sets rows.
func (f *rollFx) roll(actor Actor, char string, rel int, action string, dice ...int) (*RollResult, error) {
	f.dice = dice
	return f.svc.Roll(context.Background(), "camp", "shop-1", actor, RollInput{RelationID: RelationRef(rel), CharacterID: char, Action: action})
}

func TestShopRoll_Banding(t *testing.T) {
	tests := []struct {
		name     string
		rules    ShopTestRules
		layout   string
		dice     []int
		wantTier int
		wantTot  int
		wantPct  int
	}{
		// Charisma 14 adds +2; usual DC 15.
		{"dc: below the DC", dcRules(), rollsOn, []int{12}, 1, 14, 10},
		{"dc: meets the DC", dcRules(), rollsOn, []int{13}, 2, 15, -10},
		{"dc: one under beating it by five", dcRules(), rollsOn, []int{17}, 2, 19, -10},
		{"dc: beats it by five", dcRules(), rollsOn, []int{18}, 3, 20, -20},
		{"dc: easy keeper, DC 10", dcRules(), strings.Replace(rollsOn, `"usual"`, `"easy"`, 1), []int{8}, 2, 10, -10},
		{"dc: easy keeper, beats 10 by five", dcRules(), strings.Replace(rollsOn, `"usual"`, `"easy"`, 1), []int{13}, 3, 15, -20},
		{"dc: hard keeper, DC 20", dcRules(), strings.Replace(rollsOn, `"usual"`, `"hard"`, 1), []int{17}, 1, 19, 10},
		{"dc: hard keeper met", dcRules(), strings.Replace(rollsOn, `"usual"`, `"hard"`, 1), []int{18}, 2, 20, -10},
		// Presence 2 on 2d10; cut-offs 11 and 16.
		{"tiers: top of tier 1", tierRules(), rollsOn, []int{4, 5}, 1, 11, 10},
		{"tiers: bottom of tier 2", tierRules(), rollsOn, []int{5, 5}, 2, 12, -10},
		{"tiers: top of tier 2", tierRules(), rollsOn, []int{7, 7}, 2, 16, -10},
		{"tiers: bottom of tier 3", tierRules(), rollsOn, []int{7, 8}, 3, 17, -20},
		{"tiers: easy keeper adds the edge", tierRules(), strings.Replace(rollsOn, `"usual"`, `"easy"`, 1), []int{4, 5}, 2, 13, -10},
		{"tiers: hard keeper adds the bane", tierRules(), strings.Replace(rollsOn, `"usual"`, `"hard"`, 1), []int{5, 5}, 1, 10, 10},
		{"the owner's own percentages", dcRules(), `{"rolls":{"haggle":true,"stubborn":"usual","great":50,"middling":5,"poor":0}}`, []int{18}, 3, 20, -50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRollFx(tt.rules, tt.layout)
			res, err := f.roll(pU1, "c1", 1, RollHaggle, tt.dice...)
			if err != nil {
				t.Fatal(err)
			}
			if res.Tier != tt.wantTier || res.Total != tt.wantTot || res.Haggle.Pct != tt.wantPct {
				t.Errorf("tier %d total %d pct %d, want %d %d %d", res.Tier, res.Total, res.Haggle.Pct, tt.wantTier, tt.wantTot, tt.wantPct)
			}
			if res.Haggle.Was != 10 || res.Haggle.Currency != "gp" {
				t.Errorf("was/currency = %v %q", res.Haggle.Was, res.Haggle.Currency)
			}
		})
	}
}

func TestShopRoll_Result(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	res, err := f.roll(pU1, "c1", 1, RollHaggle, 18)
	if err != nil {
		t.Fatal(err)
	}
	if res.Label != "Charisma test" || res.Outcome != "20% off" || res.Modifier != 2 || res.Haggle.Price != 8 || res.RollID != "roll-1" || res.StatMissing {
		t.Errorf("result = %+v %+v", res, res.Haggle)
	}
	if len(res.Dice) != 1 || res.Dice[0] != (RollDie{Sides: 20, Value: 18}) {
		t.Errorf("dice = %+v", res.Dice)
	}
	raw, _ := json.Marshal(res)
	for _, k := range []string{`"action"`, `"dice"`, `"modifier"`, `"total"`, `"tier"`, `"label"`, `"outcome"`, `"statMissing"`, `"haggle"`, `"rollId"`} {
		if !strings.Contains(string(raw), k) {
			t.Errorf("response %s lacks %s", raw, k)
		}
	}
	if strings.Contains(string(raw), `"look"`) {
		t.Errorf("a haggle answered with a look: %s", raw)
	}
}

func TestShopRoll_StatModifier(t *testing.T) {
	tests := []struct {
		name  string
		stat  ShopTestStat
		value int
		want  int
	}{
		{"score 10", ShopTestStat{Score: true}, 10, 0},
		{"score 11", ShopTestStat{Score: true}, 11, 0},
		{"score 12", ShopTestStat{Score: true}, 12, 1},
		{"score 20", ShopTestStat{Score: true}, 20, 5},
		{"score 9 floors to -1", ShopTestStat{Score: true}, 9, -1},
		{"score 8", ShopTestStat{Score: true}, 8, -1},
		{"score 7 floors to -2", ShopTestStat{Score: true}, 7, -2},
		{"score 1", ShopTestStat{Score: true}, 1, -5},
		{"plain adds the value", ShopTestStat{}, 3, 3},
		{"plain negative", ShopTestStat{}, -2, -2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statModifier(tt.stat, tt.value); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestShopRoll_MissingStatAddsNothing(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	res, err := f.roll(pU2, "c2", 1, RollHaggle, 15) // Mira's sheet has no charisma
	if err != nil {
		t.Fatal(err)
	}
	if !res.StatMissing || res.Modifier != 0 || res.Total != 15 || res.Tier != 2 {
		t.Errorf("result = %+v", res)
	}
}

func TestShopRoll_NoStatsConfigured(t *testing.T) {
	r := dcRules()
	r.Stats, r.Haggle, r.Look = nil, "", ""
	f := newRollFx(r, rollsOn)
	res, err := f.roll(pU1, "c1", 1, RollHaggle, 15)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatMissing || res.Modifier != 0 || res.Label != "Haggle test" {
		t.Errorf("result = %+v", res)
	}
}

func TestShopRoll_OwnerPicksTheStat(t *testing.T) {
	// The owner's haggleStat wins over the system's default.
	f := newRollFx(dcRules(), strings.Replace(rollsOn, `"haggleStat":""`, `"haggleStat":"int"`, 1))
	res, err := f.roll(pU1, "c1", 1, RollHaggle, 15) // int 8 gives -1
	if err != nil {
		t.Fatal(err)
	}
	if res.Label != "Intelligence test" || res.Modifier != -1 || res.Total != 14 {
		t.Errorf("result = %+v", res)
	}
}

func TestShopRoll_Refusals(t *testing.T) {
	tests := []struct {
		name   string
		layout string
		actor  Actor
		char   string
		rel    int
		action string
		setup  func(*rollFx)
		want   int
	}{
		{"no rolls in the layout", "", pU1, "c1", 1, RollHaggle, nil, 403},
		{"layout without rolls", `{"version":1}`, pU1, "c1", 1, RollHaggle, nil, 403},
		{"haggle off", strings.Replace(rollsOn, `"haggle":true`, `"haggle":false`, 1), pU1, "c1", 1, RollHaggle, nil, 403},
		{"look off", strings.Replace(rollsOn, `"look":true`, `"look":false`, 1), pU1, "c1", 1, RollLook, nil, 403},
		{"look still works with haggle off", strings.Replace(rollsOn, `"haggle":true`, `"haggle":false`, 1), pU1, "c1", 1, RollLook, nil, 0},
		{"not on today's shelf", rollsOn, pU1, "c1", 1, RollHaggle, func(f *rollFx) { f.today.off[1] = true }, 400},
		{"no price to haggle over", rollsOn, pU1, "c1", 8, RollHaggle, nil, 400},
		{"a good with no price can still be looked at", rollsOn, pU1, "c1", 8, RollLook, nil, 0},
		{"someone else's character", rollsOn, pU1, "c2", 1, RollHaggle, nil, 403},
		{"the owner may roll for any character", rollsOn, owner, "c2", 1, RollHaggle, nil, 0},
		{"not a character", rollsOn, pU1, "i1", 1, RollHaggle, nil, 404},
		{"unknown action", rollsOn, pU1, "c1", 1, "steal", nil, 400},
		{"no character", rollsOn, pU1, "", 1, RollHaggle, nil, 400},
		{"no listing named", rollsOn, pU1, "c1", 0, RollHaggle, nil, 400},
		{"not one of this shop's listings", rollsOn, pU1, "c1", 4, RollHaggle, nil, 404},
		{"not a sells listing", rollsOn, pU1, "c1", 5, RollHaggle, nil, 404},
		{"a hidden listing", rollsOn, pU1, "c1", 7, RollHaggle, nil, 404},
		{"a good the player cannot see", rollsOn, pU1, "c1", 2, RollHaggle, func(f *rollFx) { f.vis.hidden["i2"] = true }, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRollFx(dcRules(), tt.layout)
			if tt.setup != nil {
				tt.setup(f)
			}
			_, err := f.roll(tt.actor, tt.char, tt.rel, tt.action, 10)
			if code(err) != tt.want {
				t.Errorf("code = %d (%v), want %d", code(err), err, tt.want)
			}
			if tt.want != 0 && len(f.repo.rows) != 0 {
				t.Errorf("a refused roll stored %+v", f.repo.rows)
			}
		})
	}
}

func TestShopRoll_OneTryPerGoodCharacterActionDay(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 10); err != nil {
		t.Fatal(err)
	}
	_, err := f.roll(pU1, "c1", 1, RollHaggle, 20)
	if code(err) != 409 || !strings.Contains(err.Error(), "already haggled over this today") {
		t.Errorf("second haggle: %v", err)
	}
	if len(f.repo.rows) != 1 || f.repo.rows[0].Tier != 1 {
		t.Errorf("the second try changed the stored roll: %+v", f.repo.rows)
	}
	// Each of these is a different try.
	for _, c := range []struct {
		name   string
		actor  Actor
		char   string
		rel    int
		action string
	}{
		{"look at the same good", pU1, "c1", 1, RollLook},
		{"haggle over another good", pU1, "c1", 2, RollHaggle},
		{"another character", owner, "c2", 1, RollHaggle},
	} {
		if _, err := f.roll(c.actor, c.char, c.rel, c.action, 10); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := f.roll(pU1, "c1", 1, RollLook, 10); code(err) != 409 || !strings.Contains(err.Error(), "already looked closer at this today") {
		t.Errorf("second look: %v", err)
	}
	f.today.day++
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 10); err != nil {
		t.Errorf("the next shelf day: %v", err)
	}
}

func TestShopRoll_RefusedTryDoesNotSpendTheDay(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.today.off[1] = true
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 10); code(err) != 400 {
		t.Fatal(err)
	}
	f.today.off[1] = false
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 10); err != nil {
		t.Errorf("after the good came back: %v", err)
	}
}

func TestShopRoll_LookRevealsByTier(t *testing.T) {
	tests := []struct {
		name        string
		die         int
		wantTier    int
		wantNote    string
		wantExtra   string
		wantOutcome string
	}{
		// Intelligence 14 adds +2; usual DC 15.
		{"tier 1 shows nothing", 12, 1, "", "", "Nothing more to see."},
		{"tier 2 shows the note", 13, 2, "Loose stone behind the till", "", "Noticed something"},
		{"tier 3 shows the extra too", 18, 3, "Loose stone behind the till", "Sells to smugglers", "Noticed something"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRollFx(dcRules(), rollsOn)
			f.sheets["c1"]["int"] = 14
			f.repo.secrets[1] = GoodSecret{Note: "Loose stone behind the till", Extra: "Sells to smugglers"}
			res, err := f.roll(pU1, "c1", 1, RollLook, tt.die)
			if err != nil {
				t.Fatal(err)
			}
			if res.Tier != tt.wantTier || res.Look == nil || res.Look.Note != tt.wantNote || res.Look.Extra != tt.wantExtra || res.Outcome != tt.wantOutcome || res.Haggle != nil {
				t.Errorf("result = %+v look %+v", res, res.Look)
			}
			if res.Label != "Intelligence test" {
				t.Errorf("label = %q", res.Label)
			}
		})
	}
}

// listedGoods makes the overview read goods: the shop lists relations 1 and 2.
func (f *rollFx) listedGoods() {
	f.svc.(*shopRollService).Listings = &fakeListings{rows: []ShopListing{
		listing(1, "i1", `{"price":10,"currency":"gp"}`),
		listing(2, "i2", `{"price":2.5,"currency":"gp"}`),
	}}
}

func (f *rollFx) rolls(actor Actor, char string) (*RollsView, error) {
	return f.svc.Rolls(context.Background(), "camp", "shop-1", actor, char)
}

func TestShopRoll_RevealPersistsAcrossDays_HagglesDoNot(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.listedGoods()
	f.sheets["c1"]["int"] = 14
	f.repo.secrets[1] = GoodSecret{Note: "the note", Extra: "the extra"}
	if _, err := f.roll(pU1, "c1", 1, RollLook, 18); err != nil {
		t.Fatal(err)
	}
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 18); err != nil {
		t.Fatal(err)
	}

	v, err := f.rolls(pU1, "c1")
	if err != nil {
		t.Fatal(err)
	}
	g := v.Goods["1"]
	if g.Look == nil || g.Look.Tier != 3 || g.Look.Note != "the note" || g.Look.Extra != "the extra" || !g.Look.Today {
		t.Errorf("today's look = %+v", g.Look)
	}
	if g.Haggle == nil || g.Haggle.Tier != 3 || g.Haggle.Pct != -20 || g.Haggle.Price != 8 || g.Haggle.Was != 10 {
		t.Errorf("today's haggle = %+v", g.Haggle)
	}

	f.today.day += 5
	v, _ = f.rolls(pU1, "c1")
	g = v.Goods["1"]
	if g.Look == nil || g.Look.Note != "the note" || g.Look.Extra != "the extra" || g.Look.Today {
		t.Errorf("a later day's look = %+v", g.Look)
	}
	if g.Haggle != nil {
		t.Errorf("yesterday's haggle is still shown: %+v", g.Haggle)
	}

	// A poor look later never takes back what a good one uncovered.
	if _, err := f.roll(pU1, "c1", 1, RollLook, 1); err != nil {
		t.Fatal(err)
	}
	v, _ = f.rolls(pU1, "c1")
	if g := v.Goods["1"]; g.Look.Tier != 3 || g.Look.Note != "the note" || !g.Look.Today {
		t.Errorf("after a poor look = %+v", g.Look)
	}
	// Another character has uncovered nothing.
	if v, _ := f.rolls(owner, "c2"); len(v.Goods) != 0 {
		t.Errorf("another character sees %+v", v.Goods)
	}
}

func TestShopRoll_RevealFollowsTheOwnersLaterEdit(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.listedGoods()
	f.sheets["c1"]["int"] = 14
	f.repo.secrets[1] = GoodSecret{Note: "old"}
	if _, err := f.roll(pU1, "c1", 1, RollLook, 13); err != nil {
		t.Fatal(err)
	}
	f.repo.secrets[1] = GoodSecret{Note: "new"}
	if v, _ := f.rolls(pU1, "c1"); v.Goods["1"].Look.Note != "new" {
		t.Errorf("note = %q", v.Goods["1"].Look.Note)
	}
	delete(f.repo.secrets, 1)
	if v, _ := f.rolls(pU1, "c1"); v.Goods["1"].Look.Note != "" || v.Goods["1"].Look.Tier != 2 {
		t.Errorf("after the note was removed = %+v", v.Goods["1"].Look)
	}
}

func TestShopRoll_OverviewHidesGoodsTheCharacterCannotSee(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.listedGoods()
	f.repo.secrets[2] = GoodSecret{Note: "hidden thing"}
	f.sheets["c1"]["int"] = 14
	if _, err := f.roll(pU1, "c1", 2, RollLook, 18); err != nil {
		t.Fatal(err)
	}
	f.vis.hidden["i2"] = true
	v, _ := f.rolls(pU1, "c1")
	if len(v.Goods) != 0 {
		t.Errorf("a hidden good's reveal is shown: %+v", v.Goods)
	}
}

func TestShopRoll_Overview(t *testing.T) {
	t.Run("with a character", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		v, err := f.rolls(pU1, "c1")
		if err != nil {
			t.Fatal(err)
		}
		h := v.Test.Haggle
		if !v.Haggle || !v.Look || v.Test.Dice != "1d20" || v.Test.Style != "dc" || v.Test.Against != "DC 15" ||
			h.Stat != "cha" || h.Label != "Charisma" || h.Value == nil || *h.Value != 14 || *h.Mod != 2 || *h.Missing {
			t.Errorf("view = %+v haggle %+v", v, h)
		}
		if l := v.Test.Look; l.Stat != "int" || *l.Value != 8 || *l.Mod != -1 {
			t.Errorf("look = %+v", l)
		}
		if len(v.Stats) != 2 || v.Stats[0] != (RollStatChoice{Key: "cha", Label: "Charisma"}) || v.Defaults != (RollDefaults{Haggle: "cha", Look: "int"}) || v.Goods == nil {
			t.Errorf("stats/defaults = %+v %+v", v.Stats, v.Defaults)
		}
	})
	t.Run("tiers style says Easy, Usual or Hard", func(t *testing.T) {
		f := newRollFx(tierRules(), strings.Replace(rollsOn, `"usual"`, `"hard"`, 1))
		v, _ := f.rolls(pU1, "c1")
		if v.Test.Against != "Hard" || v.Test.StubbornMod != -2 || v.Test.Haggle.Stat != "presence" {
			t.Errorf("test = %+v", v.Test)
		}
	})
	t.Run("a missing field is reported", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		v, _ := f.rolls(owner, "c2")
		if !*v.Test.Haggle.Missing || *v.Test.Haggle.Mod != 0 {
			t.Errorf("haggle = %+v", v.Test.Haggle)
		}
	})
	t.Run("the owner without a character gets the settings view", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		v, err := f.rolls(owner, "")
		if err != nil {
			t.Fatal(err)
		}
		if v.Test.Haggle.Value != nil || v.Test.Haggle.Mod != nil || v.Test.Haggle.Missing != nil || len(v.Stats) != 2 ||
			v.Defaults.Haggle != "cha" || v.Defaults.Look != "int" || v.Goods == nil || len(v.Goods) != 0 {
			t.Errorf("view = %+v", v)
		}
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), `"value"`) || strings.Contains(string(raw), `"mod"`) || !strings.Contains(string(raw), `"goods":{}`) {
			t.Errorf("wire = %s", raw)
		}
	})
	t.Run("a player needs a character", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		if _, err := f.rolls(pU1, ""); code(err) != 400 {
			t.Errorf("code = %d", code(err))
		}
	})
	t.Run("a player cannot read another's character", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		if _, err := f.rolls(pU1, "c2"); code(err) != 403 {
			t.Errorf("code = %d", code(err))
		}
	})
	t.Run("a shop the player cannot see", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		f.vis.hidden["shop-1"] = true
		if _, err := f.rolls(pU1, "c1"); code(err) != 404 {
			t.Errorf("code = %d", code(err))
		}
	})
	t.Run("switches off with no layout", func(t *testing.T) {
		f := newRollFx(dcRules(), "")
		v, _ := f.rolls(pU1, "c1")
		if v.Haggle || v.Look {
			t.Errorf("view = %+v", v)
		}
	})
}

// A roll the table sees holds the dice and the outcome, never what a look uncovered.
func TestShopRoll_LogNeverHoldsTheNote(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.sheets["c1"]["int"] = 14
	f.repo.secrets[1] = GoodSecret{Note: "SECRETNOTE", Extra: "SECRETEXTRA"}
	if _, err := f.roll(pU1, "c1", 1, RollLook, 18); err != nil {
		t.Fatal(err)
	}
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 18); err != nil {
		t.Fatal(err)
	}
	if _, err := f.roll(pU1, "c1", 2, RollLook, 1); err != nil {
		t.Fatal(err)
	}
	if len(f.log.entries) != 3 {
		t.Fatalf("logged %d rolls", len(f.log.entries))
	}
	raw, _ := json.Marshal(f.log.entries)
	if strings.Contains(string(raw), "SECRET") {
		t.Errorf("the log holds the note: %s", raw)
	}
	want := []struct{ label, outcome string }{
		{"Look closer: Potion", "Noticed something"}, {"Haggle: Potion", "20% off"}, {"Look closer: Secret", "Nothing more"},
	}
	for i, w := range want {
		e := f.log.entries[i]
		if e.Label != w.label || e.Outcome != w.outcome || e.From != "The Gilded Cup" || e.RollerName != "Thorin" || e.RollerID != "u1" || e.Formula != "1d20" {
			t.Errorf("entry %d = %+v", i, e)
		}
	}
}

func TestShopRoll_LogOutcomes(t *testing.T) {
	for _, tt := range []struct {
		die  int
		want string
	}{{12, "Price up 10%"}, {13, "10% off"}, {18, "20% off"}} {
		f := newRollFx(dcRules(), rollsOn)
		if _, err := f.roll(pU1, "c1", 1, RollHaggle, tt.die); err != nil {
			t.Fatal(err)
		}
		if got := f.log.entries[0].Outcome; got != tt.want {
			t.Errorf("die %d: outcome %q, want %q", tt.die, got, tt.want)
		}
	}
}

func TestShopRoll_RollWithoutALog(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.svc.(*shopRollService).Log = nil
	res, err := f.roll(pU1, "c1", 1, RollHaggle, 18)
	if err != nil || res.RollID != "" || res.Haggle == nil {
		t.Errorf("result %+v err %v", res, err)
	}
}

func TestHaggledUnit(t *testing.T) {
	tests := []struct {
		unit Cents
		pct  int
		want Cents
	}{
		{1000, -20, 800}, {1000, -10, 900}, {1000, 10, 1100}, {250, -10, 225},
		{105, -10, 95},  // 94.5 rounds up
		{3, -90, 1},     // 0.3 cents never below one
		{1, -90, 1},     // a one cent good stays one cent
		{1000, 0, 1000}, // no change
		{1000, 200, 3000},
	}
	for _, tt := range tests {
		if got := haggledUnit(tt.unit, tt.pct); got != tt.want {
			t.Errorf("haggledUnit(%d, %d) = %d, want %d", tt.unit, tt.pct, got, tt.want)
		}
	}
}

// The buy route charges the haggled price, for that character, that good, that day.
func TestShopRoll_HaggleChangesTheBasketPrice(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.openDowntime()
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 18); err != nil { // 20% off a 10 gp potion
		t.Fatal(err)
	}
	priced := func(actor Actor, char string, rel int) Cents {
		t.Helper()
		_, _, total, err := f.buyFx.svc.(*shopBuyService).priceBasket(context.Background(), "camp", "shop-1", actor, char, basket(char, [2]int{rel, 1}), true)
		if err != nil {
			t.Fatal(err)
		}
		return total
	}
	if got := priced(pU1, "c1", 1); got != 800 {
		t.Errorf("the haggler pays %d, want 800", got)
	}
	if got := priced(pU1, "c1", 2); got != 250 {
		t.Errorf("another good costs %d, want the listed 250", got)
	}
	if got := priced(owner, "c2", 1); got != 1000 {
		t.Errorf("another character pays %d, want the listed 1000", got)
	}
	f.today.day++
	if got := priced(pU1, "c1", 1); got != 1000 {
		t.Errorf("the next day costs %d, want the listed 1000", got)
	}
	f.today.day--

	// And the purchase itself charges it and records it.
	res, err := f.buyFx.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 2}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Spent == nil || *res.Spent != 16 || *res.MoneyLeft != 34 || f.money("c1") != 34.0 {
		t.Errorf("result %+v money %v, want 16 spent and 34 left", res, f.money("c1"))
	}
	if len(f.txRows) != 1 || f.txRows[0].PriceNumeric == nil || *f.txRows[0].PriceNumeric != 16 || *f.txRows[0].PricePaid != "16 gp" {
		t.Errorf("the purchase row = %+v", f.txRows[0])
	}
	if f.listings.stock(1) != 3 {
		t.Errorf("stock = %d", f.listings.stock(1))
	}
}

func TestShopRoll_FailedHaggleRaisesThePrice(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.openDowntime()
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 1); err != nil {
		t.Fatal(err)
	}
	res, err := f.buyFx.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}))
	if err != nil || *res.Spent != 11 {
		t.Errorf("result %+v err %v, want 11 spent", res, err)
	}
}

func TestShopRoll_WithoutHagglesTheListedPriceStands(t *testing.T) {
	b := newBuyFxWith(&rollFakeToday{day: 100})
	b.openDowntime()
	res, err := b.svc.Buy(context.Background(), "camp", "shop-1", pU1, basket("c1", [2]int{1, 1}))
	if err != nil || *res.Spent != 10 {
		t.Errorf("result %+v err %v", res, err)
	}
}

// A queued request is quoted at the haggled price and approved at it the same day.
func TestShopRoll_QueuedRequestKeepsTheHaggledQuote(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	if _, err := f.roll(pU1, "c1", 1, RollHaggle, 18); err != nil {
		t.Fatal(err)
	}
	id := f.ask(t, pU1, basket("c1", [2]int{1, 1}))
	if q := f.reqs.rows[0].QuotedTotal; q != 800 {
		t.Fatalf("quote = %d, want 800", q)
	}
	if _, err := f.buyFx.svc.ApproveRequest(context.Background(), "camp", owner, id); err != nil {
		t.Fatalf("same-day approval: %v", err)
	}
	if f.money("c1") != 42.0 {
		t.Errorf("money = %v, want 42", f.money("c1"))
	}
	// Approved after the shelf changed, the haggle no longer holds and the
	// price is above the quote, so it is refused rather than overcharged.
	g := newRollFx(dcRules(), rollsOn)
	if _, err := g.roll(pU1, "c1", 1, RollHaggle, 18); err != nil {
		t.Fatal(err)
	}
	id = g.ask(t, pU1, basket("c1", [2]int{1, 1}))
	g.today.day++
	req, err := g.buyFx.svc.ApproveRequest(context.Background(), "camp", owner, id)
	if err != nil || req.Status != PurchaseFailed || !strings.Contains(g.reqs.reason(id), "price went up") || g.money("c1") != 50.0 {
		t.Errorf("next-day approval = %+v err %v reason %q money %v, want a failed request and no charge", req, err, g.reqs.reason(id), g.money("c1"))
	}
}

func TestShopRoll_Secrets(t *testing.T) {
	ctx := context.Background()
	t.Run("only the owner reads or writes", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		if _, err := f.svc.Secrets(ctx, "camp", "shop-1", pU1); code(err) != 403 {
			t.Errorf("player read code = %d", code(err))
		}
		if _, err := f.svc.Secrets(ctx, "camp", "shop-1", Actor{UserID: "s", Role: rScribe}); code(err) != 403 {
			t.Errorf("scribe read code = %d", code(err))
		}
		if _, err := f.svc.SaveSecret(ctx, "camp", "shop-1", 1, pU1, GoodSecret{Note: "x"}); code(err) != 403 {
			t.Errorf("player write code = %d", code(err))
		}
		if len(f.repo.secrets) != 0 {
			t.Errorf("a player wrote %+v", f.repo.secrets)
		}
	})
	t.Run("save, read, trim and delete", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		out, err := f.svc.SaveSecret(ctx, "camp", "shop-1", 1, owner, GoodSecret{Note: "  a note \n", Extra: "\tan extra"})
		if err != nil || out.Note != "a note" || out.Extra != "an extra" {
			t.Fatalf("saved %+v err %v", out, err)
		}
		all, err := f.svc.Secrets(ctx, "camp", "shop-1", owner)
		if err != nil || all["1"] != (GoodSecret{Note: "a note", Extra: "an extra"}) || len(all) != 1 {
			t.Errorf("secrets = %+v err %v", all, err)
		}
		if _, err := f.svc.SaveSecret(ctx, "camp", "shop-1", 1, owner, GoodSecret{Note: " ", Extra: ""}); err != nil {
			t.Fatal(err)
		}
		if all, _ := f.svc.Secrets(ctx, "camp", "shop-1", owner); len(all) != 0 {
			t.Errorf("both empty did not delete: %+v", all)
		}
	})
	t.Run("limits and plain text", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			in   GoodSecret
			want int
		}{
			{"400 characters is fine", GoodSecret{Note: strings.Repeat("é", 400)}, 0},
			{"401 note characters", GoodSecret{Note: strings.Repeat("a", 401)}, 400},
			{"401 extra characters", GoodSecret{Extra: strings.Repeat("a", 401)}, 400},
			{"a control character", GoodSecret{Note: "a\x00b"}, 400},
			{"an escape", GoodSecret{Extra: "a\x1bb"}, 400},
		} {
			f := newRollFx(dcRules(), rollsOn)
			if _, err := f.svc.SaveSecret(ctx, "camp", "shop-1", 1, owner, tt.in); code(err) != tt.want {
				t.Errorf("%s: code = %d", tt.name, code(err))
			}
		}
	})
	t.Run("only on this shop's listings", func(t *testing.T) {
		f := newRollFx(dcRules(), rollsOn)
		for _, rel := range []int{4, 5, 99} {
			if _, err := f.svc.SaveSecret(ctx, "camp", "shop-1", rel, owner, GoodSecret{Note: "x"}); code(err) != 404 {
				t.Errorf("relation %d: code = %d", rel, code(err))
			}
		}
		if _, err := f.svc.SaveSecret(ctx, "camp", "shop-9", 1, owner, GoodSecret{Note: "x"}); code(err) != 404 {
			t.Errorf("unknown shop: code = %d", code(err))
		}
	})
}

// The notes live in their own table: nothing the room layout or the rolls
// view sends a player can hold one.
func TestShopRoll_NoteNeverInPlayerViews(t *testing.T) {
	f := newRollFx(dcRules(), rollsOn)
	f.listedGoods()
	f.repo.secrets[1] = GoodSecret{Note: "SECRETNOTE", Extra: "SECRETEXTRA"}
	v, _ := f.rolls(pU1, "c1")
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "SECRET") {
		t.Errorf("an unrevealed note leaked: %s", raw)
	}
	if strings.Contains(rollsOn, "SECRET") || strings.Contains(string(mustJSON(ShopRoomRolls{})), "note") {
		t.Error("the layout's rolls settings can hold a note")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
