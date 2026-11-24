package homebrew

import (
	"context"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

var (
	aiDM     = aiplug.Actor{UserID: "dm-1", Role: permissions.RoleOwner}
	aiPlayer = aiplug.Actor{UserID: "jo-1", Role: permissions.RolePlayer}
)

// record reads a block as the importer hands it to a kind: front matter as
// loose YAML with lower-cased top-level keys.
func record(t *testing.T, action, block string) aiplug.Record {
	t.Helper()
	body := strings.TrimSpace(block)
	body = strings.TrimPrefix(body, "---")
	body = strings.TrimSuffix(body, "---")
	fields := map[string]any{}
	if err := yaml.Unmarshal([]byte(body), &fields); err != nil {
		t.Fatalf("yaml: %v\n%s", err, body)
	}
	name, _ := fields["name"].(string)
	return aiplug.Record{Kind: "workshop-recipe", Action: action, Name: name, Fields: fields}
}

func newAIRig(t *testing.T) (*AI, Service) {
	t.Helper()
	svc, _ := newTestService(t)
	return NewAI(svc), svc
}

// The Workshop is the Director's: a player's key or Safe privacy reads
// nothing and can ask for nothing.
func TestAI_DirectorsOnly(t *testing.T) {
	ai, svc := newAIRig(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, camp, dm, in("Seed")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ai.Answer(ctx, camp, aiPlayer, aiplug.Record{}); err == nil {
		t.Error("a player read the Workshop")
	}
	if out, err := ai.Export(ctx, camp, aiPlayer); err != nil || out != "" {
		t.Errorf("player export = %q, %v", out, err)
	}
	r := record(t, "", "---\nkind: workshop-recipe\nname: New\nparts:\n  - {key: a, kind: widget, label: A, see: party}\n---")
	if p := ai.Plan(ctx, camp, aiPlayer, r); p.Error == "" {
		t.Error("a player's request planned")
	}
	if err := ai.Apply(ctx, camp, aiPlayer, r); err == nil {
		t.Error("a player's request applied")
	}
}

// The lookup shows each recipe as its block, and what a recipe can use.
func TestAI_Answer(t *testing.T) {
	ai, svc := newAIRig(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, camp, dm, in("Remnant need")); err != nil {
		t.Fatal(err)
	}
	chip, text, err := ai.Answer(ctx, camp, aiDM, aiplug.Record{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1 recipes", "kind: workshop-recipe", `name: "Remnant need"`, `"key":"a"`, `"event":"moon"`, "`widget`: Widget", "`secret`: Secret; always `see: director`", "`widget.hit`", "needs `part`, a part of kind widget", "`ping`"} {
		if !strings.Contains(chip+text, want) {
			t.Errorf("answer lacks %q:\n%s", want, text)
		}
	}
}

// A recipe read with the lookup comes back unchanged through the block, and
// a new one is made from the block's example shape.
func TestAI_RoundTripAndCreate(t *testing.T) {
	ai, svc := newAIRig(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, camp, dm, in("Seed")); err != nil {
		t.Fatal(err)
	}
	out, err := ai.Export(ctx, camp, aiDM)
	if err != nil {
		t.Fatal(err)
	}
	r := record(t, aiplug.ActionUpdate, out)
	p := ai.Plan(ctx, camp, aiDM, r)
	if p.Error != "" || len(p.Warnings) != 0 || !strings.Contains(p.Summary, "Change the recipe") {
		t.Fatalf("round trip plan: %+v", p)
	}

	newBlock := `---
kind: workshop-recipe
name: The Hunger
parts:
  - {key: count, kind: widget, label: Respites, see: director}
rules:
  - when: {event: widget.hit, part: count}
    timing: held
    then:
      - {action: bump, part: count}
      - {action: ping}
---`
	r = record(t, "", newBlock)
	p = ai.Plan(ctx, camp, aiDM, r)
	if p.Error != "" || p.Seen != "new" {
		t.Fatalf("create plan: %+v", p)
	}
	r.Seen = p.Seen
	if err := ai.Apply(ctx, camp, aiDM, r); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.List(ctx, camp, dm)
	if len(list) != 2 {
		t.Fatalf("recipes: %+v", list)
	}
	got, _ := ai.findRecipe(ctx, camp, dm, "the hunger")
	if got == nil || len(got.Doc.Rules) != 1 || len(got.Doc.Rules[0].Then) != 2 || got.Doc.Rules[0].Timing != TimingHeld {
		t.Fatalf("stored: %+v", got)
	}
	if p := ai.Plan(ctx, camp, aiDM, record(t, aiplug.ActionCreate, newBlock)); !strings.Contains(p.Error, "already a recipe") {
		t.Errorf("duplicate create: %+v", p)
	}
}

// The Workshop's own checks show on the review row, and an update that
// drops a part says so.
func TestAI_PlanRefusalsAndWarnings(t *testing.T) {
	ai, svc := newAIRig(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, camp, dm, RecipeInput{Name: "Seed", Doc: Doc{Parts: []Part{w("a"), w("b")}, Rules: []Rule{}}}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, action, block, want string
	}{
		{"owner refuses data", "", "---\nname: X\nparts:\n  - {key: p, kind: picky, label: P, see: party}\n---", "bad thing"},
		{"unknown kind", "", "---\nname: X\nparts:\n  - {key: p, kind: nope, label: P, see: party}\n---", "nope"},
		{"unknown key", "", "---\nname: X\nparts:\n  - {key: p, kind: widget, label: P, colour: red}\n---", "colour"},
		{"empty new recipe", "", "---\nname: X\n---", "needs `parts` or `rules`"},
		{"update a missing one", aiplug.ActionUpdate, "---\nname: Nope\nrules: []\n---", "no recipe called"},
		{"delete a missing one", aiplug.ActionDelete, "---\nname: Nope\n---", "no recipe called"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := ai.Plan(ctx, camp, aiDM, record(t, tc.action, tc.block))
			if !strings.Contains(p.Error, tc.want) {
				t.Errorf("error %q, want %q", p.Error, tc.want)
			}
		})
	}
	p := ai.Plan(ctx, camp, aiDM, record(t, aiplug.ActionUpdate, "---\nname: Seed\nrename: Sprout\nparts:\n  - {key: a, kind: widget, label: W a, see: party}\n---"))
	if p.Error != "" || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "(b)") || !strings.Contains(p.Summary, `renamed "Sprout"`) {
		t.Fatalf("update plan: %+v", p)
	}
}

// A recipe edited in the Workshop during review is not overwritten, and an
// approved delete removes it.
func TestAI_ChangedDuringReviewAndDelete(t *testing.T) {
	ai, svc := newAIRig(t)
	ctx := context.Background()
	made, err := svc.Create(ctx, camp, dm, in("Seed"))
	if err != nil {
		t.Fatal(err)
	}
	r := record(t, aiplug.ActionUpdate, "---\nname: Seed\nrules: []\n---")
	r.Seen = ai.Plan(ctx, camp, aiDM, r).Seen
	if _, err := svc.Update(ctx, camp, made.ID, dm, RecipeInput{Name: "Seed", Doc: made.Doc, Version: made.Version}); err != nil {
		t.Fatal(err)
	}
	if err := ai.Apply(ctx, camp, aiDM, r); err == nil || !strings.Contains(err.Error(), "changed in the Workshop") {
		t.Fatalf("stale apply: %v", err)
	}
	del := record(t, aiplug.ActionDelete, "---\nname: seed\n---")
	del.Seen = ai.Plan(ctx, camp, aiDM, del).Seen
	if err := ai.Apply(ctx, camp, aiDM, del); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.List(ctx, camp, dm); len(list) != 0 {
		t.Fatalf("left: %+v", list)
	}
}
