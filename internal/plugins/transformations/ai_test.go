package transformations

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

var (
	aiDM     = aiplug.Actor{UserID: "u-dm", Role: permissions.RoleOwner}
	aiPlayer = aiplug.Actor{UserID: "u-pl", Role: permissions.RolePlayer}
)

// namedPages finds the test service's pages by name.
type namedPages map[string]string

func (n namedPages) FindPage(_ context.Context, _, name string) (string, string, bool, error) {
	id, ok := n[strings.ToLower(name)]
	return id, name, ok, nil
}

func newAILayers(on bool) (*AILayers, Service) {
	svc, _ := newTestService()
	pages := namedPages{"mira": "mirela", "the hidden one": "secret", "locked": "locked"}
	return NewAILayers(svc, pages, func(context.Context, string) bool { return on }), svc
}

func layerRec(action string, fields map[string]any) aiplug.Record {
	return aiplug.Record{Kind: "layer", Action: action, Name: "Revenant", Fields: fields}
}

// The changes as YAML hands them over: nested values are maps and numbers
// come as ints.
func revenantFields() map[string]any {
	return map[string]any{"page": "Mira", "source": "drawsteel:revenant", "changes": []any{
		map[string]any{"field": "ancestry", "op": "set", "value": "Revenant"},
		map[string]any{"field": "recoveries_max", "op": "add", "value": -2},
		map[string]any{"field": "features_json", "op": "append", "value": map[string]any{"name": "Tough But Withered"}, "label": "Tough But Withered"},
	}}
}

func TestAILayers_Plan(t *testing.T) {
	tests := []struct {
		name     string
		on       bool
		a        aiplug.Actor
		r        aiplug.Record
		wantErr  string
		wantSumm string
	}{
		{"puts one on", true, aiDM, layerRec(aiplug.ActionCreate, revenantFields()), "",
			"puts Revenant on Mira: ancestry set to Revenant, recoveries_max add -2, features_json append Tough But Withered"},
		{"add-on off", false, aiDM, layerRec(aiplug.ActionCreate, revenantFields()), "switched off", ""},
		{"players can't", true, aiPlayer, layerRec(aiplug.ActionCreate, revenantFields()), "only the Director", ""},
		{"no such page", true, aiDM, layerRec(aiplug.ActionCreate, map[string]any{"page": "Nobody"}), "no page called", ""},
		{"a page the Director can't edit", true, aiDM, layerRec(aiplug.ActionCreate, map[string]any{"page": "Locked"}), "can't edit", ""},
		{"a bad field", true, aiDM, layerRec(aiplug.ActionCreate, map[string]any{"page": "Mira", "changes": []any{map[string]any{"field": "Bad Field", "op": "set", "value": "x"}}}), "not valid", ""},
		{"no changes", true, aiDM, layerRec(aiplug.ActionCreate, map[string]any{"page": "Mira"}), "between 1 and 40", ""},
		{"never changed in place", true, aiDM, layerRec(aiplug.ActionUpdate, revenantFields()), "take it off", ""},
		{"taking off one that isn't there", true, aiDM, layerRec(aiplug.ActionDelete, map[string]any{"page": "Mira"}), "has no layer called", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, _ := newAILayers(tt.on)
			p := k.Plan(context.Background(), "c1", tt.a, tt.r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.wantSumm {
				t.Fatalf("plan = %+v, want summary %q", p, tt.wantSumm)
			}
		})
	}
}

// An approved layer is stored as the sheet's own Push would store it, the
// lookup reads it back, and taking it off by name removes it.
func TestAILayers_ApplyAndLookup(t *testing.T) {
	ctx := context.Background()
	k, svc := newAILayers(true)
	if err := k.Apply(ctx, "c1", aiDM, layerRec(aiplug.ActionCreate, revenantFields())); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ListForPage(ctx, "c1", "mirela", dm)
	if err != nil || len(got.Layers) != 1 {
		t.Fatalf("layers = %+v, %v", got, err)
	}
	l := got.Layers[0]
	if l.Source != "drawsteel:revenant" || string(l.Changes[1].Value) != "-2" || string(l.Changes[2].Value) != `{"name":"Tough But Withered"}` {
		t.Fatalf("stored %+v", l)
	}

	_, text, err := k.Answer(ctx, "c1", aiPlayer, aiplug.Record{Fields: map[string]any{"page": "Mira"}})
	if err != nil || !strings.Contains(text, "## What is transforming Mira (1)") || !strings.Contains(text, "- Revenant (drawsteel:revenant): ancestry set to Revenant") {
		t.Fatalf("answer %q, %v", text, err)
	}
	if _, _, err := k.Answer(ctx, "c1", aiPlayer, aiplug.Record{Fields: map[string]any{"page": "The hidden one"}}); err == nil || !strings.Contains(err.Error(), "no page called") {
		t.Fatalf("a hidden page answered: %v", err)
	}

	if err := k.Apply(ctx, "c1", aiDM, layerRec(aiplug.ActionDelete, map[string]any{"page": "mira"})); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.ListForPage(ctx, "c1", "mirela", dm); len(got.Layers) != 0 {
		t.Fatalf("still on: %+v", got.Layers)
	}
}

func TestAILayers_LookupOff(t *testing.T) {
	k, _ := newAILayers(false)
	if _, _, err := k.Answer(context.Background(), "c1", aiDM, aiplug.Record{Fields: map[string]any{"page": "Mira"}}); err == nil {
		t.Fatal("answered with the add-on off")
	}
}
