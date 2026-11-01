package crafting

import (
	"context"
	"reflect"
	"testing"
)

func TestRoll_RememberedIngredients(t *testing.T) {
	moonlit := []Rule{{When: []Cond{{Kind: "ingredient", Mark: "Moonlit"}}, Then: Effect{Kind: "points", Amount: 3}, Players: "book"}}
	cases := []struct {
		name    string
		used    []Kept
		rules   []Rule
		quality *Quality
		total   int
	}{
		{"a moonlit ingredient went in", []Kept{{Qty: 1, Marks: []string{"moonlit"}}}, moonlit, nil, 12},
		{"no marked ingredient went in", nil, moonlit, nil, 9},
		{"fine ingredients add points", []Kept{{Qty: 1, Quality: "fine"}}, nil, &Quality{Fine: "points", Poor: "bane"}, 11},
		{"poor ingredients take points", []Kept{{Qty: 2, Quality: "poor"}}, nil, &Quality{Fine: "edge", Poor: "points"}, 7},
		{"no Quality setting, no change", []Kept{{Qty: 1, Quality: "fine"}}, nil, nil, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newProjectFx()
			r := fx.repo.recipes["heal"]
			r.Rules, r.Extras.Quality, r.Extras.Botch.Costs = tc.rules, tc.quality, BotchBane
			fx.repo.recipes["heal"] = r
			fx.arm.used = tc.used
			pv := fx.start(t)
			fx.ready(pv.ID, 1)
			fx.dice = []int{3, 4}
			res, err := fx.roll(t, pv.ID, WorkSteady)
			if err != nil {
				t.Fatal(err)
			}
			if res.Total != tc.total {
				t.Errorf("total = %d, want %d; terms %q", res.Total, tc.total, termTexts(res))
			}
		})
	}

}

func TestStart_PicksReachTheArmory(t *testing.T) {
	fx := newProjectFx()
	_, err := fx.s.StartProject(context.Background(), "c", "mira", player, StartInput{RecipeID: "heal", Picks: map[int]string{1: "moss"},
		Use: map[int][]KeptPick{0: {{ID: 7, N: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(fx.arm.spent) != 1 || !reflect.DeepEqual(fx.arm.spent[0][0].Use, []KeptPick{{ID: 7, N: 1}}) {
		t.Errorf("spent %+v", fx.arm.spent)
	}
}

func TestRoll_TheMadeItemRemembers(t *testing.T) {
	day := &Day{Label: "3 Frostmoot", Abs: 100, After: func(n int) (int64, string) { return int64(100 + n), "10 Frostmoot" }}
	fx := newProjectFx()
	fx.days.day = day
	r := fx.repo.recipes["heal"]
	r.Rules = []Rule{{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "fine"}, Players: "book"}}
	r.Extras = Extras{Botch: Botch{Costs: BotchBane}, MakersMark: true, Keeps: &Keeps{Days: 7, ThenBecomes: "dragon's tear"}}
	fx.repo.recipes["heal"] = r
	pv := fx.start(t)
	fx.ready(pv.ID, 1)
	p := fx.store.projects[pv.ID]
	p.Points = 20
	fx.store.projects[pv.ID] = p
	fx.dice = []int{10, 10}
	res, err := fx.roll(t, pv.ID, WorkSteady)
	if err != nil {
		t.Fatal(err)
	}
	want := Kept{Quality: "fine", How: "made", By: "Mira", On: "3 Frostmoot", OffDay: 107, OffOn: "10 Frostmoot", Becomes: "secret", BecomesName: "dragon's tear"}
	if len(fx.arm.made) != 1 || fx.arm.made[0].Look == nil {
		t.Fatalf("made %+v, want one item with its picture", fx.arm.made)
	}
	got := fx.arm.made[0]
	got.Look = nil
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("made %+v, want %+v", got, want)
	}
	if res.Item == nil || res.Item.Quality != "fine" || res.Item.By != "Mira" {
		t.Errorf("the result card's item %+v", res.Item)
	}

	t.Run("a plain recipe's item remembers only its picture", func(t *testing.T) {
		fx := newProjectFx()
		pv := fx.start(t)
		fx.ready(pv.ID, 1)
		p := fx.store.projects[pv.ID]
		p.Points = 20
		fx.store.projects[pv.ID] = p
		fx.dice = []int{10, 10}
		res, err := fx.roll(t, pv.ID, WorkSteady)
		if err != nil {
			t.Fatal(err)
		}
		if len(fx.arm.made) != 1 || fx.arm.made[0].Look == nil {
			t.Fatalf("made %+v", fx.arm.made)
		}
		k := fx.arm.made[0]
		k.Look = nil
		if !k.Empty() || res.Item == nil || res.Item.Look == nil {
			t.Errorf("made %+v, item %+v", fx.arm.made, res.Item)
		}
	})
}

func TestTakeSpoil_HarvestRemembersTheCut(t *testing.T) {
	for _, c := range []struct {
		name    string
		dice    []int
		quality string
	}{
		{"a poor cut", []int{4, 3}, "poor"},
		{"a plain cut", []int{6, 5}, ""},
		{"a fine cut", []int{10, 9}, "fine"},
	} {
		t.Run(c.name, func(t *testing.T) {
			fx := newSpoilFx()
			fx.fight(t)
			fx.dice = c.dice
			if _, err := fx.s.TakeSpoil(context.Background(), "c", fx.lineFor(t, "Troll", "water"), player, TakeInput{CharacterID: "mira"}); err != nil {
				t.Fatal(err)
			}
			if len(fx.arm.spoilKept) != 1 {
				t.Fatalf("given %+v", fx.arm.spoilKept)
			}
			k := fx.arm.spoilKept[0]
			if k.Quality != c.quality || (c.quality == "") != k.Empty() || (c.quality != "" && (k.How != "harvested" || k.By != "Mira")) {
				t.Errorf("kept %+v", k)
			}
		})
	}
}

func TestRespite_TurnsWhatWentOff(t *testing.T) {
	ctx := context.Background()
	fx := newProjectFx()
	if _, err := fx.s.LogRespite(ctx, "c", director); err != nil || len(fx.arm.offDays) != 0 {
		t.Fatalf("no calendar: %v, went off %v", err, fx.arm.offDays)
	}
	fx.days.day = &Day{Label: "3 Frostmoot", Abs: 100}
	if _, err := fx.s.LogRespite(ctx, "c", director); err != nil || !reflect.DeepEqual(fx.arm.offDays, []int64{100}) {
		t.Fatalf("respite: %v, went off %v", err, fx.arm.offDays)
	}
}
