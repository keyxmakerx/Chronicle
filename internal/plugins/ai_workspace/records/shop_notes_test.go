package records

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

type fakePay bool

func (f fakePay) PaysByWealth(context.Context, string) bool { return bool(f) }

// limitPay adds a price ceiling like the Armory's, to test the plan refuses it.
type limitPay struct{ fakePay }

func (limitPay) CheckPrice(p float64) error {
	if p < 0 || p > 10_000_000 {
		return apperror.NewBadRequest("A price that large isn't allowed.")
	}
	return nil
}

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
	const path = "../../../../static/js/widgets/shop_room_paper.js"
	b, err := os.ReadFile(path)
	if err != nil {
		// A skip here would let the pin pass without ever running.
		t.Fatalf("widget not readable at %s (relative to this package): %v", path, err)
	}
	if got := strings.Contains(string(b), "building: true"); got != paperLookPaused {
		t.Errorf("paperLookPaused = %v but the widget's building flag says %v", paperLookPaused, got)
	}
}

// A price the Armory would refuse is a plan problem, not an apply failure;
// the edges of the allowed range still plan.
func TestShopStockPriceBounds(t *testing.T) {
	pages := shopPages()
	rope := page("rope", "Rope", "Item", false)
	pages[rope.Slug] = &rope
	tests := []struct {
		name    string
		price   float64
		action  string
		wantErr bool
	}{
		{"zero is allowed", 0, ActionCreate, false},
		{"top of the range", 10_000_000, ActionCreate, false},
		{"above the range", 10_000_001, ActionCreate, true},
		{"absurdly large", 1e18, ActionCreate, true},
		{"negative", -1, ActionCreate, true},
		{"above the range on update", 1e18, ActionUpdate, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := ShopStockKind(pages, &writeRels{}, limitPay{})
			fields := map[string]any{"shop": "The Rusty Anchor", "item": "Rope", "price": tt.price}
			p := k.Plan(context.Background(), camp, owner, craftRec("shop-stock", tt.action, "", fields))
			if tt.wantErr && (!strings.Contains(p.Error, "price") || strings.Contains(p.Error, "bad_request")) {
				t.Fatalf("plan = %+v, want a price problem in plain words", p)
			}
			if !tt.wantErr && strings.Contains(p.Error, "price") {
				t.Fatalf("plan = %+v, want no price problem", p)
			}
		})
	}
}

// The stock answer names a price as the Wealth level it is in a game that
// pays by Wealth, and as coins in any other.
func TestStockLookupNamesWealthPrices(t *testing.T) {
	pages := []entities.Entity{}
	for _, e := range shopPages() {
		pages = append(pages, *e)
	}
	rels := &fakeRels{rels: []relations.Relation{
		{ID: 1, SourceEntityID: "forge", TargetEntityID: "inn", TargetEntityName: "Nails", RelationType: "sells", Metadata: json.RawMessage(`{"price":3,"currency":"gp","quantity":2}`)},
	}}
	tests := []struct {
		name          string
		pay           ShopPayAPI
		want, wantNot string
	}{
		{"wealth game", fakePay(true), "- Nails: Wealth 3, 2 in stock", "gp"},
		{"coin game", fakePay(false), "- Nails: 3 gp, 2 in stock", "Wealth"},
		{"no pay wired", nil, "- Nails: 3 gp, 2 in stock", "Wealth"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &Lookups{Pages: &fakePages{all: pages}, Rels: rels, Pay: tc.pay}
			got := l.Answer(context.Background(), camp, owner, []Record{lookup("stock", map[string]any{"shop": "Ember Forge"})})[0]
			if got.Error != "" || !strings.Contains(got.Text, tc.want) || strings.Contains(got.Text, tc.wantNot) {
				t.Fatalf("stock answer:\n%s\n(error %q)", got.Text, got.Error)
			}
		})
	}
}
