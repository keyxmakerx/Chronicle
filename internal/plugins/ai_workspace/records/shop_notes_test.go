package records

import (
	"context"
	"os"
	"strings"
	"testing"
)

type fakePay bool

func (f fakePay) PaysByWealth(context.Context, string) bool { return bool(f) }

// A Wealth game reads the price as a level and warns on what is ignored or
// out of reach; a price of 0 is flagged in any game.
func TestShopStockPriceNotes(t *testing.T) {
	pages := shopPages()
	rope := page("rope", "Rope", "Item", false)
	pages[rope.Slug] = &rope
	tests := []struct {
		name    string
		wealth  bool
		fields  map[string]any
		summary string
		warn    []string
	}{
		{"coins keep the coin line", false, map[string]any{"price": 25, "currency": "gp"}, "25 gp", nil},
		{"wealth reads a level", true, map[string]any{"price": 2}, "Wealth 2", nil},
		{"wealth ignores currency", true, map[string]any{"price": 2, "currency": "gp"}, "Wealth 2", []string{"`currency` is ignored"}},
		{"wealth far too high", true, map[string]any{"price": 25}, "Wealth 25", []string{"far above the Wealth levels"}},
		{"wealth at the bound is fine", true, map[string]any{"price": maxLikelyWealth}, "Wealth 10", nil},
		{"zero in coins", false, map[string]any{"price": 0}, "0 gp", []string{"No price"}},
		{"zero in wealth", true, map[string]any{"price": 0}, "Wealth 0", []string{"No price"}},
		{"high coin price is no warning", false, map[string]any{"price": 500}, "500 gp", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := ShopStockKind(pages, &writeRels{}, fakePay(tt.wealth))
			tt.fields["shop"], tt.fields["item"] = "The Rusty Anchor", "Rope"
			p := k.Plan(context.Background(), camp, owner, craftRec("shop-stock", ActionCreate, "", tt.fields))
			if p.Error != "" || p.Summary != tt.summary || len(p.Warnings) != len(tt.warn) {
				t.Fatalf("plan = %+v, want %q with %d warnings", p, tt.summary, len(tt.warn))
			}
			for i, w := range tt.warn {
				if !strings.Contains(p.Warnings[i], w) {
					t.Errorf("warning %d = %q, want %q", i, p.Warnings[i], w)
				}
			}
		})
	}
}

// The kind's format doc explains Wealth prices.
func TestShopStockDocMentionsWealth(t *testing.T) {
	if d := ShopStockKind(nil, nil, nil).Doc(); !strings.Contains(d, "Wealth level a hero needs") {
		t.Errorf("doc says nothing about Wealth prices: %s", d)
	}
}

// While paper is paused, a paper look or a paper-only key still saves but
// carries a plain-words warning.
func TestShopRoomPaperWarning(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		warn   string
	}{
		{"paper look", map[string]any{"look": "paper"}, "(look)"},
		{"counter look is quiet", map[string]any{"look": "counter"}, ""},
		{"paper-only keys", map[string]any{"setting": "tower", "palette": "moss"}, "(setting, palette)"},
		{"mood", map[string]any{"mood": "dwarven"}, "(mood)"},
		{"type and manner are quiet", map[string]any{"type": "market", "manner": "warm"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, _, _ := shopKinds()
			tt.fields["shop"] = "Ember Forge"
			p := k.Plan(context.Background(), camp, owner, craftRec("shop-room", ActionUpdate, "", tt.fields))
			if p.Error != "" {
				t.Fatalf("refused: %s", p.Error)
			}
			var got string
			for _, w := range p.Warnings {
				if strings.HasPrefix(w, "Saved, but nothing changes on screen") {
					got = w
				}
			}
			if (tt.warn == "") != (got == "") || !strings.Contains(got, tt.warn) {
				t.Errorf("warnings = %q, want one containing %q", p.Warnings, tt.warn)
			}
		})
	}
}

// The Go constant must follow the widget's own switch.
func TestPaperLookPausedMatchesWidget(t *testing.T) {
	b, err := os.ReadFile("../../../../static/js/widgets/shop_room_paper.js")
	if err != nil {
		t.Skipf("widget not reachable: %v", err)
	}
	if got := strings.Contains(string(b), "building: true"); got != paperLookPaused {
		t.Errorf("paperLookPaused = %v but the widget's building flag says %v", paperLookPaused, got)
	}
}
