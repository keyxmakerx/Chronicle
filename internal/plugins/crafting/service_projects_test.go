package crafting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------- Project fakes ----------

type fakeStore struct {
	projects map[string]Project
	order    []string
	rolls    int
	respites []Respite
}

func newFakeStore() *fakeStore { return &fakeStore{projects: map[string]Project{}} }

func (f *fakeStore) CreateProject(_ context.Context, p Project) error {
	p.Version = 1
	f.projects[p.ID] = p
	f.order = append(f.order, p.ID)
	return nil
}
func (f *fakeStore) GetProject(_ context.Context, campaignID, id string) (Project, bool, error) {
	p, ok := f.projects[id]
	if !ok || p.CampaignID != campaignID {
		return Project{}, false, nil
	}
	return p, true, nil
}
func (f *fakeStore) ListProjects(_ context.Context, campaignID, characterID, status string, limit int) ([]Project, error) {
	out := []Project{}
	for _, id := range f.order {
		p := f.projects[id]
		if p.CampaignID == campaignID && p.Status == status && (characterID == "" || p.CharacterID == characterID) {
			out = append(out, p)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (f *fakeStore) CountActive(ctx context.Context, campaignID, characterID string) (int, error) {
	l, _ := f.ListProjects(ctx, campaignID, characterID, ProjectActive, 0)
	return len(l), nil
}
func (f *fakeStore) UpdateProject(_ context.Context, p Project, expected int) (bool, error) {
	old, ok := f.projects[p.ID]
	if !ok || old.Version != expected {
		return false, nil
	}
	old.Points, old.RollsReady, old.Status, old.Delivered, old.FinishedAt = p.Points, p.RollsReady, p.Status, p.Delivered, p.FinishedAt
	old.Version++
	f.projects[p.ID] = old
	return true, nil
}
func (f *fakeStore) AddRoll(context.Context, string, string, string, string, string, int, int, int) error {
	f.rolls++
	return nil
}
func (f *fakeStore) LogRespite(_ context.Context, campaignID, _, dayLabel string) (int, error) {
	n := 0
	for id, p := range f.projects {
		if p.CampaignID == campaignID && p.Status == ProjectActive {
			p.RollsReady = min(p.RollsReady+1, maxRollsReady)
			f.projects[id] = p
			n++
		}
	}
	f.respites = append(f.respites, Respite{DayLabel: dayLabel, Projects: n})
	return n, nil
}
func (f *fakeStore) LastRespite(context.Context, string) (*Respite, error) {
	if len(f.respites) == 0 {
		return nil, nil
	}
	r := f.respites[len(f.respites)-1]
	return &r, nil
}

// fakeArmory is Mira's pack: four Bloodmoss, two Spring water and 30 gp.
type fakeArmory struct {
	have     map[string]int
	gold     int64
	spent    [][]PackItem
	given    map[string]int
	spoils   []string
	failGive bool
}

func newFakeArmory() *fakeArmory {
	return &fakeArmory{have: map[string]int{"moss": 4, "water": 2}, gold: 3000, given: map[string]int{}}
}

func (a *fakeArmory) Pack(context.Context, string, Viewer, string) (*Pack, error) {
	p := &Pack{Name: "Mira", PayKind: "coins", Have: a.gold, MoneyLabel: "Gold"}
	for _, id := range []string{"moss", "water"} {
		if a.have[id] > 0 {
			p.Items = append(p.Items, PackItem{ItemID: id, Name: dirEntities[id].Name, Qty: a.have[id]})
		}
	}
	return p, nil
}
func (a *fakeArmory) Charge(_ *Pack, c Cost) int64 { return int64(c.Amount * 100) }
func (a *fakeArmory) Spend(_ context.Context, _ string, _ Viewer, _ string, items []PackItem, c Cost, _ string) (string, error) {
	need := map[string]int{}
	for _, it := range items {
		need[it.ItemID] += it.Qty
	}
	for id, n := range need {
		if a.have[id] < n {
			return "", errInvalid("Mira doesn't carry enough " + dirEntities[id].Name)
		}
	}
	if int64(c.Amount*100) > a.gold {
		return "", errInvalid("Mira can't pay")
	}
	for id, n := range need {
		a.have[id] -= n
	}
	a.gold -= int64(c.Amount * 100)
	a.spent = append(a.spent, items)
	if c.Amount > 0 {
		return "25 gp", nil
	}
	return "", nil
}
func (a *fakeArmory) Give(_ context.Context, _ string, _ Viewer, _, itemID string, qty int, _ string) error {
	if a.failGive {
		return errors.New("pack is gone")
	}
	a.given[itemID] += qty
	return nil
}
func (a *fakeArmory) GiveSpoil(_ context.Context, _ string, _ Viewer, characterID, itemID string, qty int, harvested bool) error {
	if a.failGive {
		return errors.New("pack is gone")
	}
	a.spoils = append(a.spoils, fmt.Sprintf("%s %s %d %v", characterID, itemID, qty, harvested))
	return nil
}

type fakeSheets map[string]int

func (f fakeSheets) Stat(_ context.Context, _, _, key string) (int, bool, error) {
	n, ok := f[key]
	return n, ok, nil
}

type fakeDays struct{ day *Day }

func (f *fakeDays) Today(context.Context, string) (*Day, error) { return f.day, nil }

type fakeRespiteHook struct{ calls int }

func (f *fakeRespiteHook) RespiteLogged(context.Context, string, Viewer) error {
	f.calls++
	return nil
}

type projectFx struct {
	s     *service
	repo  *fakeRepo
	store *fakeStore
	arm   *fakeArmory
	days  *fakeDays
	hook  *fakeRespiteHook
	dice  []int
}

// newProjectFx is a campaign with one healing potion recipe everyone knows:
// two Bloodmoss and any herb, 25 gp, Reason, a goal of 30. Dice come from
// fx.dice in order, and are 1 once they run out.
func newProjectFx() *projectFx {
	s, repo, _ := newTestService()
	fx := &projectFx{s: s, repo: repo, store: newFakeStore(), arm: newFakeArmory(), days: &fakeDays{}, hook: &fakeRespiteHook{}}
	repo.tags = map[string][]string{"moss": {"herb"}, "water": {"liquid"}}
	repo.recipes["heal"] = Recipe{ID: "heal", Name: "Healing potion", Template: "potion", Who: "all", Goal: 30, Stat: "reason",
		Materials: []Material{{ItemID: "moss", Qty: 2}, {Tag: "herb", Qty: 1}},
		Steps:     []Step{{T: "grind", Turns: 3}},
		Result:    Result{ItemID: "potion", Qty: 1}, Cost: Cost{Amount: 25, Currency: "gp"}, Version: 1}
	repo.order = []string{"heal"}
	s.projects = ProjectDeps{Store: fx.store, Armory: fx.arm, Sheets: fakeSheets{"reason": 2}, Days: fx.days, Respite: fx.hook,
		Roll: func(int) int {
			if len(fx.dice) == 0 {
				return 1
			}
			d := fx.dice[0]
			fx.dice = fx.dice[1:]
			return d
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}
	return fx
}

// start begins the potion for Mira as the player, filling the herb with
// Bloodmoss.
func (fx *projectFx) start(t *testing.T) *ProjectView {
	t.Helper()
	pv, err := fx.s.StartProject(context.Background(), "c", "mira", player, StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return pv
}

func (fx *projectFx) ready(id string, n int) {
	p := fx.store.projects[id]
	p.RollsReady = n
	fx.store.projects[id] = p
}

func (fx *projectFx) roll(t *testing.T, id, work string) (*RollResult, error) {
	t.Helper()
	return fx.s.RollProject(context.Background(), "c", id, player, RollInput{Work: work, Version: fx.store.projects[id].Version})
}

func termTexts(r *RollResult) string {
	var parts []string
	for _, t := range r.Terms {
		parts = append(parts, t.Text)
	}
	return strings.Join(parts, " | ")
}

// ---------- Tests ----------

func TestStartProject_TakesWhatItNeeds(t *testing.T) {
	fx := newProjectFx()
	pv := fx.start(t)
	if fx.arm.have["moss"] != 1 || fx.arm.gold != 500 {
		t.Fatalf("pack after start: moss %d, gold %d; want 1 and 500", fx.arm.have["moss"], fx.arm.gold)
	}
	if pv.Status != ProjectActive || pv.RollsReady != 0 || pv.Goal != 30 || pv.Makes != "1 × Healing potion" {
		t.Fatalf("project = %+v", pv)
	}
	if pv.Took != "2 × Bloodmoss, 1 × Bloodmoss, 25 gp" {
		t.Fatalf("took = %q", pv.Took)
	}
}

func TestStartProject_Refusals(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		setup func(fx *projectFx)
		who   Viewer
		char  string
		in    StartInput
		code  int
	}{
		{"another player's hero", nil, player, "tobin", StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}}, 404},
		{"no herb picked", nil, player, "mira", StartInput{RecipeID: "heal"}, 422},
		{"picked item without the tag", nil, player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "water"}}, 422},
		{"picked item not carried", nil, player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "potion"}}, 422},
		{"draft recipe", func(fx *projectFx) { r := fx.repo.recipes["heal"]; r.Draft = true; fx.repo.recipes["heal"] = r },
			player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}}, 404},
		{"not learned", func(fx *projectFx) { r := fx.repo.recipes["heal"]; r.Who = "learned"; fx.repo.recipes["heal"] = r },
			player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}}, 404},
		{"no goal", func(fx *projectFx) { r := fx.repo.recipes["heal"]; r.Goal = 0; fx.repo.recipes["heal"] = r },
			player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}}, 422},
		{"no result item", func(fx *projectFx) { r := fx.repo.recipes["heal"]; r.Result.ItemID = ""; fx.repo.recipes["heal"] = r },
			player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}}, 422},
		{"too little moss", func(fx *projectFx) { fx.arm.have["moss"] = 2 },
			player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}}, 422},
		{"too many projects", func(fx *projectFx) {
			for i := 0; i < maxActiveProjects; i++ {
				_ = fx.store.CreateProject(ctx, Project{ID: "p" + string(rune('a'+i)), CampaignID: "c", CharacterID: "mira", Status: ProjectActive})
			}
		}, player, "mira", StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"}}, 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newProjectFx()
			if tc.setup != nil {
				tc.setup(fx)
			}
			before := len(fx.store.order)
			_, err := fx.s.StartProject(ctx, "c", tc.char, tc.who, tc.in)
			if code(err) != tc.code {
				t.Fatalf("code = %d (%v), want %d", code(err), err, tc.code)
			}
			if len(fx.store.order) != before || len(fx.arm.spent) != 0 {
				t.Fatalf("a refused start saved a project or took something")
			}
		})
	}
}

