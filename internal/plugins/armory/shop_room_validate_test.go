package armory

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// goodRoom returns a valid layout as a generic map so each case can break one
// field without restating the rest.
func goodRoom() map[string]any {
	return map[string]any{
		"version": 1, "roomType": "forge", "setting": "room", "size": "m",
		"furniture": "normal", "decorations": "some", "palette": "oak",
		"seeds": map[string]any{"room": 1, "goods": 2, "deco": 3},
		"pieces": []any{
			map[string]any{"id": 1, "kind": "anvil", "wall": "", "x": 2.5, "y": 3, "off": 0, "len": 1, "w": 1, "d": 1, "pinned": true},
		},
		"items":    map[string]any{"12": map[string]any{"icon": "hammer", "color": "steel"}},
		"portrait": map[string]any{"left": 10.5, "top": 90},
		"lines":    []any{"  Welcome  ", "", "   "},
	}
}

func mutate(f func(m map[string]any)) []byte {
	m := goodRoom()
	f(m)
	b, _ := json.Marshal(m)
	return b
}

func pieceWith(k string, v any) func(m map[string]any) {
	return func(m map[string]any) {
		p := m["pieces"].([]any)[0].(map[string]any)
		p[k] = v
	}
}

func TestNormalizeShopRoomLayout_Rejects(t *testing.T) {
	manyPieces := func(n int) func(m map[string]any) {
		return func(m map[string]any) {
			ps := make([]any, n)
			for i := range ps {
				ps[i] = map[string]any{"id": i + 1, "kind": "rug", "wall": "", "x": 1, "y": 1, "off": 0, "len": 1, "w": 1, "d": 1, "pinned": false}
			}
			m["pieces"] = ps
		}
	}
	manyItems := func(n int) func(m map[string]any) {
		return func(m map[string]any) {
			it := map[string]any{}
			for i := 1; i <= n; i++ {
				it[strconv.Itoa(i)] = map[string]any{"icon": "", "color": ""}
			}
			m["items"] = it
		}
	}
	item := func(k string, v any) func(m map[string]any) {
		return func(m map[string]any) { m["items"] = map[string]any{k: v} }
	}
	look := func(icon, color string) map[string]any { return map[string]any{"icon": icon, "color": color} }

	effect := func(k string, v any) func(m map[string]any) {
		return func(m map[string]any) { m["effects"] = map[string]any{k: v} }
	}

	cases := []struct {
		name string
		body []byte
	}{
		{"not json", []byte("nope")},
		{"trailing data", append(mutate(func(map[string]any) {}), []byte(` {}`)...)},
		{"array body", []byte("[]")},
		{"bad version", mutate(func(m map[string]any) { m["version"] = 2 })},
		{"missing version", mutate(func(m map[string]any) { delete(m, "version") })},
		{"bad roomType", mutate(func(m map[string]any) { m["roomType"] = "bank" })},
		{"bad setting", mutate(func(m map[string]any) { m["setting"] = "moon" })},
		{"bad size", mutate(func(m map[string]any) { m["size"] = "xl" })},
		{"bad furniture", mutate(func(m map[string]any) { m["furniture"] = "" })},
		{"decoration on a missing piece", mutate(func(m map[string]any) { m["decor"] = []any{map[string]any{"piece": 9, "spot": 0, "icon": "gem"}} })},
		{"decoration spot out of range", mutate(func(m map[string]any) { m["decor"] = []any{map[string]any{"piece": 1, "spot": 64, "icon": "gem"}} })},
		{"decoration with no icon", mutate(func(m map[string]any) { m["decor"] = []any{map[string]any{"piece": 1, "spot": 0, "icon": ""}} })},
		{"decoration with an unknown icon", mutate(func(m map[string]any) { m["decor"] = []any{map[string]any{"piece": 1, "spot": 0, "icon": "rocket"}} })},
		{"too many decorations", mutate(func(m map[string]any) {
			ds := make([]any, maxShopRoomDecor+1)
			for i := range ds {
				ds[i] = map[string]any{"piece": 1, "spot": 0, "icon": "gem"}
			}
			m["decor"] = ds
		})},
		{"bad decorations", mutate(func(m map[string]any) { m["decorations"] = "many" })},
		{"bad palette", mutate(func(m map[string]any) { m["palette"] = "pink" })},
		{"bad look", mutate(func(m map[string]any) { m["look"] = "neon" })},
		{"bad mood", mutate(func(m map[string]any) { m["mood"] = "grimdark" })},
		{"effects shadows negative", mutate(effect("shadows", -1))},
		{"effects shadows high", mutate(effect("shadows", 251))},
		{"effects warmth high", mutate(effect("warmth", 251))},
		{"effects window high", mutate(effect("window", 251))},
		{"effects haze negative", mutate(effect("haze", -1))},
		{"effects vignette high", mutate(effect("vignette", 251))},
		{"effects fractional", mutate(effect("haze", 1.5))},
		{"effects dust type", mutate(effect("dust", "yes"))},
		{"effects weather", mutate(effect("weather", "hail"))},
		{"effects time", mutate(effect("time", "noon"))},
		{"seed zero", mutate(func(m map[string]any) { m["seeds"] = map[string]any{"room": 0, "goods": 2, "deco": 3} })},
		{"seed too big", mutate(func(m map[string]any) { m["seeds"] = map[string]any{"room": 1, "goods": 1000000001, "deco": 3} })},
		{"seed missing", mutate(func(m map[string]any) { m["seeds"] = map[string]any{"room": 1, "goods": 2} })},
		{"seed fractional", mutate(func(m map[string]any) { m["seeds"] = map[string]any{"room": 1.5, "goods": 2, "deco": 3} })},
		{"too many pieces", mutate(manyPieces(81))},
		{"piece id zero", mutate(pieceWith("id", 0))},
		{"piece id too big", mutate(pieceWith("id", 100001))},
		{"piece kind", mutate(pieceWith("kind", "throne"))},
		{"piece wall", mutate(pieceWith("wall", "Z"))},
		{"piece x low", mutate(pieceWith("x", -1.01))},
		{"piece y high", mutate(pieceWith("y", 20.5))},
		{"piece off", mutate(pieceWith("off", 99))},
		{"piece len", mutate(pieceWith("len", -5))},
		{"piece w", mutate(pieceWith("w", 21))},
		{"piece d", mutate(pieceWith("d", -2))},
		{"piece pinned type", mutate(pieceWith("pinned", "yes"))},
		{"duplicate piece id", mutate(func(m map[string]any) {
			p := m["pieces"].([]any)[0]
			m["pieces"] = []any{p, p}
		})},
		{"too many items", mutate(manyItems(501))},
		{"item key not number", mutate(item("abc", look("", "")))},
		{"item key zero", mutate(item("0", look("", "")))},
		{"item key leading zero", mutate(item("012", look("", "")))},
		{"item key negative", mutate(item("-1", look("", "")))},
		{"item key too big", mutate(item("2147483648", look("", "")))},
		{"item key empty", mutate(item("", look("", "")))},
		{"item icon", mutate(item("1", look("trash", "")))},
		{"item color", mutate(item("1", look("", "purple")))},
		{"portrait left", mutate(func(m map[string]any) { m["portrait"] = map[string]any{"left": 101, "top": 0} })},
		{"portrait top", mutate(func(m map[string]any) { m["portrait"] = map[string]any{"left": 0, "top": -1} })},
		{"too many lines", mutate(func(m map[string]any) {
			ls := make([]any, 11)
			for i := range ls {
				ls[i] = "x"
			}
			m["lines"] = ls
		})},
		{"line too long", mutate(func(m map[string]any) { m["lines"] = []any{strings.Repeat("a", 201)} })},
		{"line too long runes", mutate(func(m map[string]any) { m["lines"] = []any{strings.Repeat("é", 201)} })},
		{"unknown manner", mutate(func(m map[string]any) { m["manner"] = "sly" })},
		{"unknown keeper spot", mutate(func(m map[string]any) { m["keeperSpot"] = "under" })},
		{"unknown moment", mutate(func(m map[string]any) { m["moments"] = map[string]any{"haggle": []any{"No."}} })},
		{"too many moment lines", mutate(func(m map[string]any) {
			m["moments"] = map[string]any{"hello": []any{"a", "b", "c", "d", "e", "f"}}
		})},
		{"moment line too long", mutate(func(m map[string]any) { m["moments"] = map[string]any{"bye": []any{strings.Repeat("é", 201)}} })},
		{"size cap", append([]byte(`{"pad":"`), append([]byte(strings.Repeat("a", 64<<10)), []byte(`"}`)...)...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := NormalizeShopRoomLayout(tc.body)
			if err == nil {
				t.Fatalf("expected rejection, got %s", out)
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
				t.Fatalf("want bad request, got %v", err)
			}
		})
	}
}

