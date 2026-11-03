package connector

import (
	"context"
	"strings"
	"testing"
)

// longGuide is the copy-and-paste prompt's shape, padded past the length
// answered whole: an opening, the entity types, the page format, the block
// formats and a closing section.
func longGuide() string {
	pad := strings.Repeat("Words about writing pages. ", 1000)
	return "You are helping me.\n\n## What you can do\n\n- a page\n- `kind: event`: events\n\n" +
		"## My campaign's entity types\n\n\n" +
		"- **Character** (slug: `character`) — preset: character\n  - `level` (Level): a number\n  - `title` (Title): text\n\n" +
		"- **Creature** (slug: `creature`)\n  - `stamina` (Stamina): a number\n  - `abilities_json` (Abilities Data): text\n  - `ev` (EV): a number\n\n" +
		"- **Lore** (slug: `lore`)\n\n\n" +
		"A page's sheet is set with a `fields:` map. To build a monster, give it every stat block field.\n\n\n" +
		"## Format your output as markdown with YAML front-matter\n\n" + pad + "\n\n" +
		"## Other things you can add, change or remove\n\nGive it\n`kind:` (below) and `action:`.\n\n" +
		"### kind: event (Calendar event)\n\nEVENT FORMAT " + pad + "\n\n" +
		"## How to answer my requests\n\nAnswer with blocks.\n"
}

func TestHowToPart(t *testing.T) {
	g := longGuide()
	tests := []struct {
		name, part, category string
		has, lacks           []string
		bad                  string
	}{
		{name: "guide", has: []string{"## What you can do", "- Character (2 fields)", "- Creature (3 fields)", "- Lore (0 fields)",
			"`category` set to the category's name", "block_format", "## How to answer my requests", "Words about writing pages"},
			lacks: []string{"EVENT FORMAT", "`stamina`", "(below)"}},
		{name: "one category", category: "creature", has: []string{"`stamina`", "`ev` (EV)", "To build a monster"},
			lacks: []string{"`level`", "Words about", "EVENT FORMAT"}},
		{name: "every category", part: "Categories", has: []string{"`level`", "`stamina`", "To build a monster"}, lacks: []string{"Words about"}},
		{name: "unknown category", category: "Vehicles", bad: "the categories are Character, Creature, Lore"},
		{name: "unknown part", part: "blocks", bad: "there is no part"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, ok, why := howToPart(g, tc.part, tc.category)
			if tc.bad != "" {
				if ok || !strings.Contains(why, tc.bad) {
					t.Fatalf("ok %v, why %q", ok, why)
				}
				return
			}
			if !ok {
				t.Fatalf("refused: %s", why)
			}
			for _, h := range tc.has {
				if !strings.Contains(out, h) {
					t.Errorf("missing %q", h)
				}
			}
			for _, l := range tc.lacks {
				if strings.Contains(out, l) {
					t.Errorf("should not hold %q", l)
				}
			}
		})
	}
	// The block formats (half the padding) leave the guide.
	if out, _, _ := howToPart(g, "", ""); len(out) > len(g)-25000 {
		t.Errorf("the guide is %d of %d characters", len(out), len(g))
	}
	short := "## What you can do\n\n## My campaign's entity types\n\n- **Hero** (slug: `hero`)\n  - `level` (Level): a number\n"
	if out, _, _ := howToPart(short, "", ""); out != short {
		t.Error("a short guide was split")
	}
}

// Over the connector the guide carries the connection's preface and a
// category's fields come alone.
func TestHandle_HowToWriteChangesParts(t *testing.T) {
	s, sess, _ := askRig(t)
	s.Instructions = func(context.Context, *Connection, string) (string, error) { return longGuide(), nil }
	tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolHowTo, nil)))
	if tr.IsError || !strings.Contains(tr.Content[0].Text, "send them with "+toolAsk) || strings.Contains(tr.Content[0].Text, "EVENT FORMAT") {
		t.Fatalf("guide = %+v", tr)
	}
	out := s.Handle(context.Background(), sess, callBody(toolHowTo, map[string]any{"category": "Creature"}))
	tr = toolText(t, out)
	if tr.IsError || strings.Contains(tr.Content[0].Text, "connected to Chronicle") || !strings.Contains(tr.Content[0].Text, "`stamina`") {
		t.Fatalf("category = %+v", tr)
	}
	if out.Called == nil || out.Called.Summary != "Read the sheet fields of Creature" {
		t.Fatalf("logged %+v", out.Called)
	}
	if tr := toolText(t, s.Handle(context.Background(), sess, callBody(toolHowTo, map[string]any{"category": "Vehicles"}))); !tr.IsError {
		t.Fatalf("unknown category answered: %+v", tr)
	}
}
