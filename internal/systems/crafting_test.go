package systems

import (
	"reflect"
	"strings"
	"testing"
)

// A system's crafting block is checked when the package installs, so a
// block the host could not roll with never reaches a recipe editor.
func TestValidateCrafting(t *testing.T) {
	stat := func(key, label string) CraftingStat { return CraftingStat{Key: key, Label: label} }
	tests := []struct {
		name string
		in   *CraftingRules
		want string // part of the error, "" for none
	}{
		{"no block", nil, ""},
		{"the default", ptr(DefaultCraftingRules()), ""},
		{"a full block", &CraftingRules{Dice: "2d10", Edge: 2, Bane: -2, Tiers: []int{11, 16},
			Characteristics: []CraftingStat{stat("might", "Might"), stat("reason", "Reason")},
			Labels:          CraftingLabels{Goal: "Project goal", Prerequisite: "Item prerequisite", Source: "Project source"}}, ""},
		{"curly apostrophe in a label", &CraftingRules{Labels: CraftingLabels{Goal: "Maker’s goal"}}, ""},
		{"dice written wrong", &CraftingRules{Dice: "2d7"}, "dice"},
		{"too many dice", &CraftingRules{Dice: "5d10"}, "dice"},
		{"dice with a bonus", &CraftingRules{Dice: "2d10+2"}, "dice"},
		{"edge too big", &CraftingRules{Edge: 11}, "edge and bane"},
		{"bane too small", &CraftingRules{Bane: -11}, "edge and bane"},
		{"bad key", &CraftingRules{Characteristics: []CraftingStat{stat("Might", "Might")}}, "key"},
		{"key twice", &CraftingRules{Characteristics: []CraftingStat{stat("might", "Might"), stat("might", "Strength")}}, "twice"},
		{"no label", &CraftingRules{Characteristics: []CraftingStat{stat("might", "")}}, "label"},
		{"markup in a label", &CraftingRules{Characteristics: []CraftingStat{stat("might", "<b>Might</b>")}}, "label"},
		{"too many characteristics", &CraftingRules{Characteristics: manyStats(13)}, "too many"},
		{"one tier", &CraftingRules{Tiers: []int{11}}, "tiers"},
		{"tiers falling", &CraftingRules{Tiers: []int{16, 11}}, "tiers"},
		{"tier below one", &CraftingRules{Tiers: []int{0, 16}}, "tiers"},
		{"label with spaces round it", &CraftingRules{Labels: CraftingLabels{Source: " Source"}}, "labels"},
		{"label too long", &CraftingRules{Labels: CraftingLabels{Goal: strings.Repeat("a", 41)}}, "labels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCrafting(tt.in)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.want != "" && err == nil:
				t.Fatalf("no error, want one about %q", tt.want)
			case tt.want != "" && !strings.Contains(err.Error(), tt.want):
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}
}

// Callers read the rules without checking for gaps: every empty part comes
// from the defaults, and what the system did say is kept.
func TestCraftingRulesFillsGaps(t *testing.T) {
	var none *SystemManifest
	if got := none.CraftingRules(); !reflect.DeepEqual(got, DefaultCraftingRules()) {
		t.Errorf("nil manifest = %+v, want the defaults", got)
	}
	if got := (&SystemManifest{}).CraftingRules(); !reflect.DeepEqual(got, DefaultCraftingRules()) {
		t.Errorf("no block = %+v, want the defaults", got)
	}
	m := &SystemManifest{Crafting: &CraftingRules{Dice: "3d6", Edge: 1, Bane: -1,
		Characteristics: []CraftingStat{{Key: "int", Label: "Intelligence"}}, Labels: CraftingLabels{Goal: "Progress"}}}
	got := m.CraftingRules()
	want := CraftingRules{Dice: "3d6", Edge: 1, Bane: -1, Tiers: []int{11, 16},
		Characteristics: []CraftingStat{{Key: "int", Label: "Intelligence"}},
		Labels:          CraftingLabels{Goal: "Progress", Prerequisite: "Item prerequisite", Source: "Project source"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	got.Characteristics[0].Label = "changed"
	if m.Crafting.Characteristics[0].Label != "Intelligence" {
		t.Error("changing the returned rules changed the manifest")
	}
}

func TestParseDice(t *testing.T) {
	tests := []struct {
		in          string
		count, size int
		ok          bool
	}{
		{"2d10", 2, 10, true},
		{"1d20", 1, 20, true},
		{"4d6", 4, 6, true},
		{"d20", 0, 0, false},
		{"2D10", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, tt := range tests {
		c, s, ok := ParseDice(tt.in)
		if c != tt.count || s != tt.size || ok != tt.ok {
			t.Errorf("ParseDice(%q) = %d, %d, %v; want %d, %d, %v", tt.in, c, s, ok, tt.count, tt.size, tt.ok)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func manyStats(n int) []CraftingStat {
	out := make([]CraftingStat, n)
	for i := range out {
		out[i] = CraftingStat{Key: "s" + strings.Repeat("x", i), Label: "Stat"}
	}
	return out
}
