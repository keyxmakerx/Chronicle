package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const plainCounter = `{"name":"Plain counter","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"shelves","counter":"straight"},"theme":{"wall":"plaster","wood":"#6b4527","cloth":"#5a2024","accent":"#e0a83a","trim":"#c89a4a","light":"lamp","warmth":0.7,"outside":"night"},"props":[]}`
const smithyCounter = `{"name":"The Smithy","version":1,"skeleton":{"shape":"square","opening":"window-right","display":"pegboard","counter":"straight"},"theme":{"wall":"stone","wood":"#4a3220","cloth":"#5a3a22","accent":"#ff9a50","trim":"#9aa0a8","light":"forge","warmth":0.9,"outside":"dusk"},"props":["anvil","sparks"]}`

const arcaneCounter = `{"name":"The Arcane Shop","version":1,"skeleton":{"shape":"round","opening":"round-window","display":"pigeonholes","counter":"curved"},"theme":{"wall":"night-sky","wood":"#4a3a5a","cloth":"#4a2a6a","accent":"#8ab4ff","trim":"#d9b25a","light":"candles","warmth":0.45,"outside":"night"},"props":["orrery","floating-books","crystal","ladder"]}`
const marketCounter = `{"name":"The Night Market","version":1,"skeleton":{"shape":"open-air","opening":"none","display":"domes","counter":"stall-table"},"theme":{"wall":"brick","wood":"#4a2a20","cloth":"#8a2a2e","accent":"#d9a441","trim":"#d9a441","light":"lanterns","warmth":0.8,"outside":"rain"},"props":["rain","steam-cart","motes"]}`

// roomWithCounter is the arranged forge room with a saved counter theme.
func roomWithCounter(counter string) string {
	return strings.Replace(arrangedRoom, `"manner":"gruff"`, `"manner":"gruff","counter":`+counter, 1)
}

func savedCounter(t *testing.T, f *fakeShopLooks, shop string) counterTheme {
	t.Helper()
	var room struct{ Counter *counterTheme }
	if err := json.Unmarshal(f.rooms[shop], &room); err != nil || room.Counter == nil {
		t.Fatalf("no counter saved: %v %s", err, f.rooms[shop])
	}
	return *room.Counter
}

