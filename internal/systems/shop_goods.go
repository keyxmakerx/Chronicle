package systems

import (
	"fmt"
	"strconv"
	"strings"
)

// maxGoodsDefs bounds how many categories a package may name as goods.
const maxGoodsDefs = 8

// GoodsDef is one category of a package that a shop may stock, named in the
// manifest's optional "goods" list. Chronicle never guesses what is for
// sale: a package says which category holds goods and which of its fields
// say what kind of good it is, how rare, and what it costs.
type GoodsDef struct {
	// Category is a category slug of this manifest.
	Category string `json:"category"`
	// KindKey is the property that says what kind of good an entry is
	// ("Consumable", "Trinket"); Kinds keeps only those kinds. Both empty
	// takes every entry.
	KindKey string   `json:"kind_key,omitempty"`
	Kinds   []string `json:"kinds,omitempty"`
	// RarityKey is the property holding the entry's rarity, an ordinal such
	// as "2nd" (an echelon) or a number. Entries without one count as the
	// commonest.
	RarityKey string `json:"rarity_key,omitempty"`
	// RarityLabel is the word for that scale in the draft ("echelon"), so
	// the picker can say "1st echelon". Empty reads "rarity".
	RarityLabel string `json:"rarity_label,omitempty"`
	// PriceKey is the property holding a price in the campaign's main coin.
	// Absent, or an entry with no number there, means the entry has no
	// price and the Director sets one: Chronicle never invents a number.
	PriceKey string `json:"price_key,omitempty"`
}

// validateGoods rejects a goods list the host could not read.
func validateGoods(m *SystemManifest) error {
	if len(m.Goods) > maxGoodsDefs {
		return fmt.Errorf("too many goods categories (%d, max %d)", len(m.Goods), maxGoodsDefs)
	}
	cats := map[string]bool{}
	for _, c := range m.Categories {
		cats[c.Slug] = true
	}
	seen := map[string]bool{}
	for i, g := range m.Goods {
		if !cats[g.Category] {
			return fmt.Errorf("goods %d: category %q is not one of this system's categories", i, g.Category)
		}
		if seen[g.Category] {
			return fmt.Errorf("goods %d: category %q is listed twice", i, g.Category)
		}
		seen[g.Category] = true
		for name, k := range map[string]string{"kind_key": g.KindKey, "rarity_key": g.RarityKey, "price_key": g.PriceKey} {
			if k != "" && !slugPattern.MatchString(k) {
				return fmt.Errorf("goods %d: %s %q must contain only lowercase letters, numbers, hyphens, and underscores", i, name, k)
			}
		}
		if len(g.Kinds) > 0 && g.KindKey == "" {
			return fmt.Errorf("goods %d: kinds needs a kind_key", i)
		}
		if len(g.RarityLabel) > 24 {
			return fmt.Errorf("goods %d: rarity_label is longer than 24 characters", i)
		}
	}
	return nil
}

// Good is one stockable entry of a package, read for a shop's fill.
type Good struct {
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind,omitempty"`
	Category string  `json:"category"`
	Rarity   int     `json:"rarity"`
	Price    float64 `json:"price"`
	HasPrice bool    `json:"hasPrice"`
	Summary  string  `json:"summary,omitempty"`
}

// maxGoodsRead bounds one package read; a bigger list is cut, never an error.
const maxGoodsRead = 2000

// GoodsOf reads the stockable entries a system's manifest names. A category
// that cannot be read is skipped, so one bad file never empties the fill.
func GoodsOf(sys System) (goods []Good, rarityLabel string) {
	if sys == nil || sys.DataProvider() == nil || sys.Info() == nil {
		return nil, ""
	}
	for _, def := range sys.Info().Goods {
		items, err := sys.DataProvider().List(def.Category)
		if err != nil {
			continue
		}
		if rarityLabel == "" {
			rarityLabel = def.RarityLabel
		}
		kinds := map[string]bool{}
		for _, k := range def.Kinds {
			kinds[strings.ToLower(k)] = true
		}
		for _, it := range items {
			kind := goodProp(it.Properties, def.KindKey)
			if len(kinds) > 0 && !kinds[strings.ToLower(kind)] {
				continue
			}
			g := Good{Slug: it.ID, Name: it.Name, Kind: kind, Category: def.Category, Summary: it.Summary}
			g.Rarity = ordinalOf(goodProp(it.Properties, def.RarityKey))
			if def.PriceKey != "" {
				g.Price, g.HasPrice = goodNumber(it.Properties, def.PriceKey)
			}
			goods = append(goods, g)
			if len(goods) >= maxGoodsRead {
				return goods, rarityLabel
			}
		}
	}
	return goods, rarityLabel
}

func goodProp(props map[string]any, key string) string {
	if key == "" {
		return ""
	}
	switch v := props[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// goodNumber reads a positive number from a property; zero, negative or
// non-numeric text is "no price".
func goodNumber(props map[string]any, key string) (float64, bool) {
	var n float64
	switch v := props[key].(type) {
	case float64:
		n = v
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, false
		}
		n = f
	default:
		return 0, false
	}
	if n <= 0 || n != n {
		return 0, false
	}
	return n, true
}

// ordinalOf reads the leading number of "2nd", "3", "Tier 4"; 0 when none.
func ordinalOf(s string) int {
	n := 0
	seen := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
			seen = true
			if n > 1000 {
				return 1000
			}
		} else if seen {
			break
		}
	}
	return n
}