func TestStartProject_LearnedRecipe(t *testing.T) {
	fx := newProjectFx()
	r := fx.repo.recipes["heal"]
	r.Who, r.Known = "learned", []string{"mira"}
	fx.repo.recipes["heal"] = r
	fx.start(t)
}

func TestRollProject_AddsUpAndFinishes(t *testing.T) {
	fx := newProjectFx()
	pv := fx.start(t)
	fx.ready(pv.ID, 2)

	fx.dice = []int{7, 8}
	res, err := fx.roll(t, pv.ID, WorkClean)
	if err != nil {
		t.Fatal(err)
	}
	// 15 on the dice, 2 Reason, 2 for the edge.
	if res.Total != 19 || res.From != 0 || res.To != 19 || res.Finished {
		t.Fatalf("first roll = %+v", res)
	}
	if got := termTexts(res); got != "2d10 | Reason, from Mira’s sheet | An edge: clean work at the bench" {
		t.Fatalf("terms = %q", got)
	}
	if res.Project.RollsReady != 1 || res.Project.Points != 19 || len(res.Dice) != 2 || res.Sides != 10 {
		t.Fatalf("after first roll: %+v", res.Project)
	}

	fx.dice = []int{10, 9}
	res, err = fx.roll(t, pv.ID, WorkBotched)
	if err != nil {
		t.Fatal(err)
	}
	// 19 on the dice, 2 Reason, -2 for the bane: 19, over the goal by 8.
	if res.Total != 19 || res.To != 30 || !res.Finished || res.Made != "1 × Healing potion" {
		t.Fatalf("finishing roll = %+v", res)
	}
	if res.Project.Status != ProjectMade || res.Project.RollsReady != 0 {
		t.Fatalf("finished project = %+v", res.Project)
	}
	if fx.arm.given["potion"] != 1 || !fx.store.projects[pv.ID].Delivered {
		t.Fatalf("potion not delivered: given %v, delivered %v", fx.arm.given, fx.store.projects[pv.ID].Delivered)
	}
	if !strings.Contains(strings.Join(res.Notes, " "), "8 points over the goal are dropped.") {
		t.Fatalf("notes = %q", res.Notes)
	}
	if fx.store.rolls != 2 {
		t.Fatalf("rolls recorded = %d, want 2", fx.store.rolls)
	}
	if _, err := fx.roll(t, pv.ID, WorkClean); code(err) != 409 {
		t.Fatalf("rolling a made project: %v, want 409", err)
	}
}

