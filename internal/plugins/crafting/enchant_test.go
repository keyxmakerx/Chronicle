package crafting

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// newEnchantFx is the potion fixture with a recipe that enchants the potion
// Mira carries with Frostbite, using one Bloodmoss; an item holds two.
func newEnchantFx(name string) *projectFx {
	fx := newProjectFx()
	x := DefaultExtras()
	x.Enchant = &Enchant{Line: 0, Name: name, Slots: 2}
	fx.repo.recipes["frost"] = Recipe{ID: "frost", Name: "Frostbite", Who: "all", Goal: 10, Stat: "reason",
		Materials: []Material{{ItemID: "potion", Qty: 3}, {ItemID: "moss", Qty: 1}}, Extras: x, Version: 1}
	fx.repo.order = append(fx.repo.order, "frost")
	fx.arm.have["potion"] = 1
	return fx
}

func (fx *projectFx) startEnchant(use map[int][]KeptPick) (*ProjectView, error) {
	return fx.s.StartProject(context.Background(), "c", "mira", player, StartInput{RecipeID: "frost", Use: use})
}

// finish rolls the project to its goal.
func (fx *projectFx) finish(t *testing.T, id string) *RollResult {
	t.Helper()
	p := fx.store.projects[id]
	p.Points = p.Goal - 1
	fx.store.projects[id] = p
	fx.ready(id, 1)
	fx.dice = []int{5, 5}
	res, err := fx.roll(t, id, WorkSteady)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Finished {
		t.Fatalf("roll didn't finish the project: %+v", res)
	}
	return res
}

