package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// fakeArmory keeps counts the way the Armory does: 0 takes the line away.
type fakeArmory struct {
	counts bool
	rels   *writeRels
	sets   []int
}

func (f *fakeArmory) Counts(context.Context, string, Actor, string, string) bool { return f.counts }
func (f *fakeArmory) SetCarried(_ context.Context, _ string, _ Actor, char, item string, qty int) error {
	f.sets = append(f.sets, qty)
	for i, r := range f.rels.rels {
		if r.SourceEntityID == char && r.TargetEntityID == item {
			if qty == 0 {
				f.rels.rels = append(f.rels.rels[:i], f.rels.rels[i+1:]...)
				return nil
			}
			m := map[string]any{}
			_ = json.Unmarshal(r.Metadata, &m)
			m["quantity"] = qty
			f.rels.rels[i].Metadata, _ = json.Marshal(m)
			return nil
		}
	}
	md, _ := json.Marshal(map[string]any{"quantity": qty, "equipped": false})
	f.rels.rels = append(f.rels.rels, relations.Relation{ID: len(f.rels.rels) + 10, SourceEntityID: char, TargetEntityID: item, RelationType: "Has Item", Metadata: md})
	return nil
}

// writeRels is a relation store the carried-item kind may write to.
type writeRels struct {
	rels                  []relations.Relation
	creates, metas, drops int
}

func (f *writeRels) ListByEntity(_ context.Context, _, id string) ([]relations.Relation, error) {
	var out []relations.Relation
	for _, r := range f.rels {
		if r.SourceEntityID == id {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *writeRels) Create(context.Context, string, string, string, string, string, string, json.RawMessage, ...bool) (*relations.Relation, error) {
	f.creates++
	return &relations.Relation{}, nil
}
func (f *writeRels) UpdateMetadata(_ context.Context, id int, md json.RawMessage) error {
	f.metas++
	for i := range f.rels {
		if f.rels[i].ID == id {
			f.rels[i].Metadata = md
		}
	}
	return nil
}
func (f *writeRels) Delete(context.Context, int) error { f.drops++; return nil }

func TestCarriedItemThroughTheArmory(t *testing.T) {
	const camp = "c1"
	pages := fakeEntities{
		"ser-aldric": {ID: "e-ser", Name: "Ser Aldric", CampaignID: camp},
		"potion":     {ID: "e-pot", Name: "Potion", CampaignID: camp},
	}
	held := func() []relations.Relation {
		return []relations.Relation{{ID: 1, SourceEntityID: "e-ser", TargetEntityID: "e-pot", RelationType: "Has Item",
			Metadata: json.RawMessage(`{"quantity":3,"equipped":true}`)}}
	}
	tests := []struct {
		name        string
		action      string
		fields      map[string]any
		start       []relations.Relation
		wantSummary string
		wantSets    []int
		wantMeta    string // a fragment of the line's metadata after; "" when it is gone
	}{
		{"raise", ActionUpdate, map[string]any{"quantity": 5}, held(), "gives 2 (holds 5 after) · in the item history", []int{5}, `"quantity":5`},
		{"lower", ActionUpdate, map[string]any{"quantity": 1}, held(), "takes 2 away (holds 1 after) · in the item history", []int{1}, `"equipped":true`},
		{"delete", ActionDelete, nil, held(), "takes all 3 away · in the item history", []int{0}, ""},
		{"first one", ActionCreate, nil, nil, "gives 1 (holds 1 after) · in the item history", []int{1}, `"quantity":1`},
		{"notes only", ActionUpdate, map[string]any{"notes": "cracked vial"}, held(), "1 carried", nil, `"notes":"cracked vial"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rels := &writeRels{rels: tc.start}
			arm := &fakeArmory{counts: true, rels: rels}
			k := CarriedItemKind(pages, rels, arm)
			f := map[string]any{"character": "Ser Aldric", "item": "Potion"}
			for key, v := range tc.fields {
				f[key] = v
			}
			r := Record{Kind: "carried-item", Action: tc.action, Fields: f}
			p := k.Plan(context.Background(), camp, Actor{UserID: "gm", Role: 3}, r)
			if p.Error != "" || p.Summary != tc.wantSummary {
				t.Fatalf("plan %+v, want summary %q", p, tc.wantSummary)
			}
			if err := k.Apply(context.Background(), camp, Actor{UserID: "gm", Role: 3}, r); err != nil {
				t.Fatal(err)
			}
			if len(arm.sets) != len(tc.wantSets) || (len(arm.sets) > 0 && arm.sets[0] != tc.wantSets[0]) {
				t.Fatalf("set %v, want %v", arm.sets, tc.wantSets)
			}
			if rels.creates != 0 || rels.drops != 0 {
				t.Fatalf("wrote the relation by hand: %d creates, %d deletes", rels.creates, rels.drops)
			}
			got := ""
			for _, l := range rels.rels {
				got = string(l.Metadata)
			}
			if (tc.wantMeta == "") != (got == "") || !strings.Contains(got, tc.wantMeta) {
				t.Fatalf("line %q, want %q", got, tc.wantMeta)
			}
		})
	}
}

func TestCarriedItemPlainWhereTheArmoryDoesNotCount(t *testing.T) {
	const camp = "c1"
	pages := fakeEntities{
		"ser-aldric": {ID: "e-ser", Name: "Ser Aldric", CampaignID: camp},
		"potion":     {ID: "e-pot", Name: "Potion", CampaignID: camp},
	}
	rels := &writeRels{rels: []relations.Relation{{ID: 1, SourceEntityID: "e-ser", TargetEntityID: "e-pot", RelationType: "Has Item", Metadata: json.RawMessage(`{"quantity":3}`)}}}
	arm := &fakeArmory{counts: false, rels: rels}
	k := CarriedItemKind(pages, rels, arm)
	r := Record{Kind: "carried-item", Action: ActionUpdate, Fields: map[string]any{"character": "Ser Aldric", "item": "Potion", "quantity": 1}}
	if p := k.Plan(context.Background(), camp, Actor{UserID: "s", Role: 2}, r); p.Summary != "1 carried" {
		t.Fatalf("plan %+v", p)
	}
	if err := k.Apply(context.Background(), camp, Actor{UserID: "s", Role: 2}, r); err != nil {
		t.Fatal(err)
	}
	if len(arm.sets) != 0 || rels.metas != 1 {
		t.Fatalf("armory %v, relation writes %d", arm.sets, rels.metas)
	}
}

func TestCarriedItemArmoryCountsAreWhole(t *testing.T) {
	pages := fakeEntities{
		"ser-aldric": {ID: "e-ser", Name: "Ser Aldric", CampaignID: "c1"},
		"potion":     {ID: "e-pot", Name: "Potion", CampaignID: "c1"},
	}
	rels := &writeRels{}
	k := CarriedItemKind(pages, rels, &fakeArmory{counts: true, rels: rels})
	r := Record{Kind: "carried-item", Action: ActionCreate, Fields: map[string]any{"character": "Ser Aldric", "item": "Potion", "quantity": 1.5}}
	if p := k.Plan(context.Background(), "c1", Actor{UserID: "gm", Role: 3}, r); !strings.Contains(p.Error, "whole number") {
		t.Fatalf("plan %+v", p)
	}
}
