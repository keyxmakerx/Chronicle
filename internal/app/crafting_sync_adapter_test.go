package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/crafting"
)

// craftSyncMembers is a fixed roster with co-DM grants, for the acting rule.
type craftSyncMembers struct {
	roles   map[string]int
	granted map[string]bool
}

func (m craftSyncMembers) MemberRole(_ context.Context, _, userID string) (int, bool, error) {
	r, ok := m.roles[userID]
	return r, ok, nil
}

func (m craftSyncMembers) IsDmGranted(_ context.Context, _, userID string) (bool, error) {
	return m.granted[userID], nil
}

// fakeCraftingSync records the viewer and input of each call it hears.
type fakeCraftingSync struct {
	crafting.Service
	viewers []crafting.Viewer
	inputs  []any
}

func (f *fakeCraftingSync) heard(v crafting.Viewer, in any) {
	f.viewers, f.inputs = append(f.viewers, v), append(f.inputs, in)
}

func (f *fakeCraftingSync) Character(_ context.Context, _, id string, v crafting.Viewer) (*crafting.CharacterCrafting, error) {
	f.heard(v, id)
	return &crafting.CharacterCrafting{}, nil
}

func (f *fakeCraftingSync) StartProject(_ context.Context, _, _ string, v crafting.Viewer, in crafting.StartInput) (*crafting.ProjectView, error) {
	f.heard(v, in)
	return &crafting.ProjectView{}, nil
}

func (f *fakeCraftingSync) RollProject(_ context.Context, _, _ string, v crafting.Viewer, in crafting.RollInput) (*crafting.RollResult, error) {
	f.heard(v, in)
	return &crafting.RollResult{}, nil
}

func (f *fakeCraftingSync) CancelProject(_ context.Context, _, id string, v crafting.Viewer) error {
	f.heard(v, id)
	return nil
}

func (f *fakeCraftingSync) Spoils(_ context.Context, _ string, v crafting.Viewer) (*crafting.SpoilsPage, error) {
	f.heard(v, nil)
	return &crafting.SpoilsPage{}, nil
}

func (f *fakeCraftingSync) TakeSpoil(_ context.Context, _ string, _ int64, v crafting.Viewer, in crafting.TakeInput) (*crafting.TakeResult, error) {
	f.heard(v, in)
	return &crafting.TakeResult{}, nil
}

func (f *fakeCraftingSync) StudySpoil(_ context.Context, _ string, _ int64, v crafting.Viewer, in crafting.TakeInput) (*crafting.StudyResult, error) {
	f.heard(v, in)
	return &crafting.StudyResult{}, nil
}

func (f *fakeCraftingSync) Experiment(_ context.Context, _, _ string, v crafting.Viewer, in crafting.TryInput) (*crafting.TryResult, error) {
	f.heard(v, in)
	return &crafting.TryResult{}, nil
}

func (f *fakeCraftingSync) BreakDown(_ context.Context, _, _ string, v crafting.Viewer, in crafting.BreakInput) (*crafting.BreakResult, error) {
	f.heard(v, in)
	return &crafting.BreakResult{}, nil
}

func (f *fakeCraftingSync) Help(_ context.Context, _, _ string, v crafting.Viewer, in crafting.HelpInput) (*crafting.RollResult, error) {
	f.heard(v, in)
	return &crafting.RollResult{}, nil
}

func (f *fakeCraftingSync) FightEnded(_ context.Context, _ string, v crafting.Viewer, fight crafting.Fight) (*crafting.FightSpoils, error) {
	f.heard(v, fight)
	return &crafting.FightSpoils{Lines: []string{"Troll left spoils in the tray: 1 × Troll tooth."}, Spoils: 2}, nil
}

// craftSyncCalls runs every adapter call once with the same key and name.
func craftSyncCalls(a *syncCraftingAPIAdapter, key, acting string) []error {
	ctx := context.Background()
	body := json.RawMessage(`{"actingUserId":"x","characterId":"hero"}`)
	var errs []error
	keep := func(_ any, err error) { errs = append(errs, err) }
	keep(a.Character(ctx, "c", key, acting, "hero"))
	keep(a.StartProject(ctx, "c", key, acting, "hero", json.RawMessage(`{"recipeId":"r1","picks":{"0":"i1"}}`)))
	keep(a.RollProject(ctx, "c", key, acting, "pj", json.RawMessage(`{"work":"clean","version":3}`)))
	errs = append(errs, a.CancelProject(ctx, "c", key, acting, "pj"))
	keep(a.Spoils(ctx, "c", key, acting))
	keep(a.TakeSpoil(ctx, "c", key, acting, 4, body))
	keep(a.StudySpoil(ctx, "c", key, acting, 4, body))
	keep(a.FightEnded(ctx, "c", key, acting, json.RawMessage(`{"name":"Bog","creatures":[{"pageId":"troll","name":"Troll","down":2,"copies":3}]}`)))
	keep(a.Experiment(ctx, "c", key, acting, "hero", json.RawMessage(`{"items":[{"itemId":"i1","qty":2}]}`)))
	keep(a.BreakDown(ctx, "c", key, acting, "hero", json.RawMessage(`{"recipeId":"r1","copy":4}`)))
	keep(a.Help(ctx, "c", key, acting, "pj", json.RawMessage(`{"characterId":"hero","version":3}`)))
	return errs
}

