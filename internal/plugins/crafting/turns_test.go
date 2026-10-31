package crafting

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// withTonic adds a recipe Mira doesn't know and can find by experimenting:
// two Bloodmoss and one Spring water, close at one of two right.
func withTonic(fx *projectFx, d Discover) {
	fx.repo.recipes["tonic"] = Recipe{ID: "tonic", Name: "Moonlit tonic", Template: "potion", Who: "learned", Goal: 10,
		Materials: []Material{{ItemID: "moss", Qty: 2}, {ItemID: "water", Qty: 1}},
		Result:    Result{ItemID: "potion", Qty: 1}, Extras: Extras{Discover: &d}, Version: 1}
	fx.repo.order = append(fx.repo.order, "tonic")
}

func TestExperiment(t *testing.T) {
	hint := "The moss wants water drawn at dawn."
	tests := []struct {
		name      string
		discover  Discover
		picks     []TryPick
		outcome   string
		recipe    string
		hint      string
		used      []string
		back      []string
		learned   []string
		moss, wat int
	}{
		{"all right finds it and uses what went in", Discover{CloseWhen: "1 of 2", Hint: hint, LearnedBy: "maker", FailedTry: TryHalf},
			[]TryPick{{"moss", 2}, {"water", 1}}, TryFound, "Moonlit tonic", hint, []string{"2 × Bloodmoss", "1 × Spring water"}, []string{}, []string{"Mira"}, 2, 1},
		{"the party learns it when the recipe says so", Discover{CloseWhen: "1 of 2", LearnedBy: "party", FailedTry: TryUsed},
			[]TryPick{{"water", 1}, {"moss", 2}}, TryFound, "Moonlit tonic", "", []string{"1 × Spring water", "2 × Bloodmoss"}, []string{}, []string{"Mira", "Tobin"}, 2, 1},
		{"a near miss gives the hint and costs half", Discover{CloseWhen: "1 of 2", Hint: hint, LearnedBy: "maker", FailedTry: TryHalf},
			[]TryPick{{"moss", 2}, {"water", 2}}, TryClose, "", hint, []string{"1 × Bloodmoss", "1 × Spring water"}, []string{"1 × Bloodmoss", "1 × Spring water"}, nil, 3, 1},
		{"a near miss that keeps everything", Discover{CloseWhen: "1 of 2", Hint: hint, LearnedBy: "maker", FailedTry: TryKept},
			[]TryPick{{"moss", 2}}, TryClose, "", hint, []string{}, []string{"2 × Bloodmoss"}, nil, 4, 2},
		{"the wrong amount is not right", Discover{CloseWhen: "1 of 2", Hint: hint, LearnedBy: "maker", FailedTry: TryKept},
			[]TryPick{{"moss", 3}}, TryNothing, "", "", []string{"3 × Bloodmoss"}, []string{}, nil, 1, 2},
		{"something extra spoils a find", Discover{CloseWhen: "1 of 2", Hint: hint, LearnedBy: "maker", FailedTry: TryKept},
			[]TryPick{{"moss", 2}, {"water", 1}, {"potion", 1}}, TryClose, "", hint, []string{}, []string{"2 × Bloodmoss", "1 × Spring water", "1 × Healing potion"}, nil, 4, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newProjectFx()
			fx.arm.have["potion"] = 1
			withTonic(fx, tt.discover)
			got, err := fx.s.Experiment(context.Background(), "c", "mira", player, TryInput{Items: tt.picks})
			if err != nil {
				t.Fatalf("experiment: %v", err)
			}
			if got.Outcome != tt.outcome || got.Recipe != tt.recipe || got.Hint != tt.hint {
				t.Fatalf("got %s %q hint %q; want %s %q hint %q", got.Outcome, got.Recipe, got.Hint, tt.outcome, tt.recipe, tt.hint)
			}
			if tt.outcome != TryFound && got.RecipeID != "" {
				t.Fatalf("a try that found nothing named the recipe's id %q", got.RecipeID)
			}
			if !reflect.DeepEqual(got.Used, tt.used) || !reflect.DeepEqual(got.Back, tt.back) {
				t.Fatalf("used %v back %v; want %v and %v", got.Used, got.Back, tt.used, tt.back)
			}
			if !reflect.DeepEqual(got.Learned, tt.learned) {
				t.Fatalf("learned by %v; want %v", got.Learned, tt.learned)
			}
			if fx.arm.have["moss"] != tt.moss || fx.arm.have["water"] != tt.wat {
				t.Fatalf("pack: moss %d water %d; want %d and %d", fx.arm.have["moss"], fx.arm.have["water"], tt.moss, tt.wat)
			}
			known := map[string]bool{}
			for _, k := range fx.repo.recipes["tonic"].Known {
				known[k] = true
			}
			if (tt.outcome == TryFound) != known["mira"] {
				t.Fatalf("Mira knows the tonic: %v after a %s", known["mira"], tt.outcome)
			}
			if len(got.Used) > 0 && fx.arm.whys[0] != "in an experiment" {
				t.Fatalf("the history says %q", fx.arm.whys[0])
			}
		})
	}
}

