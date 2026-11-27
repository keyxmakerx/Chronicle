package records

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// counterDoc is the part of the shop-room kind's guide about the counter
// look's theme.
const counterDoc = "`counter` is the theme of the counter look (standing at the counter), a map with these optional keys. " +
	"`start` first resets the theme to a starter (plain, smithy, apothecary, arcane, night market, ship's hold, hollow tree, crystal cavern, undercroft, smugglers' cave, fey glade, caldera, star deck, shop at nowhere, whispering vault, sanctum, taproom, cutting room, chart room, menagerie, reliquary, trading post, frost hall, drowned arcade, caravan tent, sky barge, bog witch's hut, gearworks, dragon's hoard, sun court, haunted parlour, glasshouse); without it the change is made on the saved theme, or on the starter that suits the room's `type` while none is saved. " +
	"`name` (at most 60 characters); the room's shape: `shape` (square, round, open-air, alcove, cavern, hold, hollow, tunnel, glade, void, ribs, taproom, parlour, study, barn, chapel, cabin, ice-cave, reef, tent, gondola, hut, works, colonnade, conservatory), `opening` (none, window-left, window-right, window-centre, round-window, doorway, porthole, grate, knothole, lava-fall, planet, lone-door, rift, portal, hearth, bay-window, harbour-window, stable-door, rose-window, frost-window, aurora, sea-arch, tent-flap, cloud-sea, bog-window, clock-face, dragon-eye, sun-disc, tall-mirror, garden-door), `display` (shelves, pegboard, drawers, pigeonholes, domes, roots, adrift, orbits, columns, tendrils, circles, casks, bolts, scroll-rack, hutches, niches, gear-wall, ice-ledges, shells, carpet-wall, cargo-nets, crooked-shelves, cogs, hoard, clouds, curio, plant-stand), `counter` (straight, curved, stall-table, log, slab, adrift, bar, cutting, plan-chest, feed-stall, altar, woodpile, ice-block, wreck, carpet, gunwale, driftwood, engine, coin-drift, marble-slab, sheeted, potting-bench); " +
	"the paint and light: `wall` (plaster, stone, timber, brick, canvas, night-sky, rock, hull, bark, glade, basalt, nebula, white, obsidian, runestone, daub, damask, chart, barnboard, ashlar, logs, glacier, deepwater, tent-stripes, open-sky, wattle, brass-plate, lair, marble, faded-paper, glazing), `light` (lamp, forge, candles, lanterns, moonlight, crystals, brazier, wisps, star, halo, witchfire, orb, firelight, votives, frostfire, jellyfish, pierced-lamp, cauldron, filament, sunlight, candelabra), `warmth` (0 cool to 1 warm), `outside` (day, dusk, night, rain), `wood`, `cloth`, `accent` and `trim` (each `#rrggbb`); " +
	"and the props: `props` replaces the list (up to 24; none empties it), `add` and `remove` change it (anvil, alembic, jars, crystal, herbs, floating-books, orrery, ladder, banner, sconce, sparks, lanterns, rain, steam-cart, motes, fireflies, drips, cobwebs, nets, chains, mushrooms, tentacle, petals, butterflies, embers, smoke, comets, floating-stones, eyes, glyphs, magic-circle, dew, beads, rune-chase, arcs, bubbles, ghostlights, crumbs, dust, tankards, keg, antlers, mannequin, ribbons, shears, globe, charts, inkwell, birdcages, perch, feathers, haybale, critters, censer, casket, stove, pelts, sacks, snowfall, ice-swan, fish, kelp, caustics, crab, glass-lamps, hookah, cobra, pennants, propeller, grapnel, toad, bottle-tree, gears, steam-puffs, automaton, gauge, glints, dragon-tail, doves, laurel, lyre, ghost, portrait, phonograph, hanging-baskets, palm, watering-can). " +
	"A theme change never lays the furniture out again. The stock lookup prints the saved theme as this same map."

// counterTheme mirrors armory.ShopRoomCounter's JSON, which this package may
// not import; the armory's normalizer stays the authority on every value.
type counterTheme struct {
	Name     string          `json:"name"`
	Version  int             `json:"version"`
	Skeleton counterSkeleton `json:"skeleton"`
	Theme    counterPaint    `json:"theme"`
	Props    []string        `json:"props"`
}

type counterSkeleton struct {
	Shape   string `json:"shape"`
	Opening string `json:"opening"`
	Display string `json:"display"`
	Counter string `json:"counter"`
}

