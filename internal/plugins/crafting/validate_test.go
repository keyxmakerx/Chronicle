package crafting

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

func idx(n int) *int { return &n }

func baseFacts() recipeFacts {
	return recipeFacts{
		Items:          map[string]bool{"moss": true, "water": true, "potion": true},
		Characters:     map[string]bool{"mira": true},
		KeptItems:      map[string]bool{"gone": true},
		KeptCharacters: map[string]bool{"tobin": true},
		Rules:          systems.DefaultCraftingRules(),
	}
}

func TestCleanRecipeRefuses(t *testing.T) {
	ok := RecipeInput{Name: "Healing potion", Goal: 30}
	tests := []struct {
		name string
		edit func(*RecipeInput)
		want string
	}{
		{"empty name", func(in *RecipeInput) { in.Name = "   " }, "name is empty"},
		{"long name", func(in *RecipeInput) { in.Name = strings.Repeat("a", 121) }, "longer than 120"},
		{"control character", func(in *RecipeInput) { in.Name = "Bad\x00name" }, "cannot be shown"},
		{"unknown template", func(in *RecipeInput) { in.Template = "forge" }, "no template"},
		{"unknown bench", func(in *RecipeInput) { in.Bench = "kitchen" }, "no bench kind"},
		{"item and tag at once", func(in *RecipeInput) { in.Materials = []Material{{ItemID: "moss", Tag: "herb"}} }, "not both"},
		{"neither item nor tag", func(in *RecipeInput) { in.Materials = []Material{{Qty: 1}} }, "needs an item or a tag"},
		{"item from elsewhere", func(in *RecipeInput) { in.Materials = []Material{{ItemID: "stranger"}} }, "not an Armory item"},
		{"a character is no material", func(in *RecipeInput) { in.Materials = []Material{{ItemID: "mira"}} }, "not an Armory item"},
		{"a kept character is no material", func(in *RecipeInput) { in.Materials = []Material{{ItemID: "tobin"}} }, "not an Armory item"},
		{"unknown tag", func(in *RecipeInput) { in.Materials = []Material{{Tag: "gem"}} }, "no crafting tag"},
		{"too many materials", func(in *RecipeInput) { in.Materials = make([]Material, 13) }, "at most 12 materials"},
		{"unknown step", func(in *RecipeInput) { in.Steps = []Step{{T: "hammer"}} }, "no step \"hammer\""},
		{"too many steps", func(in *RecipeInput) { in.Steps = make([]Step, 25) }, "at most 24 steps"},
		{"result from elsewhere", func(in *RecipeInput) { in.Result.ItemID = "stranger" }, "item it makes"},
		{"stat with symbols", func(in *RecipeInput) { in.Stat = "<b>" }, "40 plain characters"},
		{"long prerequisite", func(in *RecipeInput) { in.Prerequisite = strings.Repeat("p", 201) }, "item prerequisite is longer"},
		{"negative cost", func(in *RecipeInput) { in.Cost = Cost{Amount: -1} }, "zero or more"},
		{"huge cost", func(in *RecipeInput) { in.Cost = Cost{Amount: 2e9} }, "zero or more"},
		{"odd coin", func(in *RecipeInput) { in.Cost = Cost{Amount: 5, Currency: "g$"} }, "letters, like gp"},
		{"long coin", func(in *RecipeInput) { in.Cost = Cost{Amount: 5, Currency: "goldpiecesxyz"} }, "letters, like gp"},
		{"unknown who", func(in *RecipeInput) { in.Who = "some" }, `"all" or "learned"`},
		{"known by a stranger", func(in *RecipeInput) { in.Known = []string{"stranger"} }, "not a character"},
		{"known by an item", func(in *RecipeInput) { in.Known = []string{"moss"} }, "not a character"},
		{"known by a kept item", func(in *RecipeInput) { in.Known = []string{"gone"} }, "not a character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ok
			tt.edit(&in)
			_, err := cleanRecipe(in, baseFacts())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to say %q", err, tt.want)
			}
		})
	}
}