func TestExperiment_NothingNearUsesEverything(t *testing.T) {
	fx := newProjectFx()
	withTonic(fx, Discover{CloseWhen: "1 of 2", LearnedBy: "maker", FailedTry: TryKept})
	got, err := fx.s.Experiment(context.Background(), "c", "mira", player, TryInput{Items: []TryPick{{"moss", 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != TryNothing || got.Cost != TryUsed || fx.arm.have["moss"] != 3 {
		t.Fatalf("got %s costing %s, moss %d; want nothing, used, 3", got.Outcome, got.Cost, fx.arm.have["moss"])
	}
}

func TestExperiment_OnceAtEachRespite(t *testing.T) {
	fx := newProjectFx()
	ctx := context.Background()
	withTonic(fx, Discover{CloseWhen: "1 of 2", LearnedBy: "maker", FailedTry: TryKept})
	try := func() error {
		_, err := fx.s.Experiment(ctx, "c", "mira", player, TryInput{Items: []TryPick{{"moss", 1}}})
		return err
	}
	if err := try(); err != nil {
		t.Fatal(err)
	}
	if err := try(); code(err) != 409 {
		t.Fatalf("a second try before a respite: %v; want 409", err)
	}
	cc, _ := fx.s.Character(ctx, "c", "mira", player)
	if cc.Try == nil || cc.Try.Ready {
		t.Fatalf("the block says the try is %+v; want spent", cc.Try)
	}
	if _, err := fx.s.LogRespite(ctx, "c", director); err != nil {
		t.Fatal(err)
	}
	if err := try(); err != nil {
		t.Fatalf("a try after the respite: %v", err)
	}
}

func TestExperiment_RefusesWithoutSpendingTheTurn(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		hero  string
		picks []TryPick
		code  int
	}{
		{"more than the hero holds", "mira", []TryPick{{"moss", 5}}, 422},
		{"nothing at all", "mira", nil, 422},
		{"another player's hero", "tobin", []TryPick{{"moss", 1}}, 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newProjectFx()
			withTonic(fx, Discover{CloseWhen: "1 of 2", LearnedBy: "maker", FailedTry: TryUsed})
			_, err := fx.s.Experiment(ctx, "c", tt.hero, player, TryInput{Items: tt.picks})
			if code(err) != tt.code {
				t.Fatalf("got %v; want %d", err, tt.code)
			}
			if len(fx.store.turns) != 0 || fx.arm.have["moss"] != 4 {
				t.Fatalf("a refused try spent the turn (%v) or moss (%d)", fx.store.turns, fx.arm.have["moss"])
			}
		})
	}
}

// A recipe with "Finding it by experimenting" off is never found, and a
// campaign with nothing left to find shows no "Try something new".
func TestExperiment_OnlyRecipesSetToBeFound(t *testing.T) {
	fx := newProjectFx()
	ctx := context.Background()
	withTonic(fx, Discover{})
	r := fx.repo.recipes["tonic"]
	r.Extras.Discover = nil
	fx.repo.recipes["tonic"] = r
	cc, err := fx.s.Character(ctx, "c", "mira", player)
	if err != nil {
		t.Fatal(err)
	}
	if cc.Try != nil {
		t.Fatalf("Try something new shows with nothing to find: %+v", cc.Try)
	}
	got, err := fx.s.Experiment(ctx, "c", "mira", player, TryInput{Items: []TryPick{{"moss", 2}, {"water", 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != TryNothing || len(fx.repo.recipes["tonic"].Known) != 0 {
		t.Fatalf("an unfindable recipe was %s", got.Outcome)
	}
}

func TestTryRight_PairsEachThingOnce(t *testing.T) {
	tags := map[string][]string{"moss": {"herb"}, "root": {"herb"}}
	mats := []Material{{Tag: "herb", Qty: 1}, {ItemID: "moss", Qty: 1}}
	if n := tryRight(mats, []TryPick{{"moss", 1}}, tags); n != 1 {
		t.Fatalf("one moss filled %d materials; want 1", n)
	}
	if n := tryRight(mats, []TryPick{{"moss", 1}, {"root", 1}}, tags); n != 2 {
		t.Fatalf("moss and root filled %d; want 2 (root the herb, moss the moss)", n)
	}
}

func TestBreakDown(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		salvage []Salvage
		potions int
		code    int
		back    []string
		notes   int
	}{
		{"gives back what the recipe says", []Salvage{{Item: "Spring water", Qty: 1}}, 1, 0, []string{"1 × Spring water"}, 0},
		{"a line that names no item is noted", []Salvage{{Item: "Spring water", Qty: 2}, {Item: "any herb", Qty: 1}}, 1, 0, []string{"2 × Spring water"}, 1},
		{"a recipe that can't be broken down", nil, 1, 422, nil, 0},
		{"none to break down", []Salvage{{Item: "Spring water", Qty: 1}}, 0, 422, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newProjectFx()
			fx.arm.have["potion"] = tt.potions
			r := fx.repo.recipes["heal"]
			r.Extras.Salvage = tt.salvage
			fx.repo.recipes["heal"] = r
			got, err := fx.s.BreakDown(ctx, "c", "mira", player, BreakInput{RecipeID: "heal"})
			if code(err) != tt.code {
				t.Fatalf("got %v; want code %d", err, tt.code)
			}
			if tt.code != 0 {
				if fx.arm.have["potion"] != tt.potions {
					t.Fatal("a refused break-down took the potion")
				}
				return
			}
			if !reflect.DeepEqual(got.Back, tt.back) || len(got.Notes) != tt.notes || fx.arm.have["potion"] != 0 {
				t.Fatalf("back %v notes %v potions %d", got.Back, got.Notes, fx.arm.have["potion"])
			}
			if fx.arm.whys[0] != "to break it down" || !strings.HasPrefix(fx.arm.gaveFor[0], "mira>mira: Got ") {
				t.Fatalf("history: %v %v", fx.arm.whys, fx.arm.gaveFor)
			}
		})
	}
}

// tobinsPotion is Tobin's potion in progress, 20 of 30.
func tobinsPotion(fx *projectFx, othersCanHelp bool) Project {
	r := fx.repo.recipes["heal"]
	r.Extras.OthersCanHelp = othersCanHelp
	fx.repo.recipes["heal"] = r
	p := Project{ID: "tp", CampaignID: "c", CharacterID: "tobin", RecipeID: "heal", RecipeName: "Healing potion", Goal: 30, Stat: "reason",
		ResultItemID: "potion", ResultQty: 1, Points: 20, Status: ProjectActive}
	_ = fx.store.CreateProject(context.Background(), p)
	return fx.store.projects["tp"]
}

func TestHelp(t *testing.T) {
	fx := newProjectFx()
	ctx := context.Background()
	p := tobinsPotion(fx, true)
	fx.dice = []int{3, 4}
	res, err := fx.s.Help(ctx, "c", "tp", player, HelpInput{CharacterID: "mira", Version: p.Version})
	if err != nil {
		t.Fatal(err)
	}
	// 3 + 4 and Mira's Reason of 2.
	if res.Total != 9 || res.To != 29 || fx.store.projects["tp"].RollsReady != 0 {
		t.Fatalf("total %d to %d rolls ready %d; want 9, 29, 0", res.Total, res.To, fx.store.projects["tp"].RollsReady)
	}
	if !strings.HasPrefix(res.Notes[0], "Mira lent a hand") || !strings.Contains(termTexts(res), "Mira’s sheet") {
		t.Fatalf("notes %v terms %s", res.Notes, termTexts(res))
	}
	if _, err := fx.s.Help(ctx, "c", "tp", player, HelpInput{CharacterID: "mira", Version: fx.store.projects["tp"].Version}); code(err) != 409 {
		t.Fatalf("a second hand this respite: %v; want 409", err)
	}
	cc, _ := fx.s.Character(ctx, "c", "mira", player)
	if len(cc.Party) != 1 || cc.Party[0].Project.Hero != "Tobin" || !cc.Party[0].Helped {
		t.Fatalf("the party's projects: %+v", cc.Party)
	}
	_, _ = fx.s.LogRespite(ctx, "c", director)
	fx.dice = []int{5, 5}
	res, err = fx.s.Help(ctx, "c", "tp", player, HelpInput{CharacterID: "mira", Version: fx.store.projects["tp"].Version})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Finished || fx.arm.given["potion"] != 1 || !strings.HasPrefix(fx.arm.gaveFor[0], "mira>tobin: Made 1 × Healing potion") {
		t.Fatalf("finished %v given %v history %v", res.Finished, fx.arm.given, fx.arm.gaveFor)
	}
	// The respite's roll stays Tobin's.
	if fx.store.projects["tp"].RollsReady != 0 || fx.store.projects["tp"].Status != ProjectMade {
		t.Fatalf("project after: %+v", fx.store.projects["tp"])
	}
}

func TestHelp_Refused(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		canHelp bool
		project string
		hero    string
		code    int
	}{
		{"a recipe that doesn't let others help", false, "tp", "mira", 422},
		{"another player's hero helping", true, "tp", "tobin", 404},
		{"no such project", true, "nope", "mira", 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newProjectFx()
			p := tobinsPotion(fx, tt.canHelp)
			_, err := fx.s.Help(ctx, "c", tt.project, player, HelpInput{CharacterID: tt.hero, Version: p.Version})
			if code(err) != tt.code {
				t.Fatalf("got %v; want %d", err, tt.code)
			}
			if len(fx.store.turns) != 0 || fx.store.projects["tp"].Points != 20 {
				t.Fatal("a refused hand spent the turn or moved the points")
			}
		})
	}
}

func TestHelp_OwnProjectIsRolledFromInProgress(t *testing.T) {
	fx := newProjectFx()
	pv := fx.start(t)
	r := fx.repo.recipes["heal"]
	r.Extras.OthersCanHelp = true
	fx.repo.recipes["heal"] = r
	_, err := fx.s.Help(context.Background(), "c", pv.ID, player, HelpInput{CharacterID: "mira", Version: pv.Version})
	if code(err) != 422 {
		t.Fatalf("helping your own project: %v; want 422", err)
	}
}