func TestNormalizeShopRoomLayout_Accepts(t *testing.T) {
	// Exact boundary values per field prove the ranges are inclusive.
	boundary := func(m map[string]any) {
		m["seeds"] = map[string]any{"room": 1000000000, "goods": 1, "deco": 500}
		m["items"] = map[string]any{"2147483647": map[string]any{"icon": "", "color": ""}, "1": map[string]any{"icon": "bow", "color": "honey"}}
		m["portrait"] = nil
		m["lines"] = []any{strings.Repeat("é", 200)}
		m["pieces"] = []any{map[string]any{"id": 100000, "kind": "lamp", "wall": "X", "x": -1, "y": 20, "off": 0, "len": 0, "w": 20, "d": -1, "pinned": false}}
	}
	cases := []struct {
		name string
		body []byte
		want func(t *testing.T, l ShopRoomLayout)
	}{
		{"good round trips normalized", mutate(func(map[string]any) {}), func(t *testing.T, l ShopRoomLayout) {
			if len(l.Lines) != 1 || l.Lines[0] != "Welcome" {
				t.Errorf("lines not trimmed/dropped: %q", l.Lines)
			}
			if l.Items["12"].Icon != "hammer" || l.Pieces[0].X != 2.5 || l.Portrait == nil || l.Portrait.Left != 10.5 {
				t.Errorf("fields changed: %+v", l)
			}
		}},
		{"paper look kept", mutate(func(m map[string]any) { m["look"] = "paper" }), func(t *testing.T, l ShopRoomLayout) {
			if l.Look != "paper" {
				t.Errorf("look = %q, want paper", l.Look)
			}
		}},
		{"counter look kept", mutate(func(m map[string]any) { m["look"] = "counter" }), func(t *testing.T, l ShopRoomLayout) {
			if l.Look != "counter" {
				t.Errorf("look = %q, want counter", l.Look)
			}
		}},
		{"no look reads as the lit room", mutate(func(m map[string]any) { delete(m, "look") }), func(t *testing.T, l ShopRoomLayout) {
			if l.Look != "" {
				t.Errorf("look = %q, want empty", l.Look)
			}
		}},
		{"mood and effects kept", mutate(func(m map[string]any) {
			m["mood"] = "eldritch"
			m["effects"] = map[string]any{"shadows": 0, "warmth": 250, "window": 100, "haze": 1, "vignette": 249,
				"dust": true, "flicker": false, "embers": true, "smoke": false, "weather": "rain", "time": "dusk"}
		}), func(t *testing.T, l ShopRoomLayout) {
			want := ShopRoomEffects{Warmth: 250, Window: 100, Haze: 1, Vignette: 249, Dust: true, Embers: true, Weather: "rain", Time: "dusk"}
			if l.Mood != "eldritch" || l.Effects == nil || *l.Effects != want {
				t.Errorf("mood/effects: %q %+v", l.Mood, l.Effects)
			}
		}},
		{"older layout has no mood or effects", mutate(func(m map[string]any) { delete(m, "mood"); delete(m, "effects") }), func(t *testing.T, l ShopRoomLayout) {
			if l.Mood != "" || l.Effects != nil {
				t.Errorf("mood/effects: %q %+v", l.Mood, l.Effects)
			}
		}},
		{"effects left out keep the default look", mutate(func(m map[string]any) { m["effects"] = map[string]any{"smoke": true} }), func(t *testing.T, l ShopRoomLayout) {
			want := ShopRoomEffects{Shadows: 100, Warmth: 100, Window: 100, Haze: 100, Vignette: 100, Dust: true, Flicker: true, Embers: true, Smoke: true}
			if l.Effects == nil || *l.Effects != want {
				t.Errorf("effects = %+v, want %+v", l.Effects, want)
			}
		}},
		{"mood furniture kinds accepted", mutate(func(m map[string]any) {
			var ps []any
			for i, k := range []string{"tentacle", "monolith", "circle", "cane", "gumdrop", "lolly", "ghost", "coffin", "gift", "pine", "coral", "kelp", "mushroom", "bloom", "urn", "palm", "pillar", "brazier"} {
				ps = append(ps, map[string]any{"id": i + 1, "kind": k, "wall": "", "x": 1, "y": 1, "off": 0, "len": 1, "w": 1, "d": 1, "pinned": false})
			}
			m["pieces"] = ps
		}), func(t *testing.T, l ShopRoomLayout) {
			if len(l.Pieces) != 18 {
				t.Errorf("pieces: %d", len(l.Pieces))
			}
		}},
		{"boundaries", mutate(boundary), func(t *testing.T, l ShopRoomLayout) {
			if l.Portrait != nil || len(l.Items) != 2 {
				t.Errorf("unexpected: %+v", l)
			}
		}},
		{"hand-placed decorations kept", mutate(func(m map[string]any) {
			m["decor"] = []any{map[string]any{"piece": 1, "spot": 63, "icon": "gem"}}
		}), func(t *testing.T, l ShopRoomLayout) {
			if len(l.Decor) != 1 || l.Decor[0] != (ShopRoomDecor{Piece: 1, Spot: 63, Icon: "gem"}) {
				t.Errorf("decor: %+v", l.Decor)
			}
		}},
		{"manner and moments kept, emptied moments dropped", mutate(func(m map[string]any) {
			m["manner"] = "gruff"
			m["moments"] = map[string]any{"hello": []any{"  Well?  ", ""}, "bye": []any{"   "}, "poor": []any{"a", "b", "c", "d", strings.Repeat("é", 200)}}
		}), func(t *testing.T, l ShopRoomLayout) {
			if l.Manner != "gruff" || len(l.Moments) != 2 || l.Moments["hello"][0] != "Well?" || len(l.Moments["poor"]) != 5 {
				t.Errorf("manner/moments: %q %q", l.Manner, l.Moments)
			}
		}},
		{"no manner or moments stays absent", mutate(func(m map[string]any) { m["moments"] = map[string]any{"look": []any{""}} }), func(t *testing.T, l ShopRoomLayout) {
			if l.Manner != "" || l.Moments != nil {
				t.Errorf("manner/moments: %q %q", l.Manner, l.Moments)
			}
		}},
		{"keeper spot kept", mutate(func(m map[string]any) { m["keeperSpot"] = "left" }), func(t *testing.T, l ShopRoomLayout) {
			if l.KeeperSpot != "left" {
				t.Errorf("keeperSpot: %q", l.KeeperSpot)
			}
		}},
		{"unknown fields are dropped", mutate(func(m map[string]any) { m["extra"] = "<script>" }), func(t *testing.T, l ShopRoomLayout) {}},
		{"empty collections become empty not null", mutate(func(m map[string]any) {
			delete(m, "pieces")
			delete(m, "items")
			delete(m, "lines")
			delete(m, "portrait")
		}), func(t *testing.T, l ShopRoomLayout) {
			if l.Pieces == nil || l.Items == nil || l.Lines == nil || l.Decor == nil {
				t.Errorf("nil collection: %+v", l)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := NormalizeShopRoomLayout(tc.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(string(out), "extra") || strings.Contains(string(out), `"pieces":null`) ||
				strings.Contains(string(out), `"items":null`) || strings.Contains(string(out), `"lines":null`) {
				t.Errorf("not normalized: %s", out)
			}
			var l ShopRoomLayout
			if err := json.Unmarshal(out, &l); err != nil {
				t.Fatal(err)
			}
			tc.want(t, l)
			// Normalizing the normalized form is a no-op.
			again, err := NormalizeShopRoomLayout(out)
			if err != nil || string(again) != string(out) {
				t.Errorf("not idempotent: %v %s", err, again)
			}
		})
	}
}

func TestShopRoomIconsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, n := range shopRoomIcons {
		if seen[n] || n == "" {
			t.Errorf("duplicate or empty icon %q", n)
		}
		seen[n] = true
	}
}

// goodCounter is a valid counter theme as a generic map, so each case can
// break one field.
func goodCounter() map[string]any {
	return map[string]any{
		"name": "  The Smithy ", "version": 1,
		"skeleton": map[string]any{"shape": "square", "opening": "window-right", "display": "pegboard", "counter": "straight"},
		"theme": map[string]any{"wall": "stone", "wood": "#4A3220", "cloth": "#5a3a22", "accent": "#ff9a50", "trim": "#9aa0a8",
			"light": "forge", "warmth": 0.9, "outside": "dusk"},
		"props": []any{"anvil", "sparks", "anvil"},
	}
}

func withCounter(f func(c map[string]any)) []byte {
	return mutate(func(m map[string]any) {
		c := goodCounter()
		f(c)
		m["counter"] = c
	})
}

func counterField(group, k string, v any) func(c map[string]any) {
	return func(c map[string]any) { c[group].(map[string]any)[k] = v }
}

func TestNormalizeShopRoomLayout_CounterTheme(t *testing.T) {
	rejects := []struct {
		name string
		body []byte
	}{
		{"bad version", withCounter(func(c map[string]any) { c["version"] = 2 })},
		{"name too long", withCounter(func(c map[string]any) { c["name"] = strings.Repeat("é", maxShopRoomCounterName+1) })},
		{"unknown shape", withCounter(counterField("skeleton", "shape", "hexagon"))},
		{"empty shape", withCounter(counterField("skeleton", "shape", ""))},
		{"unknown opening", withCounter(counterField("skeleton", "opening", "skylight"))},
		{"unknown display", withCounter(counterField("skeleton", "display", "racks"))},
		{"unknown counter", withCounter(counterField("skeleton", "counter", "bar"))},
		{"unknown wall", withCounter(counterField("theme", "wall", "marble"))},
		{"unknown light", withCounter(counterField("theme", "light", "neon"))},
		{"unknown outside", withCounter(counterField("theme", "outside", "snow"))},
		{"colour without hash", withCounter(counterField("theme", "wood", "4a3220"))},
		{"colour too short", withCounter(counterField("theme", "cloth", "#fff"))},
		{"colour not hex", withCounter(counterField("theme", "accent", "#gggggg"))},
		{"colour injection", withCounter(counterField("theme", "trim", `#000000" onload="x`))},
		{"unknown prop", withCounter(func(c map[string]any) { c["props"] = []any{"anvil", "dragon"} })},
		{"too many props", withCounter(func(c map[string]any) {
			ps := make([]any, maxShopRoomCounterProps+1)
			for i := range ps {
				ps[i] = "anvil"
			}
			c["props"] = ps
		})},
		{"counter is not an object", mutate(func(m map[string]any) { m["counter"] = "plain" })},
	}
	for _, tc := range rejects {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			out, err := NormalizeShopRoomLayout(tc.body)
			if err == nil {
				t.Fatalf("expected rejection, got %s", out)
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusBadRequest {
				t.Fatalf("want bad request, got %v", err)
			}
		})
	}

	accepts := []struct {
		name string
		body []byte
		want func(t *testing.T, c *ShopRoomCounter)
	}{
		{"canonical form", withCounter(func(map[string]any) {}), func(t *testing.T, c *ShopRoomCounter) {
			if c.Name != "The Smithy" || c.Theme.Wood != "#4a3220" {
				t.Errorf("name or colour not canonical: %q %q", c.Name, c.Theme.Wood)
			}
			if len(c.Props) != 2 || c.Props[0] != "anvil" || c.Props[1] != "sparks" {
				t.Errorf("props not de-duplicated in order: %v", c.Props)
			}
		}},
		{"warmth clamped high", withCounter(counterField("theme", "warmth", 7)), func(t *testing.T, c *ShopRoomCounter) {
			if c.Theme.Warmth != 1 {
				t.Errorf("warmth = %v", c.Theme.Warmth)
			}
		}},
		{"warmth clamped low", withCounter(counterField("theme", "warmth", -3)), func(t *testing.T, c *ShopRoomCounter) {
			if c.Theme.Warmth != 0 {
				t.Errorf("warmth = %v", c.Theme.Warmth)
			}
		}},
		{"no props becomes an empty list", withCounter(func(c map[string]any) { delete(c, "props") }), func(t *testing.T, c *ShopRoomCounter) {
			if c.Props == nil || len(c.Props) != 0 {
				t.Errorf("props = %#v", c.Props)
			}
		}},
		{"every option accepted", withCounter(func(c map[string]any) {
			c["skeleton"] = map[string]any{"shape": "open-air", "opening": "round-window", "display": "domes", "counter": "stall-table"}
			c["theme"] = map[string]any{"wall": "night-sky", "wood": "#000000", "cloth": "#FFFFFF", "accent": "#0a0B0c", "trim": "#123456", "light": "moonlight", "warmth": 0, "outside": "rain"}
			c["props"] = []any{"anvil", "alembic", "jars", "crystal", "herbs", "floating-books", "orrery", "ladder", "banner", "sconce", "sparks", "lanterns", "rain", "steam-cart", "motes"}
		}), func(t *testing.T, c *ShopRoomCounter) {
			if len(c.Props) != 15 || c.Theme.Cloth != "#ffffff" {
				t.Errorf("unexpected: %+v", c)
			}
		}},
	}
	for _, tc := range accepts {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			out, err := NormalizeShopRoomLayout(tc.body)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var l ShopRoomLayout
			if err := json.Unmarshal(out, &l); err != nil || l.Counter == nil {
				t.Fatalf("counter lost: %v %s", err, out)
			}
			tc.want(t, l.Counter)
			again, err := NormalizeShopRoomLayout(out)
			if err != nil || string(again) != string(out) {
				t.Errorf("not idempotent: %v %s", err, again)
			}
		})
	}

	t.Run("no counter stays absent", func(t *testing.T) {
		out, err := NormalizeShopRoomLayout(mutate(func(map[string]any) {}))
		if err != nil || strings.Contains(string(out), `"counter"`) {
			t.Errorf("counter written for a layout without one: %v %s", err, out)
		}
	})
	t.Run("null counter reads as the starter", func(t *testing.T) {
		out, err := NormalizeShopRoomLayout(mutate(func(m map[string]any) { m["counter"] = nil }))
		if err != nil || strings.Contains(string(out), `"counter"`) {
			t.Errorf("null counter stored: %v %s", err, out)
		}
	})
}