func TestCleanRecipeKeeps(t *testing.T) {
	in := RecipeInput{
		Name: "  Healing potion  ", Template: "steady",
		Materials: []Material{{ItemID: "moss", Qty: 0}, {Tag: "liquid", Qty: 500}, {ItemID: "gone", Qty: 2}},
		Steps: []Step{
			{T: "add", Material: idx(0), To: "bucket"},
			{T: "grind", Turns: 40},
			{T: "tip"},
			{T: "pour", Material: idx(7)},
			{T: "simmer"},
			{T: "strike", N: 3, Material: idx(1)},
		},
		Result: Result{ItemID: "potion", Qty: 0}, Goal: 5000, Stat: " Reason ",
		Cost: Cost{Amount: 12.345, Currency: " gp "},
		Who:  "learned", Known: []string{"mira", "mira", "tobin"},
		Colors: Colors{Base: "#ABCDEF", Paste: "red", Mid: "#12345", Final: "#ff4d6d"},
	}
	r, err := cleanRecipe(in, baseFacts())
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"name trimmed", r.Name, "Healing potion"},
		{"bench defaults", r.Bench, "apothecary"},
		{"qty raised to 1", r.Materials[0].Qty, 1},
		{"qty cut to 99", r.Materials[1].Qty, 99},
		{"a deleted item already in the recipe still saves", r.Materials[2].ItemID, "gone"},
		{"into falls back to the first place", r.Steps[0].To, "mortar"},
		{"grind clamped", r.Steps[1].Turns, 8},
		{"a missing material index is dropped", r.Steps[3].Material == nil, true},
		{"simmer takes its default", r.Steps[4].Secs, 5},
		{"a step that takes no material keeps none", r.Steps[5].Material == nil, true},
		{"strike kept", r.Steps[5].N, 3},
		{"result qty raised", r.Result.Qty, 1},
		{"goal clamped", r.Goal, 999},
		{"stat trimmed", r.Stat, "Reason"},
		{"cost rounded to cents", r.Cost.Amount, 12.35},
		{"coin trimmed", r.Cost.Currency, "gp"},
		{"known once each", strings.Join(r.Known, ","), "mira,tobin"},
		{"colour lowercased", r.Colors.Base, "#abcdef"},
		{"bad colours dropped", r.Colors.Paste + r.Colors.Mid, ""},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestCleanStat(t *testing.T) {
	ds := systems.DefaultCraftingRules()
	ds.Characteristics = []systems.CraftingStat{{Key: "reason", Label: "Reason"}, {Key: "might", Label: "Might"}}
	tests := []struct {
		name, in, want string
		rules          systems.CraftingRules
		err            bool
	}{
		{"none is fine", "", "", ds, false},
		{"by key", "might", "might", ds, false},
		{"by label, any case", "REASON", "reason", ds, false},
		{"not the system's", "Charisma", "", ds, true},
		{"free label without a system", "Wisdom (Herbalism)", "Wisdom (Herbalism)", systems.DefaultCraftingRules(), false},
		{"no markup", "<b>Wis</b>", "", systems.DefaultCraftingRules(), true},
		{"plain label without a system", "Sleight of hand", "Sleight of hand", systems.DefaultCraftingRules(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanStat(tt.in, tt.rules)
			if (err != nil) != tt.err || got != tt.want {
				t.Fatalf("cleanStat(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}

func TestCleanTagsAndSettings(t *testing.T) {
	got, err := cleanTags([]string{"herb", "metal", "herb"})
	if err != nil || strings.Join(got, ",") != "metal,herb" {
		t.Fatalf("cleanTags = %v, %v; want the library's order, once each", got, err)
	}
	if _, err := cleanTags([]string{"gem"}); err == nil {
		t.Fatal("an unknown tag was accepted")
	}
	if got, err := cleanTags(nil); err != nil || len(got) != 0 {
		t.Fatalf("no tags = %v, %v", got, err)
	}
	if _, err := cleanSettings(DefaultSettings()); err != nil {
		t.Fatal(err)
	}
	for _, s := range []Settings{{Skin: "forge", Difficulty: "std"}, {Skin: "desk", Difficulty: "brutal"}} {
		if _, err := cleanSettings(s); err == nil {
			t.Errorf("settings %+v were accepted", s)
		}
	}
}

func TestCleanArrangement(t *testing.T) {
	if _, err := cleanArrangement(json.RawMessage(`[1,2]`)); err == nil {
		t.Fatal("an unreadable arrangement was accepted")
	}
	raw := `{"look":{"wall":"stone","ldx":-12.6,"kit":["skull","ball",3],"Bad-Key":"x","fn":{"a":1},"night":true},
	  "pieces":{"cat":{"name":"  Pepper ","text":"` + strings.Repeat("t", 250) + `","dx":5000,"dy":-9000,"hidden":true,
	    "acts":[{"label":"Pet","fx":"b:pet","sound":"purr","say":"Purr"},{"label":"Burn","fx":"javascript:alert(1)","sound":"<script>"},"junk"]},
	  "../etc":{"name":"x"},"lamp":"not an object"}}`
	out, err := cleanArrangement(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	var a arrangement
	if err := json.Unmarshal(out, &a); err != nil {
		t.Fatal(err)
	}
	if a.Look["wall"] != "stone" || a.Look["ldx"] != float64(-13) || a.Look["night"] != true {
		t.Errorf("look = %v", a.Look)
	}
	if kit, _ := a.Look["kit"].([]any); len(kit) != 2 {
		t.Errorf("kit = %v, want only the strings", a.Look["kit"])
	}
	if _, ok := a.Look["Bad-Key"]; ok {
		t.Error("a key outside the pattern was kept")
	}
	if _, ok := a.Look["fn"]; ok {
		t.Error("an object setting was kept")
	}
	if len(a.Pieces) != 1 {
		t.Fatalf("pieces = %v, want only the cat", a.Pieces)
	}
	cat := a.Pieces["cat"]
	if cat.Name != "Pepper" || len([]rune(cat.Text)) != maxPieceText || cat.DX != 900 || cat.DY != -500 || !cat.Hidden {
		t.Errorf("cat = %+v", cat)
	}
	if len(cat.Acts) != 2 || cat.Acts[0].FX != "b:pet" || cat.Acts[0].Sound != "purr" || cat.Acts[1].FX != "" || cat.Acts[1].Sound != "" {
		t.Errorf("acts = %+v, want the made-up effect and sound emptied", cat.Acts)
	}
}