type fakeRollLog struct{ got []ProjectRoll }

func (f *fakeRollLog) ProjectRolled(_ context.Context, _ string, _ Viewer, r ProjectRoll) {
	f.got = append(f.got, r)
}

func TestRollProject_ReachesTheRollLog(t *testing.T) {
	fx := newProjectFx()
	log := &fakeRollLog{}
	fx.s.projects.Rolls = log
	pv := fx.start(t)
	fx.ready(pv.ID, 1)
	fx.dice = []int{7, 8}
	if _, err := fx.roll(t, pv.ID, WorkClean); err != nil {
		t.Fatal(err)
	}
	if len(log.got) != 1 {
		t.Fatalf("logged %d rolls, want 1", len(log.got))
	}
	r := log.got[0]
	if r.Hero != "Mira" || r.Recipe != "Healing potion" || r.Sides != 10 || len(r.Dice) != 2 || r.Total != 19 || r.To != 19 || r.Goal != 30 || r.Finished {
		t.Fatalf("logged %+v", r)
	}
}

func TestRollProject_Refusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(fx *projectFx, id string)
		in    func(fx *projectFx, id string) RollInput
		who   Viewer
		code  int
	}{
		{"no roll ready", func(fx *projectFx, id string) {}, nil, player, 409},
		{"stale version", func(fx *projectFx, id string) { fx.ready(id, 1) },
			func(fx *projectFx, id string) RollInput { return RollInput{Work: WorkClean, Version: 99} }, player, 409},
		{"unknown work", func(fx *projectFx, id string) { fx.ready(id, 1) },
			func(fx *projectFx, id string) RollInput {
				return RollInput{Work: "perfect", Version: fx.store.projects[id].Version}
			}, player, 422},
		{"skipping turned off", func(fx *projectFx, id string) {
			fx.ready(id, 1)
			set := DefaultSettings()
			set.Skip = false
			fx.repo.settings = &set
		}, func(fx *projectFx, id string) RollInput {
			return RollInput{Work: WorkSkipped, Version: fx.store.projects[id].Version}
		}, player, 422},
		{"someone else's project", func(fx *projectFx, id string) { fx.ready(id, 1) }, nil,
			Viewer{UserID: "other", MemberRole: 1, VisibilityRole: 1}, 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newProjectFx()
			pv := fx.start(t)
			tc.setup(fx, pv.ID)
			in := RollInput{Work: WorkClean, Version: fx.store.projects[pv.ID].Version}
			if tc.in != nil {
				in = tc.in(fx, pv.ID)
			}
			before := fx.store.projects[pv.ID]
			_, err := fx.s.RollProject(context.Background(), "c", pv.ID, tc.who, in)
			if code(err) != tc.code {
				t.Fatalf("code = %d (%v), want %d", code(err), err, tc.code)
			}
			if fx.store.projects[pv.ID] != before {
				t.Fatalf("a refused roll changed the project")
			}
		})
	}
}