// Every crafting call from Foundry runs as the named member, with that
// member's own role: only the Owner or a co-DM is crafting's Director, and
// only they may name someone else.
func TestSyncCraftingAdapter_ActsAsMember(t *testing.T) {
	members := craftSyncMembers{
		roles:   map[string]int{"gm": permissions.RoleOwner, "codm": permissions.RolePlayer, "p1": permissions.RolePlayer, "scribe": permissions.RoleScribe},
		granted: map[string]bool{"codm": true},
	}
	cases := []struct {
		name       string
		key        string
		acting     string
		wantCode   int
		wantViewer crafting.Viewer
	}{
		{"GM key as a named player is that player, never the Director", "gm", "p1", 0,
			crafting.Viewer{UserID: "p1", MemberRole: permissions.RolePlayer, VisibilityRole: permissions.RolePlayer}},
		{"GM key without a name is the Director", "gm", "", 0,
			crafting.Viewer{UserID: "gm", MemberRole: permissions.RoleOwner, VisibilityRole: permissions.RoleOwner, Director: true}},
		{"a co-DM's key is the Director", "codm", "", 0,
			crafting.Viewer{UserID: "codm", MemberRole: permissions.RoleOwner, VisibilityRole: permissions.RoleOwner, Director: true}},
		{"a Scribe is not the Director", "scribe", "", 0,
			crafting.Viewer{UserID: "scribe", MemberRole: permissions.RoleScribe, VisibilityRole: permissions.RoleScribe}},
		{"a player's key cannot name someone else", "p1", "gm", 403, crafting.Viewer{}},
		{"a name that is not a member is not found", "gm", "stranger", 404, crafting.Viewer{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeCraftingSync{}
			a := &syncCraftingAPIAdapter{actors: armory.NewStashAPI(nil, members), svc: svc}
			for i, err := range craftSyncCalls(a, tc.key, tc.acting) {
				if tc.wantCode == 0 {
					if err != nil {
						t.Fatalf("call %d: %v", i, err)
					}
					continue
				}
				var appErr *apperror.AppError
				if !errors.As(err, &appErr) || appErr.Code != tc.wantCode {
					t.Fatalf("call %d: want %d, got %v", i, tc.wantCode, err)
				}
			}
			if tc.wantCode != 0 {
				if len(svc.viewers) != 0 {
					t.Fatal("crafting ran on a refused call")
				}
				return
			}
			if len(svc.viewers) != 11 {
				t.Fatalf("crafting heard %d calls, want 11", len(svc.viewers))
			}
			for i, v := range svc.viewers {
				if v != tc.wantViewer {
					t.Fatalf("call %d as %+v, want %+v", i, v, tc.wantViewer)
				}
			}
		})
	}
}

// Bodies reach crafting decoded into its own input types, and a fight's
// result comes back with camelCase keys for the GM's whisper.
func TestSyncCraftingAdapter_Bodies(t *testing.T) {
	svc := &fakeCraftingSync{}
	members := craftSyncMembers{roles: map[string]int{"gm": permissions.RoleOwner}}
	a := &syncCraftingAPIAdapter{actors: armory.NewStashAPI(nil, members), svc: svc}
	for i, err := range craftSyncCalls(a, "gm", "") {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	want := []any{
		"hero",
		crafting.StartInput{RecipeID: "r1", Picks: map[int]string{0: "i1"}},
		crafting.RollInput{Work: "clean", Version: 3},
		"pj",
		nil,
		crafting.TakeInput{CharacterID: "hero"},
		crafting.TakeInput{CharacterID: "hero"},
		crafting.Fight{Name: "Bog", Creatures: []crafting.FightCreature{{PageID: "troll", Name: "Troll", Down: 2, Copies: 3}}},
		crafting.TryInput{Items: []crafting.TryPick{{ItemID: "i1", Qty: 2}}},
		crafting.BreakInput{RecipeID: "r1", Copy: 4},
		crafting.HelpInput{CharacterID: "hero", Version: 3},
	}
	if !reflect.DeepEqual(svc.inputs, want) {
		t.Fatalf("inputs\n got %#v\nwant %#v", svc.inputs, want)
	}

	got, err := a.FightEnded(context.Background(), "c", "gm", "", json.RawMessage(`{"creatures":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	if string(raw) != `{"lines":["Troll left spoils in the tray: 1 × Troll tooth."],"spoils":2}` {
		t.Fatalf("fight answer = %s", raw)
	}

	if _, err := a.TakeSpoil(context.Background(), "c", "gm", "", 1, json.RawMessage(`{"characterId":7}`)); err == nil {
		t.Fatal("a body of the wrong shape was accepted")
	}
}

// Before crafting is built, or while it is degraded, every call is
// NotFound and no member is looked up.
func TestSyncCraftingAdapter_NotAttached(t *testing.T) {
	a := &syncCraftingAPIAdapter{actors: armory.NewStashAPI(nil, craftSyncMembers{})}
	for i, err := range craftSyncCalls(a, "gm", "") {
		var appErr *apperror.AppError
		if !errors.As(err, &appErr) || appErr.Code != 404 {
			t.Fatalf("call %d: want 404, got %v", i, err)
		}
	}
}
