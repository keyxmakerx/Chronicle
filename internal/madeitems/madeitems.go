// Package madeitems lists the Dark painted made-item pictures: each item's
// key and the colours it was designed in. Crafting picks one for a made
// copy from the item's name; the armory keeps it on the copy's look; the
// pictures themselves and their recolouring are static/js/widgets/made_items.js,
// which lists the same keys in the same order (TestKeysMatchTheScript).
package madeitems

import (
	"strings"
	"unicode"
)

// Item is one painted item and its own body, trim and detail colours, used
// for whatever a recipe's materials do not colour.
type Item struct {
	Key, Body, Trim, Detail string
}

// Items are the painted items, in the order of the signed set.
var Items = []Item{
	{"sword", "#aab3bd", "#b98a3a", "#3f2a22"},
	{"dagger", "#8f99a6", "#c9a23f", "#7a3fd0"},
	{"spear", "#9aa4ae", "#7b5331", "#b0302c"},
	{"mace", "#6f7884", "#8a6238", "#2c2622"},
	{"axe", "#93a0ad", "#7b5331", "#5d2f24"},
	{"hammer", "#6f7884", "#8a6238", "#3c2a22"},
	{"staff", "#5f4630", "#b9c0c8", "#4f8fe0"},
	{"bow", "#8a5a32", "#3b2a24", "#e6dcc8"},
	{"crossbow", "#7a4f2c", "#55595f", "#d8d2c2"},
	{"arrows", "#6b4a2e", "#b98a3a", "#e6e0d0"},
	{"wand", "#3b2a24", "#c7ccd6", "#3fd0b0"},
	{"flail", "#59606a", "#6e4a2c", "#2c2622"},
	{"shield", "#2f4f7a", "#aab3bd", "#d8c070"},
	{"gauntlet", "#9aa4ae", "#b98a3a", "#5a2a2a"},
	{"helm", "#9aa4ae", "#b98a3a", "#9b2c2c"},
	{"breastplate", "#aab3bd", "#c9a23f", "#3b7fd4"},
	{"cloak", "#3c5a44", "#c9a23f", "#c0392b"},
	{"ring", "#c9a23f", "#c9a23f", "#1f7a4a"},
	{"crown", "#d4af4a", "#d4af4a", "#a3262e"},
	{"mask", "#e4dccb", "#c9a23f", "#7a1f2a"},
	{"pouch", "#7a5534", "#b98a3a", "#4a3220"},
	{"amulet", "#c9a23f", "#c9a23f", "#c0392b"},
	{"pickaxe", "#59606a", "#8a6238", "#3a2b24"},
	{"tongs", "#4d535b", "#8a6238", "#ff8a2a"},
	{"mortar", "#8f8b84", "#7b5331", "#6f9a3a"},
	{"lantern", "#3f4349", "#b98a3a", "#ffb347"},
	{"key", "#8a6f3a", "#c9a23f", "#c0392b"},
	{"spool", "#a3262e", "#b08a5a", "#c7ccd6"},
	{"anvil", "#5d646e", "#6e4a2c", "#2c2622"},
	{"compass", "#b98a3a", "#b98a3a", "#b0302c"},
	{"tome", "#5a2a2a", "#b98a3a", "#3fa67a"},
	{"scroll", "#b98a3a", "#5a3a26", "#a3262e"},
	{"orb", "#4a3f8f", "#b9c0c8", "#b9a6ff"},
	{"candle", "#e8dcc0", "#8a6f3a", "#7fd0ff"},
	{"runestone", "#6a6e72", "#8a6238", "#5dd6ff"},
	{"hourglass", "#9fc7c9", "#6e4a2c", "#e0b04a"},
	{"censer", "#b98a3a", "#d4af4a", "#ff7a2a"},
	{"crystal", "#7a4fd0", "#55504a", "#d08ae0"},
	{"bread", "#b5772e", "#8a6238", "#ecd9a0"},
	{"tankard", "#8a5a32", "#55595f", "#f1e6c8"},
	{"pie", "#c98f3c", "#7a8088", "#7a1f3a"},
	{"roast", "#9a4f24", "#e6dcc8", "#4f7a2a"},
	{"stew", "#b5622a", "#7a5534", "#d8b45a"},
	{"bottle", "#2f5a3c", "#b08a5a", "#a3262e"},
	{"ingot", "#c9a23f", "#c9a23f", "#c9a23f"},
	{"coins", "#c9a23f", "#d8d8de", "#b0703c"},
	{"chest", "#6e4a2c", "#55595f", "#c9a23f"},
	{"lute", "#b5772e", "#5a3a26", "#e6dcc8"},
	{"fang", "#e8e0cc", "#8fe04a", "#6a2a30"},
	{"claw", "#2c2a2e", "#5a6b3a", "#b0703c"},
	{"horn", "#7a6a58", "#8a6f3a", "#ffcf5a"},
	{"scale", "#2f7a6a", "#1f4f5a", "#ff8a3a"},
	{"feather", "#1f3f8a", "#e8e0cc", "#3fd0b0"},
	{"eye", "#e6dccb", "#7a2a3a", "#d0a020"},
	{"core", "#d0401f", "#4a4440", "#ffb347"},
	{"wisp", "#7fd6e6", "#8a8f98", "#e6ffff"},
	{"heart", "#5e1a24", "#3f141c", "#ff6a8a"},
	{"tentacle", "#4a3a6a", "#c9b8d8", "#5dff9a"},
	{"carapace", "#3a2f4f", "#c9b890", "#3fd0e0"},
	{"knot", "#8a8f98", "#5a4a7a", "#b07bff"},
	{"pelt", "#b0703c", "#6a4a32", "#2c1f1a"},
	{"skull", "#ddd3bd", "#6a5a48", "#5dff9a"},
	{"meat", "#8e2a2a", "#b9a583", "#5d7a3a"},
	{"hand", "#b7a888", "#c9a23f", "#7dffc0"},
	{"geode", "#5a5148", "#2f7f96", "#8fe8ff"},
	{"unicorn", "#f1ece2", "#c9d0d8", "#ffe9a8"},
	{"starfrag", "#4a4e58", "#5a4a3c", "#a8d8ff"},
	{"ink", "#cfdcd8", "#8a5a34", "#4a35c8"},
	{"conch", "#e2c9a6", "#f2a79c", "#8fe0ff"},
	{"mandrake", "#8a6a48", "#b8a47a", "#3f6a34"},
	{"egg", "#3f5a4a", "#c2a25a", "#ff9a3a"},
	{"wing", "#bfe6ff", "#3a2f4a", "#ffe08a"},
	{"potion", "#d8e4e0", "#5e1616", "#c8182e"},
	{"ore", "#5c5a58", "#e2ded6", "#d4a23f"},
	{"bolt", "#1f6a6a", "#c9a23f", "#8a6238"},
	{"herbs", "#6f8a4a", "#b8a47a", "#8a6fc0"},
	{"boots", "#6b4a2e", "#b98a3a", "#3a2a20"},
	{"torch", "#6e4a2c", "#3f4349", "#ff9a3a"},
	{"bomb", "#6a6c72", "#b8a47a", "#ffb347"},
	{"gems", "#1f9a5a", "#35204a", "#c0283c"},
}