type counterPaint struct {
	Wall    string  `json:"wall"`
	Wood    string  `json:"wood"`
	Cloth   string  `json:"cloth"`
	Accent  string  `json:"accent"`
	Trim    string  `json:"trim"`
	Light   string  `json:"light"`
	Warmth  float64 `json:"warmth"`
	Outside string  `json:"outside"`
	// Picture is the owner's own picture. The AI never sets it; it is carried
	// through unchanged so an edit keeps it.
	Picture json.RawMessage `json:"picture,omitempty"`
}

// The counter's words, with the friendlier ways to say them. Lookups
// normalise spaces and underscores to hyphens first, so synonyms are written
// hyphenated.
var (
	counterStarts = with(wordsOf("plain", "smithy", "apothecary", "arcane", "night-market",
		"ships-hold", "hollow-tree", "crystal-cavern", "undercroft", "smugglers-cave", "fey-glade", "caldera", "star-deck",
		"shop-at-nowhere", "whispering-vault", "sanctum", "taproom", "cutting-room", "chart-room", "menagerie", "reliquary",
		"trading-post", "frost-hall", "drowned-arcade", "caravan-tent", "sky-barge", "bog-witchs-hut", "gearworks", "dragons-hoard",
		"sun-court", "haunted-parlour", "glasshouse"),
		"plain-counter", "plain", "default", "plain", "the-smithy", "smithy", "forge", "smithy", "the-apothecary", "apothecary",
		"the-arcane-shop", "arcane", "arcane-shop", "arcane", "library", "arcane", "the-night-market", "night-market", "market", "night-market",
		"ship's-hold", "ships-hold", "the-ship's-hold", "ships-hold", "the-ships-hold", "ships-hold", "the-hollow-tree", "hollow-tree",
		"the-crystal-cavern", "crystal-cavern", "the-undercroft", "undercroft", "smugglers'-cave", "smugglers-cave", "the-smugglers'-cave", "smugglers-cave",
		"the-smugglers-cave", "smugglers-cave", "the-fey-glade", "fey-glade", "the-caldera", "caldera", "the-star-deck", "star-deck",
		"the-shop-at-nowhere", "shop-at-nowhere", "the-whispering-vault", "whispering-vault", "the-sanctum", "sanctum", "the-taproom", "taproom",
		"the-cutting-room", "cutting-room", "the-chart-room", "chart-room", "the-menagerie", "menagerie", "the-reliquary", "reliquary",
		"the-trading-post", "trading-post", "the-frost-hall", "frost-hall", "the-drowned-arcade", "drowned-arcade", "the-caravan-tent", "caravan-tent",
		"the-sky-barge", "sky-barge", "bog-witch's-hut", "bog-witchs-hut", "the-bog-witch's-hut", "bog-witchs-hut", "the-bog-witchs-hut", "bog-witchs-hut",
		"the-gearworks", "gearworks", "dragon's-hoard", "dragons-hoard", "the-dragon's-hoard", "dragons-hoard", "the-dragons-hoard", "dragons-hoard",
		"the-sun-court", "sun-court", "the-haunted-parlour", "haunted-parlour", "the-glasshouse", "glasshouse")
	counterShapes = with(wordsOf("square", "round", "open-air", "alcove",
		"cavern", "hold", "hollow", "tunnel", "glade", "void", "ribs", "taproom", "parlour", "study",
		"barn", "chapel", "cabin", "ice-cave", "reef", "tent", "gondola", "hut", "works", "colonnade",
		"conservatory"), "open", "open-air", "outdoor", "open-air", "outdoors", "open-air")
	counterOpenings = with(wordsOf("none", "window-left", "window-right", "window-centre", "round-window", "doorway",
		"porthole", "grate", "knothole", "lava-fall", "planet", "lone-door", "rift", "portal", "hearth", "bay-window",
		"harbour-window", "stable-door", "rose-window", "frost-window", "aurora", "sea-arch", "tent-flap", "cloud-sea", "bog-window", "clock-face",
		"dragon-eye", "sun-disc", "tall-mirror", "garden-door"),
		"left-window", "window-left", "right-window", "window-right", "centre-window", "window-centre", "center-window", "window-centre",
		"window-center", "window-centre", "centre", "window-centre", "center", "window-centre", "door", "doorway", "no-window", "none")
	counterDisplays = with(wordsOf("shelves", "pegboard", "drawers", "pigeonholes", "domes",
		"roots", "adrift", "orbits", "columns", "tendrils", "circles", "casks", "bolts", "scroll-rack", "hutches",
		"niches", "gear-wall", "ice-ledges", "shells", "carpet-wall", "cargo-nets", "crooked-shelves", "cogs", "hoard", "clouds",
		"curio", "plant-stand"),
		"shelf", "shelves", "drawer", "drawers", "pigeonhole", "pigeonholes", "dome", "domes", "cloches", "domes")
	counterCounters = with(wordsOf("straight", "curved", "stall-table",
		"log", "slab", "adrift", "bar", "cutting", "plan-chest", "feed-stall", "altar", "woodpile", "ice-block",
		"wreck", "carpet", "gunwale", "driftwood", "engine", "coin-drift", "marble-slab", "sheeted", "potting-bench"), "curve", "curved", "stall", "stall-table", "table", "stall-table")
	counterWalls = with(wordsOf("plaster", "stone", "timber", "brick", "canvas", "night-sky",
		"rock", "hull", "bark", "glade", "basalt", "nebula", "white", "obsidian", "runestone", "daub",
		"damask", "chart", "barnboard", "ashlar", "logs", "glacier", "deepwater", "tent-stripes", "open-sky", "wattle",
		"brass-plate", "lair", "marble", "faded-paper", "glazing"), "wood", "timber", "tent", "canvas", "sky", "night-sky")
	counterLights = with(wordsOf("lamp", "forge", "candles", "lanterns", "moonlight",
		"crystals", "brazier", "wisps", "star", "halo", "witchfire", "orb", "firelight", "votives", "frostfire",
		"jellyfish", "pierced-lamp", "cauldron", "filament", "sunlight", "candelabra"), "candle", "candles", "lantern", "lanterns", "moon", "moonlight")
	counterOutsides = wordsOf("day", "dusk", "night", "rain")
	counterProps    = with(wordsOf("anvil", "alembic", "jars", "crystal", "herbs", "floating-books", "orrery", "ladder", "banner", "sconce", "sparks", "lanterns", "rain", "steam-cart", "motes",
		"fireflies", "drips", "cobwebs", "nets", "chains", "mushrooms", "tentacle", "petals", "butterflies", "embers",
		"smoke", "comets", "floating-stones", "eyes", "glyphs", "magic-circle", "dew", "beads", "rune-chase", "arcs",
		"bubbles", "ghostlights", "crumbs", "dust", "tankards", "keg", "antlers", "mannequin", "ribbons", "shears",
		"globe", "charts", "inkwell", "birdcages", "perch", "feathers", "haybale", "critters", "censer", "casket",
		"stove", "pelts", "sacks", "snowfall", "ice-swan", "fish", "kelp", "caustics", "crab", "glass-lamps",
		"hookah", "cobra", "pennants", "propeller", "grapnel", "toad", "bottle-tree", "gears", "steam-puffs", "automaton",
		"gauge", "glints", "dragon-tail", "doves", "laurel", "lyre", "ghost", "portrait", "phonograph", "hanging-baskets",
		"palm", "watering-can"),
		"jar", "jars", "herb", "herbs", "spark", "sparks", "lantern", "lanterns", "mote", "motes", "books", "floating-books", "banners", "banner", "sconces", "sconce", "steam", "steam-cart", "cart", "steam-cart")
	counterKeys = []string{"start", "name", "shape", "opening", "display", "counter", "wall", "light", "warmth", "outside", "wood", "cloth", "accent", "trim", "props", "add", "remove"}
)

