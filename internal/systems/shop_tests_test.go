package systems

import (
	"reflect"
	"strings"
	"testing"
)

// The test a shop counter throws comes from shop_tests, else crafting, else a
// plain d20 against a DC.
func TestShopTestRules_Resolution(t *testing.T) {
	cr := &CraftingRules{Dice: "2d10", Edge: 2, Bane: -2, Tiers: []int{11, 16},
		Characteristics: []CraftingStat{{Key: "might", Label: "Might"}, {Key: "presence", Label: "Presence"}, {Key: "reason", Label: "Reason"}}}
	tests := []struct {
		name string
		m    *SystemManifest
		want ShopTestRulesView
	}{
		{"nil manifest", nil, DefaultShopTestRules()},
		{"no blocks", &SystemManifest{}, DefaultShopTestRules()},
		{"crafting only", &SystemManifest{Crafting: cr}, ShopTestRulesView{
			Dice: "2d10", Style: "tiers", Tiers: []int{11, 16}, Edge: 2, Bane: -2, Haggle: "presence", Look: "reason",
			Stats: []ShopTestStatDef{{Key: "might", Label: "Might"}, {Key: "presence", Label: "Presence"}, {Key: "reason", Label: "Reason"}},
			DC:    ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}},
		{"crafting without presence or reason falls to the first", &SystemManifest{Crafting: &CraftingRules{Characteristics: []CraftingStat{{Key: "grit", Label: "Grit"}}}},
			ShopTestRulesView{Dice: "2d10", Style: "tiers", Tiers: []int{11, 16}, Edge: 0, Bane: 0, Haggle: "grit", Look: "grit",
				Stats: []ShopTestStatDef{{Key: "grit", Label: "Grit"}}, DC: ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}},
		{"crafting with no stats adds nothing", &SystemManifest{Crafting: &CraftingRules{}},
			ShopTestRulesView{Dice: "2d10", Style: "tiers", Tiers: []int{11, 16}, Stats: []ShopTestStatDef{}, DC: ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}},
		{"the block wins over crafting", &SystemManifest{Crafting: cr, ShopTests: &ShopTestsDef{
			Dice: "1d20", Style: "dc", Stats: []ShopTestStatDef{{Key: "cha", Label: "Charisma", Score: true}, {Key: "int", Label: "Intelligence", Score: true}},
			Haggle: "cha", Look: "int", DC: &ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}},
			ShopTestRulesView{Dice: "1d20", Style: "dc", Haggle: "cha", Look: "int", Tiers: []int{11, 16}, Edge: 2, Bane: -2,
				Stats: []ShopTestStatDef{{Key: "cha", Label: "Charisma", Score: true}, {Key: "int", Label: "Intelligence", Score: true}},
				DC:    ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}},
		{"an empty block is the d20 dc test, with the usual edge and bane if it later says tiers", &SystemManifest{ShopTests: &ShopTestsDef{}},
			ShopTestRulesView{Dice: "1d20", Style: "dc", Stats: []ShopTestStatDef{}, Tiers: []int{11, 16}, Edge: 2, Bane: -2, DC: ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}},
		{"a written zero edge stays zero", &SystemManifest{ShopTests: &ShopTestsDef{Style: "tiers", Edge: intp(0), Bane: intp(-1)}},
			ShopTestRulesView{Dice: "1d20", Style: "tiers", Stats: []ShopTestStatDef{}, Tiers: []int{11, 16}, Edge: 0, Bane: -1, DC: ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}},
		{"half a dc fills the rest", &SystemManifest{ShopTests: &ShopTestsDef{DC: &ShopTestDCDef{Usual: 12}}},
			ShopTestRulesView{Dice: "1d20", Style: "dc", Stats: []ShopTestStatDef{}, Tiers: []int{11, 16}, Edge: 2, Bane: -2, DC: ShopTestDCDef{Easy: 10, Usual: 12, Hard: 20}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.ShopTestRules(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestValidateShopTests(t *testing.T) {
	stats := []ShopTestStatDef{{Key: "cha", Label: "Charisma", Score: true}}
	tests := []struct {
		name string
		in   *ShopTestsDef
		want string // part of the error, "" for none
	}{
		{"no block", nil, ""},
		{"the brief's example", &ShopTestsDef{Dice: "1d20", Style: "dc", Stats: stats, Haggle: "cha", Look: "cha", Tiers: []int{11, 16},
			Edge: intp(2), Bane: intp(-2), DC: &ShopTestDCDef{Easy: 10, Usual: 15, Hard: 20}}, ""},
		{"dice written wrong", &ShopTestsDef{Dice: "2d7"}, "dice"},
		{"unknown style", &ShopTestsDef{Style: "pass-fail"}, "style"},
		{"bad stat key", &ShopTestsDef{Stats: []ShopTestStatDef{{Key: "Cha", Label: "Charisma"}}}, "key"},
		{"stat twice", &ShopTestsDef{Stats: append(append([]ShopTestStatDef{}, stats...), stats...)}, "twice"},
		{"markup in a label", &ShopTestsDef{Stats: []ShopTestStatDef{{Key: "cha", Label: "<b>Cha</b>"}}}, "label"},
		{"no label", &ShopTestsDef{Stats: []ShopTestStatDef{{Key: "cha"}}}, "label"},
		{"too many stats", &ShopTestsDef{Stats: manyShopStats(13)}, "too many"},
		{"haggle names an unlisted stat", &ShopTestsDef{Stats: stats, Haggle: "wis"}, "haggle"},
		{"look names a stat with none listed", &ShopTestsDef{Look: "int"}, "look"},
		{"tiers falling", &ShopTestsDef{Tiers: []int{16, 11}}, "tiers"},
		{"one tier", &ShopTestsDef{Tiers: []int{11}}, "tiers"},
		{"edge too big", &ShopTestsDef{Edge: intp(11)}, "edge"},
		{"bane too small", &ShopTestsDef{Bane: intp(-11)}, "bane"},
		{"dc too high", &ShopTestsDef{DC: &ShopTestDCDef{Hard: 41}}, "dc hard"},
		{"dc negative", &ShopTestsDef{DC: &ShopTestDCDef{Easy: -1}}, "dc easy"},
		{"dc falling", &ShopTestsDef{DC: &ShopTestDCDef{Easy: 18, Usual: 15, Hard: 20}}, "dc must rise"},
		{"half a dc that crosses the default", &ShopTestsDef{DC: &ShopTestDCDef{Easy: 16}}, "dc must rise"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShopTests(tt.in)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)):
				t.Errorf("error %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func manyShopStats(n int) []ShopTestStatDef {
	out := make([]ShopTestStatDef, n)
	for i := range out {
		out[i] = ShopTestStatDef{Key: "s" + strings.Repeat("a", i), Label: "Stat"}
	}
	return out
}
