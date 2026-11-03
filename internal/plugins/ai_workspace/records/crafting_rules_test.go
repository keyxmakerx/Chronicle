package records

import (
	"context"
	"strings"
	"testing"
)

func TestRecipeKind_Rules(t *testing.T) {
	lantern := map[string]any{"when": []any{map[string]any{"kind": "moon", "moon": "The Lantern", "phase": "full"}}, "then": map[string]any{"kind": "edge"}, "players": "book"}
	tide := map[string]any{"when": map[string]any{"kind": "event", "event": "Low tide"}, "then": map[string]any{"kind": "points", "amount": 2}}
	tests := []struct {
		name, want, err string
		rules           any
	}{
		{"one sent back, one new, one left out", "; 1 rule kept unchanged, 1 new rule waiting for your approval in Crafting, 1 rule removed", "", []any{lantern, tide}},
		{"none clears them", "; 2 rules removed", "", "none"},
		{"an empty list clears them too", "; 2 rules removed", "", []any{}},
		{"one rule on its own reads as a list", "; 1 new rule waiting for your approval in Crafting, 2 rules removed", "", tide},
		{"an empty value is refused", "", "rules: write a list", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeCrafting()
			r := craftRec("recipe", ActionUpdate, "Healing potion", map[string]any{"rules": tt.rules})
			p := RecipeKind{Svc: f}.Plan(context.Background(), camp, owner, r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || !strings.HasSuffix(p.Summary, tt.want) {
				t.Fatalf("plan = %+v, want it to end %q", p, tt.want)
			}
		})
	}

	// The service's refusal is the row's, and Apply hands the list on.
	f := newFakeCrafting()
	r := craftRec("recipe", ActionUpdate, "Healing potion", map[string]any{"rules": []any{tide}})
	if err := (RecipeKind{Svc: f}).Apply(context.Background(), camp, owner, r); err != nil || len(f.saved) != 1 || !strings.Contains(string(f.saved[0].Rules), `"Low tide"`) {
		t.Fatalf("apply: %v, %+v", err, f.saved)
	}
	f.refuse = "rule 1: `then` needs a `kind`"
	if p := (RecipeKind{Svc: f}).Plan(context.Background(), camp, owner, r); !strings.Contains(p.Error, "rule 1") {
		t.Fatalf("refusal: %+v", p)
	}
}

func TestBenchKind(t *testing.T) {
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		err     string
	}{
		{"a new bench takes the only kind", craftRec("bench", ActionCreate, "Still room", nil), owner, `a new bench: "Still room", apothecary bench; arrange it in Crafting's build mode`, ""},
		{"a kind by its words", craftRec("bench", ActionCreate, "Still room", map[string]any{"kind_of_bench": "Apothecary bench"}), owner, `a new bench: "Still room", apothecary bench; arrange it in Crafting's build mode`, ""},
		{"a rename keeps the arrangement", craftRec("bench", ActionUpdate, "mira's kitchen", map[string]any{"rename_to": "Mira's still room"}), owner, `renamed "Mira's still room", apothecary bench; its arrangement and upgrades stay`, ""},
		{"a taken name", craftRec("bench", ActionCreate, "MIRA'S KITCHEN", nil), owner, "", "already a bench called"},
		{"no such bench", craftRec("bench", ActionUpdate, "Forge", nil), owner, "", `no bench called "Forge"`},
		{"an unknown kind", craftRec("bench", ActionCreate, "Forge", map[string]any{"kind_of_bench": "forge"}), owner, "", `"forge" is not a kind of bench; the kinds are apothecary`},
		{"removing is the app's", craftRec("bench", ActionDelete, "Mira's kitchen", nil), owner, "", "removed in Crafting"},
		{"a player can't", craftRec("bench", ActionCreate, "Still room", nil), player, "", "only the campaign's Directors"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BenchKind{Svc: newFakeCrafting()}.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary {
				t.Fatalf("plan = %+v, want %q", p, tt.summary)
			}
		})
	}

	f := newFakeCrafting()
	k := BenchKind{Svc: f}
	if err := k.Apply(context.Background(), camp, owner, craftRec("bench", ActionUpdate, "Mira's kitchen", map[string]any{"rename_to": "Still room"})); err != nil {
		t.Fatal(err)
	}
	if len(f.benches) != 1 || f.benches[0] != (CraftBench{ID: "b1", Name: "Still room", Kind: "apothecary", KindLabel: "Apothecary bench"}) {
		t.Fatalf("saved %+v", f.benches)
	}
	if out, _ := k.Export(context.Background(), camp, owner); out != "---\nkind: bench\nname: \"Mira's kitchen\"\nkind_of_bench: apothecary\n---\n" {
		t.Fatalf("export %q", out)
	}
}
