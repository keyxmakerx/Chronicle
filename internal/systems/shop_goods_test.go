package systems

import (
	"errors"
	"strings"
	"testing"
)

type goodsProvider struct {
	DataProvider
	byCat map[string][]ReferenceItem
}

func (p goodsProvider) List(c string) ([]ReferenceItem, error) {
	items, ok := p.byCat[c]
	if !ok {
		return nil, errors.New("no such category")
	}
	return items, nil
}

type goodsSystem struct {
	System
	info *SystemManifest
	dp   DataProvider
}

func (s goodsSystem) Info() *SystemManifest     { return s.info }
func (s goodsSystem) DataProvider() DataProvider { return s.dp }

func TestValidateGoods(t *testing.T) {
	cats := []CategoryDef{{Slug: "treasures", Name: "Treasures"}, {Slug: "gear", Name: "Gear"}}
	tests := []struct {
		name    string
		goods   []GoodsDef
		wantErr string
	}{
		{"none", nil, ""},
		{"ok", []GoodsDef{{Category: "treasures", KindKey: "kind", Kinds: []string{"Consumable"}, RarityKey: "echelon", PriceKey: "price"}}, ""},
		{"unknown category", []GoodsDef{{Category: "nope"}}, "not one of"},
		{"listed twice", []GoodsDef{{Category: "gear"}, {Category: "gear"}}, "twice"},
		{"bad key", []GoodsDef{{Category: "gear", PriceKey: "Price!"}}, "price_key"},
		{"kinds without key", []GoodsDef{{Category: "gear", Kinds: []string{"a"}}}, "kind_key"},
		{"long label", []GoodsDef{{Category: "gear", RarityLabel: strings.Repeat("x", 25)}}, "rarity_label"},
		{"too many", func() []GoodsDef {
			out := make([]GoodsDef, maxGoodsDefs+1)
			for i := range out {
				out[i] = GoodsDef{Category: "gear"}
			}
			return out
		}(), "too many"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGoods(&SystemManifest{Categories: cats, Goods: tt.goods})
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestGoodsOf(t *testing.T) {
	dp := goodsProvider{byCat: map[string][]ReferenceItem{
		"treasures": {
			{ID: "a", Name: "Potion", Properties: map[string]any{"kind": "Consumable", "echelon": "2nd", "price": 50.0}},
			{ID: "b", Name: "Statue", Properties: map[string]any{"kind": "Leveled", "price": 10.0}},
			{ID: "c", Name: "Charm", Properties: map[string]any{"kind": "consumable", "echelon": "1st", "price": "free"}},
			{ID: "d", Name: "Dust", Properties: map[string]any{"kind": "Consumable", "price": 0.0}},
		},
	}}
	info := &SystemManifest{Goods: []GoodsDef{
		{Category: "treasures", KindKey: "kind", Kinds: []string{"Consumable"}, RarityKey: "echelon", RarityLabel: "echelon", PriceKey: "price"},
		{Category: "missing"},
	}}
	got, label := GoodsOf(goodsSystem{info: info, dp: dp})
	if label != "echelon" {
		t.Errorf("label = %q", label)
	}
	if len(got) != 3 {
		t.Fatalf("got %d goods, want 3 (Statue is the wrong kind): %+v", len(got), got)
	}
	by := map[string]Good{}
	for _, g := range got {
		by[g.Slug] = g
	}
	if g := by["a"]; g.Rarity != 2 || !g.HasPrice || g.Price != 50 {
		t.Errorf("a = %+v", g)
	}
	if g := by["c"]; g.Rarity != 1 || g.HasPrice {
		t.Errorf("c = %+v: text price must read as no price", g)
	}
	if g := by["d"]; g.HasPrice || g.Rarity != 0 {
		t.Errorf("d = %+v: zero is no price, no rarity is 0", g)
	}

	if g, _ := GoodsOf(nil); g != nil {
		t.Error("nil system gave goods")
	}
	if g, _ := GoodsOf(goodsSystem{info: info}); g != nil {
		t.Error("system with no data gave goods")
	}
	noPrice := &SystemManifest{Goods: []GoodsDef{{Category: "treasures"}}}
	g2, _ := GoodsOf(goodsSystem{info: noPrice, dp: dp})
	for _, g := range g2 {
		if g.HasPrice {
			t.Errorf("%s has a price although the manifest names no price_key", g.Slug)
		}
	}
}

func TestOrdinalOf(t *testing.T) {
	for in, want := range map[string]int{"": 0, "2nd": 2, "Tier 4": 4, "x": 0, "12": 12, "99999": 1000} {
		if got := ordinalOf(in); got != want {
			t.Errorf("ordinalOf(%q) = %d, want %d", in, got, want)
		}
	}
}