func TestRollProject_SkipAllowedForDirector(t *testing.T) {
	fx := newProjectFx()
	pv := fx.start(t)
	fx.ready(pv.ID, 1)
	set := DefaultSettings()
	set.Skip = false
	fx.repo.settings = &set
	res, err := fx.s.RollProject(context.Background(), "c", pv.ID, director, RollInput{Work: WorkSkipped, Version: fx.store.projects[pv.ID].Version})
	if err != nil {
		t.Fatal(err)
	}
	if res.Notes[0] != workNotes[WorkSkipped] {
		t.Fatalf("notes = %q", res.Notes)
	}
}

func TestRollProject_PlainWhenTheRecipeCantBeBenched(t *testing.T) {
	// Skipping is off and the recipe has steps, but it went back to draft
	// mid-project: the block can't play it, so a plain roll is allowed.
	fx := newProjectFx()
	pv := fx.start(t)
	fx.ready(pv.ID, 1)
	set := DefaultSettings()
	set.Skip = false
	fx.repo.settings = &set
	if _, err := fx.roll(t, pv.ID, WorkSkipped); code(err) != 422 {
		t.Fatalf("skipping a recipe the hero can bench: %v, want 422", err)
	}
	r := fx.repo.recipes["heal"]
	r.Draft = true
	fx.repo.recipes["heal"] = r
	res, err := fx.roll(t, pv.ID, WorkSkipped)
	if err != nil {
		t.Fatal(err)
	}
	if res.Notes[0] != "There are no steps to work, so it was a plain roll." {
		t.Fatalf("notes = %q", res.Notes)
	}
}