func TestNormalizeShopRoomCounter(t *testing.T) {
	starter := ShopRoomCounterStarter("forge")
	tests := []struct {
		name    string
		raw     func() []byte
		wantErr bool
	}{
		{"a starter theme is accepted", func() []byte { return starter }, false},
		{"garbage is refused", func() []byte { return []byte("not json") }, true},
		{"trailing data is refused", func() []byte { return append(append([]byte{}, starter...), []byte(` {}`)...) }, true},
		{"an unknown prop is refused", func() []byte {
			var m map[string]any
			_ = json.Unmarshal(starter, &m)
			m["props"] = []string{"no-such-prop"}
			b, _ := json.Marshal(m)
			return b
		}, true},
		{"too large is refused", func() []byte { return []byte(strings.Repeat(" ", maxShopRoomBytes+1)) }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := NormalizeShopRoomCounter(tt.raw())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && !json.Valid(out) {
				t.Errorf("output is not JSON: %s", out)
			}
		})
	}
}

func withRolls(r any) []byte {
	return mutate(func(m map[string]any) { m["rolls"] = r })
}

func TestNormalizeShopRoomLayout_Rolls(t *testing.T) {
	good := func() map[string]any {
		return map[string]any{"haggle": true, "look": true, "stubborn": "hard", "haggleStat": "cha", "lookStat": "int_2",
			"great": 25, "middling": 12, "poor": 15}
	}
	with := func(k string, v any) []byte {
		r := good()
		r[k] = v
		return withRolls(r)
	}
	rejects := []struct {
		name string
		body []byte
	}{
		{"unknown stubborn", with("stubborn", "tough")},
		{"stubborn in capitals", with("stubborn", "Hard")},
		{"stat key with a capital", with("haggleStat", "Cha")},
		{"stat key with a space", with("lookStat", "my stat")},
		{"stat key with a hyphen", with("haggleStat", "a-b")},
		{"stat key too long", with("haggleStat", strings.Repeat("a", 41))},
		{"great over 90", with("great", 91)},
		{"great negative", with("great", -1)},
		{"middling over 90", with("middling", 100)},
		{"poor over 200", with("poor", 201)},
		{"poor negative", with("poor", -5)},
		{"fractional percent", with("great", 20.5)},
		{"percent as text", with("great", "20")},
		{"switch as text", with("haggle", "yes")},
		{"rolls not an object", withRolls("on")},
	}
	for _, tc := range rejects {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			if _, err := NormalizeShopRoomLayout(tc.body); err == nil {
				t.Error("expected an error")
			}
		})
	}

	t.Run("a full block is kept", func(t *testing.T) {
		out, err := NormalizeShopRoomLayout(withRolls(good()))
		if err != nil {
			t.Fatal(err)
		}
		var l ShopRoomLayout
		if err := json.Unmarshal(out, &l); err != nil {
			t.Fatal(err)
		}
		want := ShopRoomRolls{Haggle: true, Look: true, Stubborn: "hard", HaggleStat: "cha", LookStat: "int_2", Great: 25, Middling: 12, Poor: 15}
		if l.Rolls == nil || *l.Rolls != want {
			t.Errorf("rolls = %+v", l.Rolls)
		}
		again, err := NormalizeShopRoomLayout(out)
		if err != nil || string(again) != string(out) {
			t.Errorf("not idempotent: %v %s", err, again)
		}
	})
	t.Run("the edges are accepted", func(t *testing.T) {
		r := good()
		r["great"], r["middling"], r["poor"], r["haggleStat"], r["lookStat"] = 90, 0, 200, "", strings.Repeat("z", 40)
		if _, err := NormalizeShopRoomLayout(withRolls(r)); err != nil {
			t.Error(err)
		}
	})
	t.Run("left-out numbers keep their defaults and the switches stay off", func(t *testing.T) {
		out, err := NormalizeShopRoomLayout(withRolls(map[string]any{}))
		if err != nil {
			t.Fatal(err)
		}
		var l ShopRoomLayout
		_ = json.Unmarshal(out, &l)
		want := ShopRoomRolls{Stubborn: "usual", Great: 20, Middling: 10, Poor: 10}
		if l.Rolls == nil || *l.Rolls != want {
			t.Errorf("rolls = %+v", l.Rolls)
		}
	})
	t.Run("absent rolls stays absent", func(t *testing.T) {
		out, err := NormalizeShopRoomLayout(mutate(func(map[string]any) {}))
		if err != nil || strings.Contains(string(out), `"rolls"`) {
			t.Errorf("rolls written for a layout without any: %v %s", err, out)
		}
	})
}
