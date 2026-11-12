package armory

import "encoding/json"

// shopRoomCounterStartersJSON is the counter look's five starter themes. It
// is a single JSON literal so test/js/shop_room_counter_theme.test.mjs can
// extract it and assert it equals the widget's CM_STARTERS: the server (the
// AI connection's base for an edit) and the widget (what a shop with no saved
// theme is drawn as) must never drift apart.
const shopRoomCounterStartersJSON = `[
{"name":"Plain counter","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"shelves","counter":"straight"},"theme":{"wall":"plaster","wood":"#6b4527","cloth":"#5a2024","accent":"#e0a83a","trim":"#c89a4a","light":"lamp","warmth":0.7,"outside":"night"},"props":[]},
{"name":"The Smithy","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"pegboard","counter":"straight"},"theme":{"wall":"stone","wood":"#4a3220","cloth":"#5a3a22","accent":"#ff9a50","trim":"#9aa0a8","light":"forge","warmth":0.9,"outside":"dusk"},"props":["anvil","sparks"]},
{"name":"The Apothecary","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"drawers","counter":"straight"},"theme":{"wall":"plaster","wood":"#7a5230","cloth":"#3a5a3a","accent":"#3fae6a","trim":"#c89a4a","light":"lamp","warmth":0.75,"outside":"dusk"},"props":["herbs","alembic","jars"]},
{"name":"The Arcane Shop","version":1,"skeleton":{"shape":"round","opening":"round-window","display":"pigeonholes","counter":"curved"},"theme":{"wall":"night-sky","wood":"#4a3a5a","cloth":"#4a2a6a","accent":"#8ab4ff","trim":"#d9b25a","light":"candles","warmth":0.45,"outside":"night"},"props":["orrery","floating-books","crystal","ladder"]},
{"name":"The Night Market","version":1,"skeleton":{"shape":"open-air","opening":"none","display":"domes","counter":"stall-table"},"theme":{"wall":"brick","wood":"#4a2a20","cloth":"#8a2a2e","accent":"#d9a441","trim":"#d9a441","light":"lanterns","warmth":0.8,"outside":"rain"},"props":["rain","steam-cart","motes"]}
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