// counterRoomType is the shop type whose starter each `start` word names.
var counterRoomType = map[string]string{"plain": "general", "smithy": "forge", "apothecary": "apothecary", "arcane": "library", "night-market": "market"}

// counterStartName is the starter each other `start` word names: the rooms
// no shop type begins from, which the armory keeps by name.
var counterStartName = map[string]string{
	"ships-hold": "The Ship's Hold", "hollow-tree": "The Hollow Tree", "crystal-cavern": "The Crystal Cavern", "undercroft": "The Undercroft",
	"smugglers-cave": "The Smugglers' Cave", "fey-glade": "The Fey Glade", "caldera": "The Caldera", "star-deck": "The Star Deck",
	"shop-at-nowhere": "The Shop at Nowhere", "whispering-vault": "The Whispering Vault", "sanctum": "The Sanctum", "taproom": "The Taproom",
	"cutting-room": "The Cutting Room", "chart-room": "The Chart Room", "menagerie": "The Menagerie", "reliquary": "The Reliquary",
	"trading-post": "The Trading Post", "frost-hall": "The Frost Hall", "drowned-arcade": "The Drowned Arcade", "caravan-tent": "The Caravan Tent",
	"sky-barge": "The Sky Barge", "bog-witchs-hut": "The Bog Witch's Hut", "gearworks": "The Gearworks", "dragons-hoard": "The Dragon's Hoard",
	"sun-court": "The Sun Court", "haunted-parlour": "The Haunted Parlour", "glasshouse": "The Glasshouse",
}

