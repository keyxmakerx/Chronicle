package records

import (
	"context"
	"strings"
	"testing"
)

type fakeCategorySwitch struct {
	owner    string
	switches []CategorySwitch
	accepted []int
	back     []int
}

func (f *fakeCategorySwitch) IsOwner(_ context.Context, _, userID string) (bool, error) {
	return userID == f.owner, nil
}
func (f *fakeCategorySwitch) Switches(context.Context, string) ([]CategorySwitch, error) {
	return f.switches, nil
}
func (f *fakeCategorySwitch) Accept(_ context.Context, _ string, id int, _ string) error {
	f.accepted = append(f.accepted, id)
	return nil
}
func (f *fakeCategorySwitch) Back(_ context.Context, _ string, id int, _ string) error {
	f.back = append(f.back, id)
	return nil
}

func itemsSwitch(switched, canBack, answered bool) CategorySwitch {
	return CategorySwitch{
		TypeID: 4, Category: "Items", System: "Draw Steel", Label: "treasure fields",
		Adds: "Adds Type, Kind and Echelon.", Removes: "Rarity comes off the pages.",
		Switched: switched, CanSwitchBack: canBack, OwnerAnswered: answered,
		SwitchedAtBirth: switched && !canBack,
	}
}

func TestCategorySwitchKindPlan(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	tests := []struct {
		name      string
		actor     Actor
		sw        CategorySwitch
		fields    map[string]any
		action    string
		wantErr   string
		wantPlan  string
		wantApply string // "accept", "back" or ""
	}{
		{name: "the owner asks for the treasure fields", actor: owner, sw: itemsSwitch(false, false, false),
			fields: map[string]any{"category": "Items", "use": "system"}, wantPlan: "Items switch to Draw Steel’s treasure fields. Adds Type, Kind and Echelon. Rarity comes off the pages.", wantApply: "accept"},
		{name: "the category matches by any case", actor: owner, sw: itemsSwitch(false, false, true),
			fields: map[string]any{"category": "items", "use": "System"}, wantPlan: "Items switch", wantApply: "accept"},
		{name: "already switched", actor: owner, sw: itemsSwitch(true, true, true),
			fields: map[string]any{"category": "Items", "use": "system"}, wantErr: "already use"},
		{name: "switch back", actor: owner, sw: itemsSwitch(true, true, true),
			fields: map[string]any{"category": "Items", "use": "own"}, wantPlan: "go back to the fields they had before", wantApply: "back"},
		{name: "nothing to go back to", actor: owner, sw: itemsSwitch(true, false, true),
			fields: map[string]any{"category": "Items", "use": "own"}, wantErr: "from the start"},
		{name: "back when not switched", actor: owner, sw: itemsSwitch(false, false, false),
			fields: map[string]any{"category": "Items", "use": "own"}, wantErr: "already have their own fields"},
		{name: "a co-DM is not the owner", actor: Actor{UserID: "codm", Role: 3}, sw: itemsSwitch(false, false, false),
			fields: map[string]any{"category": "Items", "use": "system"}, wantErr: "only the campaign's owner"},
		{name: "a player is not the owner", actor: Actor{UserID: "own", Role: 1}, sw: itemsSwitch(false, false, false),
			fields: map[string]any{"category": "Items", "use": "system"}, wantErr: "only the campaign's owner"},
		{name: "an unknown category", actor: owner, sw: itemsSwitch(false, false, false),
			fields: map[string]any{"category": "Spells", "use": "system"}, wantErr: "no fields for a category called"},
		{name: "use must be named", actor: owner, sw: itemsSwitch(false, false, false),
			fields: map[string]any{"category": "Items"}, wantErr: "`use:`"},
		{name: "a switch can't be deleted", actor: owner, sw: itemsSwitch(true, true, true), action: ActionDelete,
			fields: map[string]any{"category": "Items"}, wantErr: "use: own"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeCategorySwitch{owner: "own", switches: []CategorySwitch{tc.sw}}
			k := CategorySwitchKind{Svc: f}
			action := tc.action
			if action == "" {
				action = ActionUpdate
			}
			r := Record{Kind: k.Name(), Action: action, Fields: tc.fields}
			p := k.Plan(context.Background(), "c1", tc.actor, r)
			if tc.wantErr != "" {
				if !strings.Contains(p.Error, tc.wantErr) {
					t.Fatalf("Plan error = %q, want it to mention %q", p.Error, tc.wantErr)
				}
				if err := k.Apply(context.Background(), "c1", tc.actor, r); err == nil {
					t.Error("Apply must refuse what Plan refuses")
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tc.wantPlan) {
				t.Fatalf("Plan = %+v, want summary with %q", p, tc.wantPlan)
			}
			if err := k.Apply(context.Background(), "c1", tc.actor, r); err != nil {
				t.Fatal(err)
			}
			switch tc.wantApply {
			case "accept":
				if len(f.accepted) != 1 || f.accepted[0] != 4 || len(f.back) != 0 {
					t.Errorf("accepted %v back %v", f.accepted, f.back)
				}
			case "back":
				if len(f.back) != 1 || len(f.accepted) != 0 {
					t.Errorf("accepted %v back %v", f.accepted, f.back)
				}
			}
		})
	}
}

func TestCategorySwitchKindExport(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	tests := []struct {
		name string
		sw   CategorySwitch
		want []string
	}{
		{"unanswered", itemsSwitch(false, false, false), []string{"use: own", "Draw Steel offers Items its treasure fields", "has not answered"}},
		{"Not now", itemsSwitch(false, false, true), []string{"use: own", "Suggest the switch only if they ask"}},
		{"switched", itemsSwitch(true, true, true), []string{"use: system", "can switch back"}},
		{"made with them", itemsSwitch(true, false, true), []string{"use: system", "from the start"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := CategorySwitchKind{Svc: &fakeCategorySwitch{owner: "own", switches: []CategorySwitch{tc.sw}}}
			out, err := k.Export(context.Background(), "c1", owner)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "kind: category-switch") || !strings.Contains(out, `category: "Items"`) {
				t.Errorf("export lacks the record: %q", out)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("export lacks %q: %q", w, out)
				}
			}
		})
	}
	k := CategorySwitchKind{Svc: &fakeCategorySwitch{owner: "own", switches: []CategorySwitch{itemsSwitch(false, false, false)}}}
	if out, _ := k.Export(context.Background(), "c1", Actor{UserID: "own", Role: 1}); out != "" {
		t.Errorf("the players-view key sees no switch, got %q", out)
	}
	none := CategorySwitchKind{Svc: &fakeCategorySwitch{owner: "own"}}
	if out, _ := none.Export(context.Background(), "c1", owner); out != "" {
		t.Errorf("no switch offered exports nothing, got %q", out)
	}
}
