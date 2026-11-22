package connector

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// Every lookup tool takes `piece`, so a long answer or list can always be read
// on; the change tools do not, and `search` is on a player's key as well as an
// owner's.
func TestTools_PieceAndSearch(t *testing.T) {
	owner := &Connection{CanAsk: true, Privacy: aiexport.PrivacyModePermitted}
	player := &Connection{ForPlayer: true}

	for _, name := range []string{"read_page", "list_pages", "map_pins", "house_rules", "shop_stock", "inventory", "rolling_tables", "search", "block_format"} {
		if tl := findTool(owner, nil, name); tl == nil || !tl.takes("piece") {
			t.Errorf("%s does not take piece", name)
		}
	}
	for _, name := range []string{toolAsk, toolHowTo, "campaign_outline"} {
		if tl := findTool(owner, nil, name); tl == nil || tl.takes("piece") {
			t.Errorf("%s should not take piece", name)
		}
	}

	for _, c := range []*Connection{owner, player} {
		tl := findTool(c, nil, "search")
		if tl == nil {
			t.Fatalf("search is not listed (player %v)", c.ForPlayer)
		}
		got, err := tl.readArgs(map[string]any{"query": "dragon", "piece": float64(2)})
		if err != nil || got["query"] != "dragon" || got["piece"] != "2" {
			t.Errorf("args = %v, %v", got, err)
		}
		if _, err := tl.readArgs(map[string]any{"piece": "2"}); err == nil {
			t.Errorf("search without a query was accepted")
		}
		if _, err := tl.readArgs(map[string]any{"query": "x", "name": "y"}); err == nil {
			t.Errorf("an argument search never declared was accepted")
		}
	}

	// The one-argument summary stays short with piece around.
	rp := findTool(owner, nil, "rolling_tables")
	if got := rp.summary(map[string]string{"name": "Loot"}); got != "Read rolling tables: “Loot”" {
		t.Errorf("summary = %q", got)
	}
	if got := rp.summary(map[string]string{"name": "Loot", "piece": "2"}); got != "Read rolling tables: “Loot”, piece “2”" {
		t.Errorf("summary with piece = %q", got)
	}
}