// The item that went in comes back as itself, with what it remembered and
// the enchantment, once, however the recipe counts the line.
func TestEnchant_GivesBackTheSameItem(t *testing.T) {
	fx := newEnchantFx("Frostbite")
	orrin := Kept{ID: 7, Qty: 1, Marks: []string{"moonlit"}, Enchants: []string{"Glow"}, How: "made", By: "Orrin", On: "3 Frostmoot"}
	fx.arm.kept = map[string][]Kept{"potion": {orrin}}
	if _, err := fx.startEnchant(nil); code(err) != 422 || !strings.Contains(err.Error(), "pick which") {
		t.Fatalf("every copy remembers something and none was picked: %v, want a request to pick", err)
	}
	pv, err := fx.startEnchant(map[int][]KeptPick{0: {{ID: 7, N: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := fx.arm.spent[0][0]; got.Qty != 1 || !reflect.DeepEqual(got.Use, []KeptPick{{ID: 7, N: 1}}) {
		t.Fatalf("took %+v, want exactly the one picked copy", got)
	}
	if pv.Makes != "1 × Healing potion" {
		t.Fatalf("makes = %q", pv.Makes)
	}
	res := fx.finish(t, pv.ID)
	if fx.arm.given["potion"] != 1 || len(fx.arm.made) != 1 {
		t.Fatalf("given = %v, made %d", fx.arm.given, len(fx.arm.made))
	}
	want := Kept{Marks: []string{"moonlit"}, Enchants: []string{"Glow", "Frostbite"}, How: "made", By: "Orrin", On: "3 Frostmoot"}
	if got := fx.arm.made[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("came back as %+v, want %+v", got, want)
	}
	if len(fx.arm.gaveFor) != 1 || fx.arm.gaveFor[0] != "mira>mira: Enchanted 1 × Healing potion with Frostbite" {
		t.Fatalf("history = %q", fx.arm.gaveFor)
	}
	if res.Item == nil || !reflect.DeepEqual(res.Item.Enchants, want.Enchants) {
		t.Fatalf("the result shows %+v", res.Item)
	}
}

func TestEnchant_Refusals(t *testing.T) {
	tests := []struct {
		name, enchant string
		kept          []Kept
		use           map[int][]KeptPick
		want          string
	}{
		{name: "no name yet", enchant: "", want: "doesn't name its enchantment"},
		{name: "already full", enchant: "Frostbite", kept: []Kept{{ID: 7, Qty: 1, Enchants: []string{"Glow", "Ward"}}},
			use: map[int][]KeptPick{0: {{ID: 7, N: 1}}}, want: "already holds 2 enchantments"},
		{name: "already has it", enchant: "Frostbite", kept: []Kept{{ID: 7, Qty: 1, Enchants: []string{"frostbite"}}},
			use: map[int][]KeptPick{0: {{ID: 7, N: 1}}}, want: "already enchanted with Frostbite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fx := newEnchantFx(tt.enchant)
			fx.arm.kept = map[string][]Kept{"potion": tt.kept}
			_, err := fx.startEnchant(tt.use)
			if code(err) != 422 || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want 422 saying %q", err, tt.want)
			}
			if len(fx.arm.spent) != 0 {
				t.Fatal("something was taken from the pack")
			}
		})
	}
}

// Cancelling gives the hero's own item back unchanged; the other materials
// stay spent, as for any project.
func TestEnchant_CancelGivesItBack(t *testing.T) {
	fx := newEnchantFx("Frostbite")
	pv, err := fx.startEnchant(nil)
	if err != nil {
		t.Fatal(err)
	}
	if fx.arm.have["potion"] != 0 || fx.arm.have["moss"] != 3 {
		t.Fatalf("after start: %v", fx.arm.have)
	}
	if err := fx.s.CancelProject(context.Background(), "c", pv.ID, player); err != nil {
		t.Fatal(err)
	}
	if fx.arm.given["potion"] != 1 || len(fx.arm.made) != 1 || !fx.arm.made[0].Empty() || fx.arm.given["moss"] != 0 {
		t.Fatalf("gave back %v, %+v", fx.arm.given, fx.arm.made)
	}
	if fx.arm.gaveFor[0] != "mira>mira: Given back: Frostbite was cancelled" {
		t.Fatalf("history = %q", fx.arm.gaveFor)
	}
	if err := fx.s.CancelProject(context.Background(), "c", pv.ID, player); code(err) != 409 || fx.arm.given["potion"] != 1 {
		t.Fatalf("cancelling twice: %v, gave %d", err, fx.arm.given["potion"])
	}
}

// The enchanted line follows its material when the list changes, and goes
// when the material does.
func TestFollowMaterials_Enchant(t *testing.T) {
	before := []Material{{ItemID: "potion", Qty: 1}, {ItemID: "moss", Qty: 1}}
	x := DefaultExtras()
	x.Enchant = &Enchant{Line: 0, Name: "Frostbite", Slots: 2}
	moved := FollowMaterials(x, before, []Material{{ItemID: "moss", Qty: 1}, {ItemID: "potion", Qty: 1}})
	if moved.Enchant == nil || moved.Enchant.Line != 1 || x.Enchant.Line != 0 {
		t.Fatalf("moved = %+v, original %+v", moved.Enchant, x.Enchant)
	}
	if gone := FollowMaterials(x, before, []Material{{ItemID: "moss", Qty: 1}}); gone.Enchant != nil {
		t.Fatalf("enchant kept on a removed material: %+v", gone.Enchant)
	}
}

func TestBook_EnchantingRecipe(t *testing.T) {
	r := &Recipe{Materials: []Material{{ItemID: "potion", Qty: 3}}, Extras: Extras{Enchant: &Enchant{Line: 0, Name: "Frostbite", Slots: 1}}}
	p := bookPage(r, map[string]string{"potion": "Healing potion"}, bookRules())
	if p.Makes != "Healing potion, enchanted with Frostbite" || p.MakesQty != 1 || p.Needs[0].Qty != 1 {
		t.Fatalf("page = %+v", p)
	}
	if len(p.Lines) != 1 || !strings.Contains(p.Lines[0], "at most 1 enchantment,") {
		t.Fatalf("lines = %q", p.Lines)
	}
}
