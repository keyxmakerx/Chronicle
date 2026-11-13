package madeitems

import (
	"os"
	"regexp"
	"testing"
)

// The script that draws the pictures lists the same items, in the same order,
// with the same colours, or a stored look would name a picture it cannot draw.
func TestKeysMatchTheScript(t *testing.T) {
	src, err := os.ReadFile("../../static/js/widgets/made_items.js")
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile(`(?m)^\s+(\w+): \["[^"]*", '(#[0-9a-f]{6})', '(#[0-9a-f]{6})', '(#[0-9a-f]{6})', \d+, \d+, \d+, [01]\]`)
	got := row.FindAllStringSubmatch(string(src), -1)
	if len(got) != len(Items) {
		t.Fatalf("made_items.js lists %d items, Items %d", len(got), len(Items))
	}
	for i, m := range got {
		it := Items[i]
		if m[1] != it.Key || m[2] != it.Body || m[3] != it.Trim || m[4] != it.Detail {
			t.Errorf("item %d: script %v, Go %+v", i, m[1:], it)
		}
	}
}

// Every painted item can be reached from a name, and every word list names
// an item that exists.
func TestEveryItemHasWords(t *testing.T) {
	listed := map[string]bool{}
	for _, w := range words {
		if !Has(w.key) {
			t.Errorf("words name %q, which is not an item", w.key)
		}
		if listed[w.key] {
			t.Errorf("%q has two word lists", w.key)
		}
		listed[w.key] = true
	}
	for _, it := range Items {
		if !listed[it.Key] {
			t.Errorf("%q has no words, so no made item is ever drawn as it", it.Key)
		}
	}
}

func TestFromName(t *testing.T) {
	// The healing potion is one of the eight waiting items; without it a
	// potion is a bottle.
	potion := "bottle"
	if Has("potion") {
		potion = "potion"
	}
	for _, c := range []struct{ name, want string }{
		{"Potion of Giant Strength", potion},
		{"Vial of venom", "bottle"},
		{"Dragon scale armour", "breastplate"},
		{"Dragonscale", "scale"},
		{"Bandage", ""},
		{"Hand crossbow", "crossbow"},
		{"Lockpicks", "pickaxe"},
		{"Vial of bubbling ink", "ink"},
		{"Bubbling ink", "ink"},
		{"Unicorn horn", "unicorn"},
		{"Ram horn", "horn"},
		{"Roast boar", "roast"},
		{"", ""},
	} {
		if got := FromName(c.name); got != c.want {
			t.Errorf("FromName(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