func TestRollProject_BotchedWithoutBanes(t *testing.T) {
	fx := newProjectFx()
	pv := fx.start(t)
	fx.ready(pv.ID, 1)
	set := DefaultSettings()
	set.Bane = false
	fx.repo.settings = &set
	fx.dice = []int{3, 4}
	res, err := fx.roll(t, pv.ID, WorkBotched)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 9 || !strings.Contains(res.Notes[0], "banes are off") {
		t.Fatalf("roll = %+v", res)
	}
}

func TestRollProject_StatMissingFromSheet(t *testing.T) {
	fx := newProjectFx()
	fx.s.projects.Sheets = fakeSheets{}
	pv := fx.start(t)
	fx.ready(pv.ID, 1)
	fx.dice = []int{3, 4}
	res, err := fx.roll(t, pv.ID, WorkSteady)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 7 || !strings.Contains(strings.Join(res.Notes, " "), "Reason isn’t a number on Mira’s sheet") {
		t.Fatalf("roll = %+v", res)
	}
}

func TestRollProject_RecipeRules(t *testing.T) {
	full := &Day{Label: "1 Frostmoot", Moons: []DayMoon{{Name: "Selune", Phase: 0.5}}}
	newMoon := &Day{Label: "15 Frostmoot", Moons: []DayMoon{{Name: "Selune", Phase: 0}}}
	moonFull := []Cond{{Kind: "moon", Moon: "Selune", Phase: "full"}}
	cases := []struct {
		name    string
		day     *Day
		rules   []Rule
		dice    []int
		work    string
		code    int
		total   int
		spoiled bool
		terms   string
		note    string
		made    int
	}{
		{name: "only under a full moon, on a new moon", day: newMoon,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "only"}, Players: "book", BookLine: "Only brewed under a full Selune."}},
			work:  WorkSteady, code: 409},
		{name: "only under a full moon, on a full moon", day: full,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "only"}, Players: "book", BookLine: "Only brewed under a full Selune."}},
			dice:  []int{3, 4}, work: WorkSteady, total: 9},
		{name: "certain can't be made holds", day: full,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "cannot"}, Players: "book"}}, work: WorkSteady, code: 409},
		{name: "a draft rule counts for nothing", day: full,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "cannot"}, Players: "book", Draft: true}},
			dice:  []int{3, 4}, work: WorkSteady, total: 9},
		{name: "a chance of can't be made that comes up spoils the roll", day: full,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "cannot"}, Chance: "1 in 4", Players: "secret"}},
			dice:  []int{1}, work: WorkClean, total: 0, spoiled: true, terms: "A rule the Director keeps hidden"},
		{name: "a chance that doesn't come up does nothing", day: full,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "cannot"}, Chance: "1 in 4", Players: "book"}},
			dice:  []int{2, 3, 4}, work: WorkSteady, total: 9},
		{name: "a rule's bane cancels clean work's edge", day: full,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "bane"}, Players: "book", BookLine: "The full moon curdles it."}},
			dice:  []int{3, 4}, work: WorkClean, total: 9,
			terms: "2d10 | Reason, from Mira’s sheet | An edge and a bane cancel: clean work at the bench; The full moon curdles it"},
		{name: "points and words", day: full,
			rules: []Rule{
				{When: moonFull, Then: Effect{Kind: "points", Amount: 5}, Players: "book", BookLine: "Moonlight helps."},
				{When: moonFull, Then: Effect{Kind: "say", Text: "The brew glows."}, Players: "whenItHappens"},
				{When: moonFull, Then: Effect{Kind: "say", Text: "Hidden words."}, Players: "secret"},
			},
			dice: []int{3, 4}, work: WorkSteady, total: 14, note: "The brew glows."},
		{name: "a bench rule is unknown until the roll, and then off without a bench", day: full,
			rules: []Rule{{When: []Cond{{Kind: "bench", Bench: "Tower lab"}}, Then: Effect{Kind: "only"}, Players: "book"}},
			work:  WorkSteady, code: 409},
		{name: "no calendar leaves moon rules off", day: nil,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "points", Amount: 5}, Players: "book"}},
			dice:  []int{3, 4}, work: WorkSteady, total: 9},
		{name: "a rule needing the time of day stays maybe and never holds", day: full,
			rules: []Rule{{When: []Cond{{Kind: "time", Time: "night"}}, Then: Effect{Kind: "only"}, Players: "book"}},
			dice:  []int{3, 4}, work: WorkSteady, total: 9},
		{name: "extra makes more when it finishes", day: full,
			rules: []Rule{{When: moonFull, Then: Effect{Kind: "extra", Amount: 2}, Players: "book"}},
			dice:  []int{10, 10}, work: WorkClean, total: 24, made: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newProjectFx()
			fx.days.day = tc.day
			pv := fx.start(t)
			r := fx.repo.recipes["heal"]
			r.Rules = tc.rules
			fx.repo.recipes["heal"] = r
			fx.ready(pv.ID, 1)
			if tc.made > 0 {
				p := fx.store.projects[pv.ID]
				p.Points = 20
				fx.store.projects[pv.ID] = p
			}
			fx.dice = tc.dice
			res, err := fx.roll(t, pv.ID, tc.work)
			if tc.code != 0 {
				if code(err) != tc.code {
					t.Fatalf("code = %d (%v), want %d", code(err), err, tc.code)
				}
				if fx.store.projects[pv.ID].RollsReady != 1 {
					t.Fatalf("a held roll was used")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.Total != tc.total || res.Spoiled != tc.spoiled {
				t.Fatalf("total %d spoiled %v, want %d %v; terms %q notes %q", res.Total, res.Spoiled, tc.total, tc.spoiled, termTexts(res), res.Notes)
			}
			if tc.terms != "" && termTexts(res) != tc.terms {
				t.Fatalf("terms = %q, want %q", termTexts(res), tc.terms)
			}
			notes := strings.Join(res.Notes, " ")
			if tc.note != "" && !strings.Contains(notes, tc.note) {
				t.Fatalf("notes = %q, want %q", notes, tc.note)
			}
			if strings.Contains(notes, "Hidden words.") {
				t.Fatalf("a secret rule's words reached the player: %q", notes)
			}
			if tc.made > 0 && fx.arm.given["potion"] != tc.made {
				t.Fatalf("made %d potions, want %d", fx.arm.given["potion"], tc.made)
			}
			if fx.store.projects[pv.ID].RollsReady != 0 && !res.Finished {
				t.Fatalf("the roll was not used")
			}
		})
	}
}

