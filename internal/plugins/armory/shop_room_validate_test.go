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
		{"no look reads as the lit room", mutate(func(m map[string]any) { delete(m, "look") }), func(t *testing.T, l ShopRoomLayout) {
			if l.Look != "" {
				t.Errorf("look = %q, want empty", l.Look)
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
