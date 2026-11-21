package armory

import "encoding/json"

// shopRoomCounterStartersJSON is the counter look's starter themes: the five
// a shop type begins from, then the rooms an owner can pick by name. It
// is a single JSON literal so test/js/shop_room_counter_theme.test.mjs can
// extract it and assert it equals the widget's CM_STARTERS: the server (the
// AI connection's base for an edit) and the widget (what a shop with no saved
// theme is drawn as) must never drift apart.
const shopRoomCounterStartersJSON = `[
{"name":"Plain counter","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"shelves","counter":"straight"},"theme":{"wall":"plaster","wood":"#6b4527","cloth":"#5a2024","accent":"#e0a83a","trim":"#c89a4a","light":"lamp","warmth":0.7,"outside":"night"},"props":[]},
{"name":"The Smithy","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"pegboard","counter":"straight"},"theme":{"wall":"stone","wood":"#4a3220","cloth":"#5a3a22","accent":"#ff9a50","trim":"#9aa0a8","light":"forge","warmth":0.9,"outside":"dusk"},"props":["anvil","sparks"]},
{"name":"The Apothecary","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"drawers","counter":"straight"},"theme":{"wall":"plaster","wood":"#7a5230","cloth":"#3a5a3a","accent":"#3fae6a","trim":"#c89a4a","light":"lamp","warmth":0.75,"outside":"dusk"},"props":["herbs","alembic","jars"]},
{"name":"The Arcane Shop","version":1,"skeleton":{"shape":"round","opening":"round-window","display":"pigeonholes","counter":"curved"},"theme":{"wall":"night-sky","wood":"#4a3a5a","cloth":"#4a2a6a","accent":"#8ab4ff","trim":"#d9b25a","light":"candles","warmth":0.45,"outside":"night"},"props":["orrery","floating-books","crystal","ladder"]},
{"name":"The Night Market","version":1,"skeleton":{"shape":"open-air","opening":"none","display":"domes","counter":"stall-table"},"theme":{"wall":"brick","wood":"#4a2a20","cloth":"#8a2a2e","accent":"#d9a441","trim":"#d9a441","light":"lanterns","warmth":0.8,"outside":"rain"},"props":["rain","steam-cart","motes"]},
{"name":"The Ship's Hold","version":1,"skeleton":{"shape":"hold","opening":"porthole","display":"pegboard","counter":"straight"},"theme":{"wall":"hull","wood":"#5a3c24","cloth":"#1f4a5a","accent":"#5fc8d8","trim":"#c8a04a","light":"lamp","warmth":0.7,"outside":"dusk"},"props":["nets","chains"]},
{"name":"The Hollow Tree","version":1,"skeleton":{"shape":"hollow","opening":"knothole","display":"shelves","counter":"curved"},"theme":{"wall":"bark","wood":"#6a4a2a","cloth":"#3a5a2a","accent":"#b8e060","trim":"#c8a04a","light":"candles","warmth":0.75,"outside":"day"},"props":["fireflies","mushrooms","herbs"]},
{"name":"The Crystal Cavern","version":1,"skeleton":{"shape":"cavern","opening":"none","display":"pigeonholes","counter":"straight"},"theme":{"wall":"rock","wood":"#4a4038","cloth":"#2a3a6a","accent":"#7fd8ff","trim":"#a8b0c0","light":"crystals","warmth":0.3,"outside":"night"},"props":["drips","motes","crystal"]},
{"name":"The Undercroft","version":1,"skeleton":{"shape":"tunnel","opening":"grate","display":"pegboard","counter":"straight"},"theme":{"wall":"brick","wood":"#3a2a20","cloth":"#4a2a4a","accent":"#8ae06a","trim":"#8a8a80","light":"brazier","warmth":0.85,"outside":"night"},"props":["drips","cobwebs","chains","tentacle"]},
{"name":"The Smugglers' Cave","version":1,"skeleton":{"shape":"cavern","opening":"grate","display":"drawers","counter":"straight"},"theme":{"wall":"rock","wood":"#5a4028","cloth":"#6a2a20","accent":"#ffb050","trim":"#b08a4a","light":"brazier","warmth":0.9,"outside":"night"},"props":["nets","chains","jars","cobwebs"]},
{"name":"The Fey Glade","version":1,"skeleton":{"shape":"glade","opening":"none","display":"roots","counter":"log"},"theme":{"wall":"glade","wood":"#6a4a2c","cloth":"#5a2a6a","accent":"#ff9ad5","trim":"#d8c070","light":"wisps","warmth":0.4,"outside":"night"},"props":["petals","butterflies","fireflies","dew","dust"]},
{"name":"The Caldera","version":1,"skeleton":{"shape":"cavern","opening":"lava-fall","display":"columns","counter":"slab"},"theme":{"wall":"basalt","wood":"#3a2a22","cloth":"#6a1a10","accent":"#ff7a30","trim":"#8a7a6a","light":"brazier","warmth":1,"outside":"night"},"props":["embers","smoke","anvil","chains","bubbles"]},
{"name":"The Star Deck","version":1,"skeleton":{"shape":"void","opening":"planet","display":"orbits","counter":"adrift"},"theme":{"wall":"nebula","wood":"#2a2a4a","cloth":"#1a2a6a","accent":"#8ab4ff","trim":"#c8d0e8","light":"star","warmth":0.5,"outside":"night"},"props":["comets","orrery","floating-books","motes","beads"]},
{"name":"The Shop at Nowhere","version":1,"skeleton":{"shape":"void","opening":"lone-door","display":"adrift","counter":"adrift"},"theme":{"wall":"white","wood":"#b0a89c","cloth":"#3a4a7a","accent":"#6a8aff","trim":"#c8b070","light":"halo","warmth":0.5,"outside":"night"},"props":["floating-stones","motes","crumbs"]},
{"name":"The Whispering Vault","version":1,"skeleton":{"shape":"ribs","opening":"rift","display":"tendrils","counter":"slab"},"theme":{"wall":"obsidian","wood":"#2a2030","cloth":"#3a1a4a","accent":"#9aff6a","trim":"#8a8090","light":"witchfire","warmth":0.6,"outside":"night"},"props":["eyes","tentacle","glyphs","cobwebs","arcs","ghostlights"]},
{"name":"The Sanctum","version":1,"skeleton":{"shape":"round","opening":"portal","display":"circles","counter":"slab"},"theme":{"wall":"runestone","wood":"#3a2a4a","cloth":"#3a1a6a","accent":"#b08aff","trim":"#d8b860","light":"orb","warmth":0.5,"outside":"night"},"props":["magic-circle","glyphs","floating-books","crystal","rune-chase"]},
{"name":"The Taproom","version":1,"skeleton":{"shape":"taproom","opening":"hearth","display":"casks","counter":"bar"},"theme":{"wall":"daub","wood":"#5a3a22","cloth":"#7a2a22","accent":"#e8a040","trim":"#b89a58","light":"firelight","warmth":0.95,"outside":"night"},"props":["tankards","keg","antlers","smoke"]},
{"name":"The Cutting Room","version":1,"skeleton":{"shape":"parlour","opening":"bay-window","display":"bolts","counter":"cutting"},"theme":{"wall":"damask","wood":"#4a2c1c","cloth":"#6a2438","accent":"#e8c860","trim":"#c8a860","light":"lamp","warmth":0.75,"outside":"day"},"props":["mannequin","ribbons","shears","motes"]},
{"name":"The Chart Room","version":1,"skeleton":{"shape":"study","opening":"harbour-window","display":"scroll-rack","counter":"plan-chest"},"theme":{"wall":"chart","wood":"#3e2a1a","cloth":"#1f4a36","accent":"#c8442a","trim":"#c8a04a","light":"lamp","warmth":0.7,"outside":"dusk"},"props":["globe","charts","inkwell"]},
{"name":"The Menagerie","version":1,"skeleton":{"shape":"barn","opening":"stable-door","display":"hutches","counter":"feed-stall"},"theme":{"wall":"barnboard","wood":"#6a4a2a","cloth":"#3a5a2a","accent":"#e0b040","trim":"#9a9a94","light":"lamp","warmth":0.85,"outside":"day"},"props":["birdcages","perch","haybale","critters","feathers"]},
{"name":"The Reliquary","version":1,"skeleton":{"shape":"chapel","opening":"rose-window","display":"niches","counter":"altar"},"theme":{"wall":"ashlar","wood":"#4a3a2a","cloth":"#7a1a2a","accent":"#e8c050","trim":"#d8b860","light":"votives","warmth":0.8,"outside":"day"},"props":["censer","casket","motes"]},
{"name":"The Trading Post","version":1,"skeleton":{"shape":"cabin","opening":"frost-window","display":"gear-wall","counter":"woodpile"},"theme":{"wall":"logs","wood":"#6a4a30","cloth":"#8a2a20","accent":"#e8a040","trim":"#8a8a8a","light":"lamp","warmth":0.9,"outside":"night"},"props":["stove","pelts","sacks"]},
{"name":"The Frost Hall","version":1,"skeleton":{"shape":"ice-cave","opening":"aurora","display":"ice-ledges","counter":"ice-block"},"theme":{"wall":"glacier","wood":"#5a6a7a","cloth":"#2a4a7a","accent":"#8ae0ff","trim":"#c8d8e8","light":"frostfire","warmth":0.2,"outside":"night"},"props":["snowfall","ice-swan","crystal","drips"]},
{"name":"The Drowned Arcade","version":1,"skeleton":{"shape":"reef","opening":"sea-arch","display":"shells","counter":"wreck"},"theme":{"wall":"deepwater","wood":"#3a5a5a","cloth":"#1a4a5a","accent":"#5fe8d0","trim":"#c8b070","light":"jellyfish","warmth":0.3,"outside":"day"},"props":["fish","kelp","crab","caustics"]},
{"name":"The Caravan Tent","version":1,"skeleton":{"shape":"tent","opening":"tent-flap","display":"carpet-wall","counter":"carpet"},"theme":{"wall":"tent-stripes","wood":"#6a4a2a","cloth":"#8a2a3a","accent":"#e8a030","trim":"#d8a840","light":"pierced-lamp","warmth":0.95,"outside":"dusk"},"props":["glass-lamps","hookah","cobra","motes"]},
{"name":"The Sky Barge","version":1,"skeleton":{"shape":"gondola","opening":"cloud-sea","display":"cargo-nets","counter":"gunwale"},"theme":{"wall":"open-sky","wood":"#6a4a2c","cloth":"#a83a2a","accent":"#ffd070","trim":"#c8a04a","light":"lamp","warmth":0.6,"outside":"day"},"props":["pennants","propeller","grapnel"]},
{"name":"The Bog Witch's Hut","version":1,"skeleton":{"shape":"hut","opening":"bog-window","display":"crooked-shelves","counter":"driftwood"},"theme":{"wall":"wattle","wood":"#4a3a26","cloth":"#3a2a4a","accent":"#9ae04a","trim":"#8a7a4a","light":"cauldron","warmth":0.5,"outside":"night"},"props":["herbs","toad","bottle-tree","cobwebs","fireflies"]},
{"name":"The Gearworks","version":1,"skeleton":{"shape":"works","opening":"clock-face","display":"cogs","counter":"engine"},"theme":{"wall":"brass-plate","wood":"#4a3a2a","cloth":"#2a3a3a","accent":"#ffb040","trim":"#c89a4a","light":"filament","warmth":0.8,"outside":"night"},"props":["gears","automaton","gauge","steam-puffs"]},
{"name":"The Dragon's Hoard","version":1,"skeleton":{"shape":"cavern","opening":"dragon-eye","display":"hoard","counter":"coin-drift"},"theme":{"wall":"lair","wood":"#3a2a1a","cloth":"#6a1a1a","accent":"#ffc840","trim":"#e8c050","light":"brazier","warmth":0.9,"outside":"night"},"props":["dragon-tail","glints","smoke","embers"]},
{"name":"The Sun Court","version":1,"skeleton":{"shape":"colonnade","opening":"sun-disc","display":"clouds","counter":"marble-slab"},"theme":{"wall":"marble","wood":"#b8a888","cloth":"#2a4a8a","accent":"#ffd860","trim":"#e8c860","light":"sunlight","warmth":0.8,"outside":"day"},"props":["doves","laurel","lyre","orrery","motes"]},
{"name":"The Haunted Parlour","version":1,"skeleton":{"shape":"parlour","opening":"tall-mirror","display":"curio","counter":"sheeted"},"theme":{"wall":"faded-paper","wood":"#3a2c28","cloth":"#3a2a4a","accent":"#9ad8e8","trim":"#8a8478","light":"candelabra","warmth":0.35,"outside":"night"},"props":["ghost","portrait","phonograph","cobwebs","ghostlights","dust"]},
{"name":"The Glasshouse","version":1,"skeleton":{"shape":"conservatory","opening":"garden-door","display":"plant-stand","counter":"potting-bench"},"theme":{"wall":"glazing","wood":"#6a5a44","cloth":"#2a5a3a","accent":"#ff8ab0","trim":"#c8b070","light":"lamp","warmth":0.7,"outside":"day"},"props":["hanging-baskets","palm","watering-can","butterflies","motes"]}
]`

// shopRoomCounterStarterFor maps a shop type to its starter's position in the
// list above, as the widget's CM_START_FOR does; any other type starts plain.
var shopRoomCounterStarterFor = map[string]int{"forge": 1, "apothecary": 2, "library": 3, "market": 4}

// ShopRoomCounterStarter is the theme a shop of this type is drawn with until
// someone saves its own, so an edit of an unsaved counter starts from what
// the room already looks like.
func ShopRoomCounterStarter(roomType string) json.RawMessage {
	var all []json.RawMessage
	if err := json.Unmarshal([]byte(shopRoomCounterStartersJSON), &all); err != nil {
		return nil
	}
	return all[shopRoomCounterStarterFor[roomType]]
}

// ShopRoomCounterStarterNamed is the starter theme with this name, nil when
// there is none, for the starters no shop type begins from.
func ShopRoomCounterStarterNamed(name string) json.RawMessage {
	var all []json.RawMessage
	if err := json.Unmarshal([]byte(shopRoomCounterStartersJSON), &all); err != nil {
		return nil
	}
	for _, raw := range all {
		var c struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &c) == nil && c.Name == name {
			return raw
		}
	}
	return nil
}