func TestRollProject_WaitsShownOnTheBlock(t *testing.T) {
	fx := newProjectFx()
	fx.days.day = &Day{Moons: []DayMoon{{Name: "Selune", Phase: 0}}}
	pv := fx.start(t)
	r := fx.repo.recipes["heal"]
	r.Rules = []Rule{{When: []Cond{{Kind: "moon", Moon: "Selune", Phase: "full"}}, Then: Effect{Kind: "only"}, Players: "secret"}}
	fx.repo.recipes["heal"] = r
	fx.ready(pv.ID, 1)
	got, err := fx.s.Character(context.Background(), "c", "mira", player)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Projects) != 1 || got.Projects[0].Waits != "A rule the Director keeps hidden." {
		t.Fatalf("projects = %+v", got.Projects)
	}
	if got.Projects[0].Took != "" {
		t.Fatalf("a player sees what the project took: %q", got.Projects[0].Took)
	}
}

func TestRollProject_FailedGiveLeavesItUndelivered(t *testing.T) {
	fx := newProjectFx()
	pv := fx.start(t)
	fx.ready(pv.ID, 1)
	p := fx.store.projects[pv.ID]
	p.Points = 29
	fx.store.projects[pv.ID] = p
	fx.arm.failGive = true
	res, err := fx.roll(t, pv.ID, WorkSteady)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Finished || res.Made != "" || fx.store.projects[pv.ID].Delivered {
		t.Fatalf("roll = %+v", res)
	}
	page, err := fx.s.Projects(context.Background(), "c", director)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Finished) != 1 || !page.Finished[0].Undelivered {
		t.Fatalf("finished = %+v", page.Finished)
	}
}

