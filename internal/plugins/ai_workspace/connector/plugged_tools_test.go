package connector

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// plugLookup is a feature's lookup, registered from outside the engine.
type plugLookup struct {
	owner bool
	got   aiplug.Record
}

func (p *plugLookup) LookupInfo() aiplug.LookupInfo {
	return aiplug.LookupInfo{What: "widgets", Keys: "optional `name`", Brief: "the widgets", Tool: aiplug.Tool{
		Name: "widgets", Title: "Widgets", Did: "Listed the widgets", Desc: "The widgets.", Owner: p.owner,
		Args: []aiplug.Arg{{Key: "name", Desc: "Part of a widget's name"}},
	}}
}

func (p *plugLookup) Answer(_ context.Context, _ string, _ aiplug.Actor, r aiplug.Record) (string, string, error) {
	p.got = r
	return "Widgets", "## Widgets\n\n- Cog\n", nil
}

// A plugged lookup becomes a tool, listed before block_format, gated like a
// built-in one and answered through the same lookup path.
func TestToolsFor_PluggedLookup(t *testing.T) {
	p := &plugLookup{}
	l := &records.Lookups{Plugged: []aiplug.Lookup{p}}
	list := toolsFor(&Connection{}, l)
	at, format := -1, -1
	for i, tl := range list {
		switch tl.Name {
		case "widgets":
			at = i
		case "block_format":
			format = i
		}
	}
	if at < 0 || format < 0 || at != format-1 {
		t.Fatalf("widgets at %d, block_format at %d", at, format)
	}
	if findTool(&Connection{}, nil, "widgets") != nil {
		t.Error("an unplugged server lists the tool")
	}

	tl := findTool(&Connection{}, l, "widgets")
	args, err := tl.readArgs(map[string]any{"name": "Co"})
	if err != nil {
		t.Fatal(err)
	}
	text, note, ok := tl.answer(context.Background(), l, "c", "Camp", aiplug.Actor{UserID: "u", Role: 3}, args)
	if !ok || text != "## Widgets\n\n- Cog" || note != "Widgets" || p.got.Name != "Co" {
		t.Errorf("answer = %q, %q, %v; record name %q", text, note, ok, p.got.Name)
	}

	p.owner = true
	if findTool(&Connection{ForPlayer: true}, l, "widgets") != nil {
		t.Error("a player's key lists an owner-only plugged tool")
	}
}