var byKey = func() map[string]Item {
	m := make(map[string]Item, len(Items))
	for _, it := range Items {
		m[it.Key] = it
	}
	return m
}()

// Has reports whether key names a painted item.
func Has(key string) bool { _, ok := byKey[key]; return ok }

// Get is the painted item key names.
func Get(key string) (Item, bool) { it, ok := byKey[key]; return it, ok }

// words pick a painted item from a made item's name; the first match in this
// order wins, so "Dragon scale armour" is armour and "Potion of giant
// strength" a bottle. Each item is listed once, its key first among its words.
var words = []struct{ key, words string }{
	{"shield", "shield buckler targe pavise"},
	{"crossbow", "crossbow arbalest"},
	{"bow", "bow longbow shortbow greatbow"},
	{"arrows", "arrows arrow quiver bolts"},
	{"dagger", "dagger knife dirk stiletto kris"},
	{"sword", "sword blade longsword sabre saber rapier scimitar cutlass falchion katana"},
	{"spear", "spear glaive halberd pike lance trident javelin polearm"},
	{"axe", "axe hatchet greataxe battleaxe handaxe"},
	{"mace", "mace club morningstar cudgel"},
	{"flail", "flail"},
	{"bomb", "bomb grenade explosive firebomb"},
	{"hammer", "hammer maul warhammer mallet"},
	{"pickaxe", "pickaxe pick mattock"},
	{"staff", "staff quarterstaff stave"},
	{"wand", "wand rod sceptre scepter"},
	{"boots", "boots boot shoes sandals greaves"},
	{"gauntlet", "gauntlet gauntlets glove gloves bracer bracers"},
	{"helm", "helm helmet coif"},
	{"breastplate", "breastplate armour armor cuirass hauberk chainmail mail plate brigandine"},
	{"cloak", "cloak cape mantle robe shroud"},
	{"crown", "crown circlet tiara diadem coronet"},
	{"mask", "mask visage"},
	{"amulet", "amulet pendant necklace talisman locket torc medallion charm"},
	{"ring", "ring band signet"},
	{"pouch", "pouch bag sack purse satchel"},
	{"tongs", "tongs pliers"},
	{"mortar", "mortar pestle"},
	{"torch", "torch brand"},
	{"lantern", "lantern lamp"},
	{"key", "key skeleton"},
	{"spool", "spool thread needle yarn"},
	{"anvil", "anvil"},
	{"compass", "compass"},
	{"hourglass", "hourglass timer"},
	{"tome", "tome book grimoire spellbook manual codex journal"},
	{"scroll", "scroll parchment"},
	{"orb", "orb sphere globe"},
	{"candle", "candle taper"},
	{"runestone", "runestone rune glyph"},
	{"censer", "censer incense thurible"},
	{"ink", "ink inkwell"},
	{"potion", "potion elixir draught philter philtre tonic remedy healing"},
	{"bottle", "bottle potion elixir draught brew philter philtre tonic tincture salve remedy oil poison venom extract essence vial flask"},
	{"tankard", "tankard mug ale beer mead stein"},
	{"bread", "bread loaf bun biscuit cake"},
	{"pie", "pie tart pastry"},
	{"roast", "roast drumstick chicken fowl"},
	{"stew", "stew soup broth porridge meal ration rations"},
	{"meat", "meat jerky steak ham haunch"},
	{"chest", "chest coffer strongbox casket"},
	{"coins", "coins coin gold"},
	{"ingot", "ingot bar bullion"},
	{"ore", "ore nugget lode"},
	{"lute", "lute lyre harp instrument"},
	{"herbs", "herbs herb sage lavender thyme rosemary"},
	{"mandrake", "mandrake root"},
	{"unicorn", "unicorn alicorn"},
	{"geode", "geode"},
	{"gems", "gems jewels jewel gemstones"},
	{"crystal", "crystal gem gemstone quartz amethyst shard"},
	{"starfrag", "star meteor meteorite starmetal"},
	{"conch", "conch seashell"},
	{"egg", "egg"},
	{"skull", "skull"},
	{"hand", "hand"},
	{"heart", "heart"},
	{"eye", "eye"},
	{"fang", "fang tooth teeth tusk"},
	{"claw", "claw talon"},
	{"horn", "horn antler"},
	{"wing", "wing wings"},
	{"feather", "feather plume quill"},
	{"scale", "scale scales"},
	{"tentacle", "tentacle"},
	{"carapace", "carapace shell chitin"},
	{"pelt", "pelt hide fur fleece leather"},
	{"bolt", "cloth fabric silk linen velvet satin wool"},
	{"knot", "knot rope cord"},
	{"wisp", "wisp ghost spirit soul"},
	{"core", "core ember cinder"},
}

// FromName is the painted item a made item's name points to, "" when no word
// in it does.
func FromName(name string) string {
	ws := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) })
	for _, w := range words {
		if !Has(w.key) {
			continue
		}
		for _, cand := range strings.Fields(w.words) {
			for _, n := range ws {
				if wordMatches(n, cand) {
					return w.key
				}
			}
		}
	}
	return ""
}

// wordMatches compares whole words, so "Boiled leather" isn't oil and
// "Bandage" isn't a band. A word may add a plural ending ("daggers"), and
// may end in a candidate of four letters or more ("longsword"); shorter
// ones match whole only, so "Dragonscale" is never ale.
func wordMatches(word, w string) bool {
	if word == w {
		return true
	}
	word = strings.TrimSuffix(word, "s")
	if strings.HasSuffix(word, "e") && !strings.HasSuffix(w, "e") {
		word = strings.TrimSuffix(word, "e")
	}
	return word == w || (len(w) >= 4 && len(word) > len(w)+2 && strings.HasSuffix(word, w))
}