func TestCharacter_ShowsWhatEachRecipeTakes(t *testing.T) {
	fx := newProjectFx()
	got, err := fx.s.Character(context.Background(), "c", "mira", player)
	if err != nil {
		t.Fatal(err)
	}
	if got.Director || got.Pack.Have != 3000 || len(got.Recipes) != 1 {
		t.Fatalf("block = %+v", got)
	}
	r := got.Recipes[0]
	if r.Charge != 2500 || r.StatValue == nil || *r.StatValue != 2 || r.Makes != "Healing potion" || len(r.Needs) != 2 || r.Needs[1].Tag != "herb" {
		t.Fatalf("recipe = %+v", r)
	}
	for _, it := range got.Pack.Items {
		if it.ItemID == "moss" && (len(it.Tags) != 1 || it.Tags[0] != "herb") {
			t.Fatalf("moss tags = %v", it.Tags)
		}
	}
	if _, err := fx.s.Character(context.Background(), "c", "tobin", player); code(err) != 404 {
		t.Fatalf("another player's hero: %v, want 404", err)
	}
	if ok, _ := fx.s.CanCraftFor(context.Background(), "c", "tobin", player); ok {
		t.Fatal("CanCraftFor says yes for another player's hero")
	}
	if ok, _ := fx.s.CanCraftFor(context.Background(), "c", "tobin", director); !ok {
		t.Fatal("CanCraftFor says no for the Director")
	}
}

// fakeCalendar is the default calendar's id as each viewer sees it.
type fakeCalendar map[string]string

func (f fakeCalendar) DefaultCalendarID(_ context.Context, _ string, _ int, userID string) string {
	return f[userID]
}

func TestCharacter_TodayOnlyForWhoSeesTheCalendar(t *testing.T) {
	ctx := context.Background()
	fx := newProjectFx()
	fx.days.day = &Day{Label: "14 Frostmoot"}
	for _, c := range []struct {
		name  string
		cal   CalendarSource
		v     Viewer
		today string
	}{
		{"a player who sees the calendar", fakeCalendar{"player": "cal-1", "dm": "cal-1"}, player, "14 Frostmoot"},
		// The rules read the day with the Director's sight; a calendar
		// hidden from the player keeps its date from them.
		{"a player the calendar is hidden from", fakeCalendar{"dm": "cal-1"}, player, ""},
		{"the Director", fakeCalendar{"dm": "cal-1"}, director, "14 Frostmoot"},
		{"no calendar plugin", nil, director, ""},
	} {
		fx.s.projects.Calendar = c.cal
		got, err := fx.s.Character(ctx, "c", "mira", c.v)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Today != c.today || (c.today != "") != (got.CalendarID != "") {
			t.Errorf("%s: today %q, calendar %q", c.name, got.Today, got.CalendarID)
		}
	}
}