func TestShopRoomCounterEdit(t *testing.T) {
	custom := `{"name":"Mine","version":1,"skeleton":{"shape":"round","opening":"doorway","display":"domes","counter":"curved"},"theme":{"wall":"brick","wood":"#111111","cloth":"#222222","accent":"#333333","trim":"#444444","light":"candles","warmth":0.2,"outside":"rain"},"props":["herbs","jars"]}`
	tests := []struct {
		name    string
		saved   string // the forge room's saved counter, "" for none
		shop    string
		counter any
		summary string
		err     string
		check   func(t *testing.T, c counterTheme)
	}{
		{"a start resets to the starter", custom, "Ember Forge", map[string]any{"start": "Night Market"},
			"Ember Forge's room: counter: start night market, name \"The Night Market\", shape open-air, opening none, counter stall-table, light lanterns, warmth 0.8, wood #4a2a20, cloth #8a2a2e, accent #d9a441, trim #d9a441, +rain, +steam-cart, +motes, -herbs, -jars", "",
			func(t *testing.T, c counterTheme) {
				if c.Name != "The Night Market" || c.Skeleton.Shape != "open-air" {
					t.Fatalf("%+v", c)
				}
			}},
		{"a start then a change", "", "Ember Forge", map[string]any{"start": "arcane", "wall": "Stone", "warmth": 0.5},
			"Ember Forge's room: counter: start arcane, name \"The Arcane Shop\", shape round, opening round-window, display pigeonholes, counter curved, light candles, warmth 0.5, outside night, wood #4a3a5a, cloth #4a2a6a, accent #8ab4ff, trim #d9b25a, +orrery, +floating-books, +crystal, +ladder, -anvil, -sparks", "", func(t *testing.T, c counterTheme) {
				if c.Name != "The Arcane Shop" || c.Theme.Wall != "stone" || c.Theme.Warmth != 0.5 {
					t.Fatalf("%+v", c)
				}
			}},
		{"a partial edit keeps the rest of the saved theme", custom, "Ember Forge", map[string]any{"shape": "alcove", "opening": "centre window", "accent": "#ABCDEF"},
			"Ember Forge's room: counter: shape alcove, opening window-centre, accent #abcdef", "",
			func(t *testing.T, c counterTheme) {
				if c.Name != "Mine" || c.Skeleton.Display != "domes" || c.Theme.Wood != "#111111" || strings.Join(c.Props, ",") != "herbs,jars" {
					t.Fatalf("%+v", c)
				}
			}},
		{"no saved counter edits the type's starter", "", "Ember Forge", map[string]any{"display": "drawers", "name": "Ember's Counter"},
			"Ember Forge's room: counter: name \"Ember's Counter\", display drawers", "",
			func(t *testing.T, c counterTheme) {
				if c.Theme.Light != "forge" || strings.Join(c.Props, ",") != "anvil,sparks" {
					t.Fatalf("the smithy starter is the base: %+v", c)
				}
			}},
		{"props replace", custom, "Ember Forge", map[string]any{"props": []any{"anvil", "Floating Books", "anvil"}},
			"Ember Forge's room: counter: +anvil, +floating-books, -herbs, -jars", "",
			func(t *testing.T, c counterTheme) {
				if strings.Join(c.Props, ",") != "anvil,floating-books" {
					t.Fatalf("%v", c.Props)
				}
			}},
		{"add and remove props", custom, "Ember Forge", map[string]any{"add": []any{"crystal", "herbs"}, "remove": "jars"},
			"Ember Forge's room: counter: +crystal, -jars", "",
			func(t *testing.T, c counterTheme) {
				if strings.Join(c.Props, ",") != "herbs,crystal" {
					t.Fatalf("%v", c.Props)
				}
			}},
		{"props none empties", custom, "Ember Forge", map[string]any{"props": "none"}, "Ember Forge's room: counter: -herbs, -jars", "", nil},
		{"the saved theme again changes nothing", custom, "Ember Forge", map[string]any{"shape": "round", "props": []any{"herbs", "jars"}}, "changes nothing in Ember Forge's room", "", nil},
		{"the type's own starter changes nothing", "", "Ember Forge", map[string]any{"start": "smithy"}, "changes nothing in Ember Forge's room", "", nil},
		{"a bad colour", custom, "Ember Forge", map[string]any{"wood": "brown"}, "", "counter.wood: write a colour as #rrggbb", nil},
		{"a bad word names the choices", custom, "Ember Forge", map[string]any{"shape": "hexagon"}, "", `counter.shape: "hexagon" is not one of alcove, open-air, round, square`, nil},
		{"a bad prop", custom, "Ember Forge", map[string]any{"add": []any{"dragon"}}, "", `counter.add: "dragon" is not one of`, nil},
		{"a bad start", custom, "Ember Forge", map[string]any{"start": "tavern"}, "", `counter.start: "tavern" is not one of`, nil},
		{"a bad warmth", custom, "Ember Forge", map[string]any{"warmth": 2}, "", "counter.warmth: write a number from 0", nil},
		{"an unknown key", custom, "Ember Forge", map[string]any{"colour": "red"}, "", `counter: "colour" is not a part of the theme`, nil},
		{"not a map", custom, "Ember Forge", "round", "", "counter: write it as a map", nil},
		{"a first room still themes", "", "The Rusty Anchor", map[string]any{"wall": "canvas"}, "sets up The Rusty Anchor's room: counter: wall canvas", "",
			func(t *testing.T, c counterTheme) {
				if c.Name != "Plain counter" || c.Theme.Wall != "canvas" {
					t.Fatalf("%+v", c)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, _, f := shopKinds()
			if tt.saved != "" {
				f.rooms["forge"] = json.RawMessage(roomWithCounter(tt.saved))
			}
			r := craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": tt.shop, "counter": tt.counter})
			p := k.Plan(context.Background(), camp, owner, r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || (tt.summary != "" && p.Summary != tt.summary) {
				t.Fatalf("plan = %+v, want %q", p, tt.summary)
			}
			if len(p.Warnings) > 0 && tt.shop == "Ember Forge" {
				t.Fatalf("a counter change must not ask to furnish again: %+v", p)
			}
			if err := k.Apply(context.Background(), camp, owner, r); err != nil {
				t.Fatal(err)
			}
			if tt.check != nil {
				tt.check(t, savedCounter(t, f, map[string]string{"Ember Forge": "forge", "The Rusty Anchor": "anchor"}[tt.shop]))
			}
		})
	}

	// A counter-only change leaves the furnish mark alone.
	k, _, f := shopKinds()
	if err := k.Apply(context.Background(), camp, owner, craftRec("shop-room", ActionUpdate, "", map[string]any{"shop": "Ember Forge", "counter": map[string]any{"wall": "brick"}})); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(f.rooms["forge"], &got)
	if got["furnish"] != nil || got["counter"] == nil {
		t.Fatalf("saved %v", got)
	}
}

// What roomBlock prints, read back as a block, plans no change.
func TestRoomBlockCounterRoundTrip(t *testing.T) {
	custom := `{"name":"Mine, \"quoted\"","version":1,"skeleton":{"shape":"round","opening":"doorway","display":"domes","counter":"curved"},"theme":{"wall":"brick","wood":"#111111","cloth":"#222222","accent":"#333333","trim":"#444444","light":"candles","warmth":0.25,"outside":"rain"},"props":["herbs","floating-books"]}`
	for name, saved := range map[string]string{"saved": roomWithCounter(custom), "none saved": arrangedRoom, "empty props": roomWithCounter(strings.Replace(custom, `["herbs","floating-books"]`, `[]`, 1))} {
		t.Run(name, func(t *testing.T) {
			k, _, _ := shopKinds()
			blk := roomBlock("Ember Forge", json.RawMessage(saved))
			if name == "none saved" {
				if !strings.Contains(blk, "# counter: none saved") || strings.Contains(blk, "\ncounter:") {
					t.Fatalf("block:\n%s", blk)
				}
			} else if !strings.Contains(blk, "\ncounter:\n  name: ") || !strings.Contains(blk, `  wood: "#111111"`) {
				t.Fatalf("block:\n%s", blk)
			}
			r := blockRecord(t, strings.TrimSuffix(blk, "\n"), ActionUpdate)
			// Only the saved counter is under test here; the room's other
			// fields round-trip in TestStockLookupShowsTheShopsLooks.
			k2, _, f := shopKinds()
			_ = k
			f.rooms["forge"] = json.RawMessage(saved)
			p := k2.Plan(context.Background(), camp, owner, r)
			if p.Error != "" || !strings.HasPrefix(p.Summary, "changes nothing") {
				t.Fatalf("read back as %+v\n%s", p, blk)
			}
		})
	}
}