// CounterStartNames is every starter `start` asks the armory for by name, so
// the wiring test can hold them against the armory's own list.
func CounterStartNames() []string {
	out := make([]string, 0, len(counterStartName))
	for _, n := range counterStartName {
		out = append(out, n)
	}
	return out
}

func counterWord(key, s string, words map[string]string) (string, error) {
	w := strings.NewReplacer(" ", "-", "_", "-").Replace(strings.ToLower(strings.TrimSpace(s)))
	if k, ok := words[w]; ok {
		return k, nil
	}
	return pickWord("counter."+key, s, words)
}

// counter edits the room's counter theme in place and returns the plan's
// words for it ("" when nothing changed). The base is the saved theme, else
// the starter for the room's type; the subkeys are merged onto it and the
// caller's save check hands the result to the armory's validator.
func (k ShopRoomKind) counter(room map[string]any, v any) (string, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", badRequestf("counter: write it as a map of the parts to change, like `shape: round`; the keys are %s", strings.Join(counterKeys, ", "))
	}
	for key := range m {
		if !oneOf(counterKeys, key) {
			return "", badRequestf("counter: %q is not a part of the theme; the keys are %s", key, strings.Join(counterKeys, ", "))
		}
	}
	roomType, _ := room["roomType"].(string)
	saved := room["counter"] != nil
	var cur counterTheme
	var base json.RawMessage
	if saved {
		base, _ = json.Marshal(room["counter"])
	} else {
		base = k.Svc.CounterStarter(roomType)
	}
	if err := json.Unmarshal(base, &cur); err != nil {
		return "", badRequestf("the room's saved counter theme can't be read; build it once in Arrange")
	}
	if cur.Props == nil {
		cur.Props = []string{}
	}
	next := cur
	next.Props = append([]string{}, cur.Props...)
	start := ""
	if r, has := m["start"]; has {
		w, err := counterWord("start", fmt.Sprint(r), counterStarts)
		if err != nil {
			return "", err
		}
		start = w
		starter := k.Svc.CounterStarter(counterRoomType[w])
		if name, named := counterStartName[w]; named {
			starter = k.Svc.CounterStarterNamed(name)
		}
		if err := json.Unmarshal(starter, &next); err != nil {
			return "", badRequestf("counter.start: the starter can't be read")
		}
		if next.Props == nil {
			next.Props = []string{}
		}
	}
	if next.Version == 0 {
		next.Version = 1
	}
	for _, f := range []struct {
		key   string
		field *string
		words map[string]string
	}{
		{"shape", &next.Skeleton.Shape, counterShapes}, {"opening", &next.Skeleton.Opening, counterOpenings},
		{"display", &next.Skeleton.Display, counterDisplays}, {"counter", &next.Skeleton.Counter, counterCounters},
		{"wall", &next.Theme.Wall, counterWalls}, {"light", &next.Theme.Light, counterLights}, {"outside", &next.Theme.Outside, counterOutsides},
	} {
		if r, has := m[f.key]; has {
			w, err := counterWord(f.key, fmt.Sprint(r), f.words)
			if err != nil {
				return "", err
			}
			*f.field = w
		}
	}
	for _, f := range []struct {
		key   string
		field *string
	}{{"wood", &next.Theme.Wood}, {"cloth", &next.Theme.Cloth}, {"accent", &next.Theme.Accent}, {"trim", &next.Theme.Trim}} {
		if r, has := m[f.key]; has {
			c := strings.ToLower(strings.TrimSpace(fmt.Sprint(r)))
			if !hexColor(c) {
				return "", badRequestf("counter.%s: write a colour as #rrggbb, like #6b4527", f.key)
			}
			*f.field = c
		}
	}
	if r, has := m["warmth"]; has {
		w, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(r)), 64)
		if err != nil || w < 0 || w > 1 {
			return "", badRequestf("counter.warmth: write a number from 0 (cool) to 1 (warm)")
		}
		next.Theme.Warmth = w
	}
	if r, has := m["name"]; has {
		n := strings.TrimSpace(fmt.Sprint(r))
		if len([]rune(n)) > 60 {
			return "", badRequestf("counter.name: at most 60 characters")
		}
		next.Name = n
	}
	for _, key := range []string{"props", "add", "remove"} {
		r, has := m[key]
		if !has {
			continue
		}
		ws, err := lines("counter."+key, r)
		if err != nil {
			return "", err
		}
		var got []string
		for _, w := range ws {
			p, err := counterWord(key, w, counterProps)
			if err != nil {
				return "", err
			}
			got = append(got, p)
		}
		switch key {
		case "props":
			next.Props = dedupe(got)
		case "add":
			next.Props = dedupe(append(next.Props, got...))
		case "remove":
			next.Props = without(next.Props, got)
		}
	}
	said := counterChanges(cur, next, start)
	if len(said) == 0 {
		return "", nil
	}
	room["counter"] = next
	return "counter: " + strings.Join(said, ", "), nil
}