func TestCancelProject_KeepsWhatItTook(t *testing.T) {
	fx := newProjectFx()
	pv := fx.start(t)
	if err := fx.s.CancelProject(context.Background(), "c", pv.ID, player); err != nil {
		t.Fatal(err)
	}
	if fx.store.projects[pv.ID].Status != ProjectCancelled || fx.arm.have["moss"] != 1 {
		t.Fatalf("after cancel: %+v, moss %d", fx.store.projects[pv.ID], fx.arm.have["moss"])
	}
	if err := fx.s.CancelProject(context.Background(), "c", pv.ID, player); code(err) != 409 {
		t.Fatalf("cancelling twice: %v, want 409", err)
	}
}

func TestRespite_GivesEveryProjectARoll(t *testing.T) {
	ctx := context.Background()
	fx := newProjectFx()
	fx.days.day = &Day{Label: "3 Frostmoot"}
	pv := fx.start(t)
	if _, err := fx.s.LogRespite(ctx, "c", player); code(err) != 403 {
		t.Fatalf("player logging a respite: %v, want 403", err)
	}
	res, err := fx.s.LogRespite(ctx, "c", director)
	if err != nil || res.Projects != 1 || fx.hook.calls != 1 {
		t.Fatalf("respite = %+v, %v; hook calls %d", res, err, fx.hook.calls)
	}
	// From the drawer, Homebrew's own rules are already running.
	if err := fx.s.RespitePressed(ctx, "c", director); err != nil || fx.hook.calls != 1 {
		t.Fatalf("pressed: %v; hook calls %d", err, fx.hook.calls)
	}
	if fx.store.projects[pv.ID].RollsReady != 2 {
		t.Fatalf("rolls ready = %d, want 2 (they stack)", fx.store.projects[pv.ID].RollsReady)
	}
	page, err := fx.s.Projects(ctx, "c", director)
	if err != nil {
		t.Fatal(err)
	}
	if page.LastRespite == nil || page.LastRespite.DayLabel != "3 Frostmoot" || page.Today != "3 Frostmoot" || len(page.Active) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if page.Active[0].Took == "" || page.Active[0].Hero != "Mira" {
		t.Fatalf("the Director's row = %+v", page.Active[0])
	}
}

func TestDirectorTools(t *testing.T) {
	ctx := context.Background()
	fx := newProjectFx()
	pv := fx.start(t)
	for name, call := range map[string]func() error{
		"projects":  func() error { _, err := fx.s.Projects(ctx, "c", player); return err },
		"give roll": func() error { _, err := fx.s.GiveRoll(ctx, "c", pv.ID, player); return err },
		"set points": func() error {
			_, err := fx.s.SetPoints(ctx, "c", pv.ID, player, SetPointsInput{Points: 5, Version: 1})
			return err
		},
	} {
		if err := call(); code(err) != 403 {
			t.Fatalf("%s as a player: %v, want 403", name, err)
		}
	}
	got, err := fx.s.GiveRoll(ctx, "c", pv.ID, director)
	if err != nil || got.RollsReady != 1 {
		t.Fatalf("give roll = %+v, %v", got, err)
	}
	if _, err := fx.s.SetPoints(ctx, "c", pv.ID, director, SetPointsInput{Points: 30, Version: got.Version}); code(err) != 422 {
		t.Fatalf("points at the goal: %v, want 422", err)
	}
	if _, err := fx.s.SetPoints(ctx, "c", pv.ID, director, SetPointsInput{Points: 5, Version: 1}); code(err) != 409 {
		t.Fatalf("stale version: %v, want 409", err)
	}
	got, err = fx.s.SetPoints(ctx, "c", pv.ID, director, SetPointsInput{Points: 12, Version: got.Version})
	if err != nil || got.Points != 12 {
		t.Fatalf("set points = %+v, %v", got, err)
	}
}

func TestProjects_NotSetUp(t *testing.T) {
	s, _, _ := newTestService()
	if _, err := s.Character(context.Background(), "c", "mira", player); code(err) != 404 {
		t.Fatalf("no store: %v, want 404", err)
	}
	if ok, err := s.CanCraftFor(context.Background(), "c", "mira", player); ok || err != nil {
		t.Fatalf("CanCraftFor with no store = %v, %v", ok, err)
	}
}