// counterChanges names what differs between the theme the room has and the
// one the block makes.
func counterChanges(cur, next counterTheme, start string) []string {
	var said []string
	if reflect.DeepEqual(cur, next) {
		return nil
	}
	if start != "" {
		said = append(said, "start "+strings.ReplaceAll(start, "-", " "))
	}
	if cur.Name != next.Name {
		said = append(said, "name "+quote(next.Name))
	}
	for _, d := range [][3]string{
		{"shape", cur.Skeleton.Shape, next.Skeleton.Shape}, {"opening", cur.Skeleton.Opening, next.Skeleton.Opening},
		{"display", cur.Skeleton.Display, next.Skeleton.Display}, {"counter", cur.Skeleton.Counter, next.Skeleton.Counter},
		{"wall", cur.Theme.Wall, next.Theme.Wall}, {"light", cur.Theme.Light, next.Theme.Light},
		{"warmth", strconv.FormatFloat(cur.Theme.Warmth, 'g', -1, 64), strconv.FormatFloat(next.Theme.Warmth, 'g', -1, 64)},
		{"outside", cur.Theme.Outside, next.Theme.Outside}, {"wood", cur.Theme.Wood, next.Theme.Wood}, {"cloth", cur.Theme.Cloth, next.Theme.Cloth},
		{"accent", cur.Theme.Accent, next.Theme.Accent}, {"trim", cur.Theme.Trim, next.Theme.Trim},
	} {
		if d[1] != d[2] {
			said = append(said, d[0]+" "+d[2])
		}
	}
	for _, p := range next.Props {
		if !oneOf(cur.Props, p) {
			said = append(said, "+"+p)
		}
	}
	for _, p := range cur.Props {
		if !oneOf(next.Props, p) {
			said = append(said, "-"+p)
		}
	}
	return said
}

func dedupe(ws []string) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		if !oneOf(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func without(ws, drop []string) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		if !oneOf(drop, w) {
			out = append(out, w)
		}
	}
	return out
}

// counterBlock writes the saved counter as the same `counter:` map the block
// reads, so it can be sent back changed. With none saved it says so in a
// comment, since the room then uses its type's starter.
func counterBlock(raw json.RawMessage) string {
	var c counterTheme
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &c) != nil {
		return "# counter: none saved, so the room uses the starter for its type; add a `counter:` map (for example `start: smithy`) to theme it\n"
	}
	var b strings.Builder
	b.WriteString("counter:\n")
	line := func(k, v string) {
		if v != "" {
			b.WriteString("  " + k + ": " + v + "\n")
		}
	}
	if c.Name != "" {
		line("name", quote(c.Name))
	}
	line("shape", c.Skeleton.Shape)
	line("opening", c.Skeleton.Opening)
	line("display", c.Skeleton.Display)
	line("counter", c.Skeleton.Counter)
	line("wall", c.Theme.Wall)
	line("light", c.Theme.Light)
	line("warmth", strconv.FormatFloat(c.Theme.Warmth, 'g', -1, 64))
	line("outside", c.Theme.Outside)
	for _, kv := range [][2]string{{"wood", c.Theme.Wood}, {"cloth", c.Theme.Cloth}, {"accent", c.Theme.Accent}, {"trim", c.Theme.Trim}} {
		if kv[1] != "" {
			line(kv[0], quote(kv[1]))
		}
	}
	props := c.Props
	line("props", "["+strings.Join(props, ", ")+"]")
	return b.String()
}
