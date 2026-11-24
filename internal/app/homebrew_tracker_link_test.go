package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// linkTrackers is a trackers.Service with a campaign's kinds, recording
// syncs and rule changes on hand-made kinds.
type linkTrackers struct {
	trackers.Service
	kinds  []trackers.Kind
	synced map[string][]trackers.OwnedKind
	calls  []string
	views  []trackers.TrackerView
}

func (l *linkTrackers) KindNamed(_ context.Context, _, name string) (trackers.Kind, bool, error) {
	for _, k := range l.kinds {
		if strings.EqualFold(k.Name, strings.TrimSpace(name)) {
			return k, true, nil
		}
	}
	return trackers.Kind{}, false, nil
}

func (l *linkTrackers) CheckOwned(context.Context, string, string, []trackers.OwnedKind) error {
	return nil
}

func (l *linkTrackers) SyncOwned(_ context.Context, _ string, prefix string, want []trackers.OwnedKind) error {
	l.synced[prefix] = want
	return nil
}

func (l *linkTrackers) LogKind(_ context.Context, campaignID, entityID, name string, v trackers.Viewer, op string, amount int, note string) ([]trackers.TrackerView, error) {
	l.calls = append(l.calls, campaignID+"|"+entityID+"|"+name+"|"+v.UserID+"|"+op+"|"+note+"|"+string(rune('0'+amount)))
	return l.views, nil
}

func hungerKinds() *linkTrackers {
	three, six := 3, 6
	zero := 0
	return &linkTrackers{synced: map[string][]trackers.OwnedKind{}, kinds: []trackers.Kind{
		{ID: "k1", Name: "The Hunger", Shape: trackers.ShapeCount, Min: &zero, Max: &three,
			Bands: []trackers.Band{{At: 1, Text: "Restless"}}},
		{ID: "k2", Name: "Doom", Shape: trackers.ShapeClock, Max: &six},
		{ID: "k3", Name: "Kept", Shape: trackers.ShapeCount, Owner: "homebrew:r9:tracker:kept", Managed: true},
	}}
}

func TestTrackerPartLink(t *testing.T) {
	tk := hungerKinds()
	reg := homebrew.NewRegistry()
	if err := registerTrackerParts(reg, tk); err != nil {
		t.Fatal(err)
	}
	svc := homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg, nil, nil)
	tests := []struct {
		name, kind, data string
		want             string // normalised data; "" means refused
		why              string
	}{
		{"links by name, spelled as the campaign spells it, with the kind's shape", "tracker", `{"link":" the hunger "}`,
			`{"shape":"count","min":0,"max":3,"bands":[{"at":1,"text":"Restless"}],"link":"The Hunger"}`, ""},
		{"a stale shape sent back is replaced by the kind's own", "tracker", `{"link":"The Hunger","shape":"count","max":9}`,
			`{"shape":"count","min":0,"max":3,"bands":[{"at":1,"text":"Restless"}],"link":"The Hunger"}`, ""},
		{"a clock part links a clock", "clock", `{"link":"Doom"}`, `{"shape":"clock","max":6,"link":"Doom"}`, ""},
		{"unknown name", "tracker", `{"link":"Thirst"}`, "", "no tracker called"},
		{"a recipe's own kind", "tracker", `{"link":"Kept"}`, "", "kept by a recipe"},
		{"a tracker part can't link a clock", "tracker", `{"link":"Doom"}`, "", "is a clock"},
		{"a clock part can't link a count", "clock", `{"link":"The Hunger"}`, "", "not a clock"},
		{"too long", "tracker", `{"link":"` + strings.Repeat("x", trackers.MaxNameChars+1) + `"}`, "", "too long"},
		{"an empty link is an ordinary part", "tracker", `{"link":"","max":2}`, `{"shape":"count","max":2}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := homebrew.Part{Key: "hunger", Kind: tt.kind, Label: "Hunger", See: homebrew.SeeDirector, Data: json.RawMessage(tt.data)}
			r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{p}})
			if tt.want == "" {
				if hbCode(err) != 422 || !strings.Contains(err.Error(), tt.why) {
					t.Fatalf("err = %v, want 422 saying %q", err, tt.why)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := string(r.Doc.Parts[0].Data); got != tt.want {
				t.Errorf("data = %s, want %s", got, tt.want)
			}
		})
	}

	t.Run("a linked part keeps no kind of its own", func(t *testing.T) {
		r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{
			{Key: "hunger", Kind: "tracker", Label: "Hunger", See: homebrew.SeeDirector, Data: json.RawMessage(`{"link":"The Hunger"}`)},
			{Key: "need", Kind: "tracker", Label: "Need", See: homebrew.SeeDirector, Data: json.RawMessage(`{"max":5}`)},
		}})
		if err != nil {
			t.Fatal(err)
		}
		got := tk.synced[recipeOwnerPrefix(r.ID)+"tracker:"]
		if len(got) != 1 || got[0].Name != "Need" {
			t.Fatalf("synced %+v, want only Need", got)
		}
	})

	t.Run("without trackers a link can't be checked", func(t *testing.T) {
		svc, _ := trackerService(t)
		_, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{{Key: "h", Kind: "tracker", Label: "H", See: homebrew.SeeDirector, Data: json.RawMessage(`{"link":"The Hunger"}`)}}})
		if hbCode(err) != 422 {
			t.Fatalf("err = %v, want 422", err)
		}
	})
}

func TestTrackerRunLinked(t *testing.T) {
	tk := hungerKinds()
	step := trackerRun(tk, trackers.OpAdd, "by")
	act := homebrew.Act{RecipeID: "r1", Args: json.RawMessage(`{"by":1}`), By: homebrew.Viewer{UserID: "dm", Director: true}, Note: "Rule from Red Tide",
		Part: &homebrew.Part{Key: "hunger", Kind: "tracker", Label: "Hunger", Data: json.RawMessage(`{"shape":"count","link":"The Hunger"}`)}}
	ctx := context.Background()

	if line, err := step(ctx, "camp", act); err != nil || line != "The Hunger is on no page yet, so nothing changed." {
		t.Errorf("no page: %q, %v", line, err)
	}
	act.Scope = "saat"
	if line, err := step(ctx, "camp", act); err != nil || line != "The Hunger is not on that page, so nothing changed." {
		t.Errorf("not on the scope page: %q, %v", line, err)
	}
	tk.views = []trackers.TrackerView{{Value: 2, Now: "Restless"}}
	if line, err := step(ctx, "camp", act); err != nil || line != "The Hunger is now 2: Restless." {
		t.Errorf("one page: %q, %v", line, err)
	}
	want := []string{"camp||The Hunger|dm|add|Rule from Red Tide|1", "camp|saat|The Hunger|dm|add|Rule from Red Tide|1", "camp|saat|The Hunger|dm|add|Rule from Red Tide|1"}
	if !reflect.DeepEqual(tk.calls, want) {
		t.Errorf("calls = %v", tk.calls)
	}

	// Renamed or removed since the recipe was saved: the Director is told
	// which part and what to do, and nothing is written.
	tk.calls = nil
	act.Part.Data = json.RawMessage(`{"link":"The Old Hunger"}`)
	if _, err := step(ctx, "camp", act); hbCode(err) != 422 || !strings.Contains(err.Error(), "choose it again") || len(tk.calls) != 0 {
		t.Errorf("gone kind: %v, calls %v", err, tk.calls)
	}
	act.Part.Data = json.RawMessage(`{"link":"Kept"}`)
	if _, err := step(ctx, "camp", act); hbCode(err) != 422 || len(tk.calls) != 0 {
		t.Errorf("kind now kept by a recipe: %v, calls %v", err, tk.calls)
	}
}

// linkedRecipes answers PartsOfKind with saved parts and records Fire.
type linkedRecipes struct {
	firedRecipes
	parts []homebrew.RecipePart
	kinds []string
}

func (l *linkedRecipes) PartsOfKind(_ context.Context, _ string, _ homebrew.Viewer, kinds ...string) ([]homebrew.RecipePart, error) {
	l.kinds = kinds
	return l.parts, nil
}

func TestTrackerRulesFireLinked(t *testing.T) {
	part := func(recipe, key, kind, data string) homebrew.RecipePart {
		return homebrew.RecipePart{RecipeID: recipe, Part: homebrew.Part{Key: key, Kind: kind, Data: json.RawMessage(data)}}
	}
	svc := &linkedRecipes{parts: []homebrew.RecipePart{
		part("r1", "hunger", "tracker", `{"link":"The Hunger"}`),
		part("r2", "pangs", "tracker", `{"link":"the hunger"}`),
		part("r2", "need", "tracker", `{"max":5}`),
		part("r3", "doom", "clock", `{"link":"Doom"}`),
	}}
	l := &trackerRules{svc: svc}
	player := trackers.Viewer{UserID: "tam"}
	if err := l.TrackerChanged(context.Background(), "camp", player, trackers.Change{KindName: "The Hunger", EntityID: "saat", From: 2, To: 3}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(svc.kinds, ",") != "tracker,clock" {
		t.Errorf("asked for kinds %v", svc.kinds)
	}
	var got []string
	for _, f := range svc.fired {
		got = append(got, f.Event+"|"+f.RecipeID+"|"+f.Part+"|"+f.Scope+"|"+string(f.Payload))
	}
	want := []string{`tracker.reached|r1|hunger|saat|{"from":2,"to":3}`, `tracker.reached|r2|pangs|saat|{"from":2,"to":3}`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fired %v, want %v", got, want)
	}
	// A chosen player's change runs the Director's recipe with the
	// Director's standing, as for a recipe's own tracker.
	for _, by := range svc.by {
		if !by.Director || by.UserID != "tam" {
			t.Errorf("by %+v", by)
		}
	}

	t.Run("a hand-made kind no part links fires nothing", func(t *testing.T) {
		svc.fired = nil
		if err := l.TrackerChanged(context.Background(), "camp", player, trackers.Change{KindName: "Gold", EntityID: "saat", From: 0, To: 1}); err != nil || len(svc.fired) != 0 {
			t.Fatalf("fired %v, %v", svc.fired, err)
		}
	})
}

func TestCrossesMark(t *testing.T) {
	tests := []struct {
		from, to, at, every int
		want                bool
	}{
		// Once: lands on or passes it, either way.
		{2, 3, 3, 0, true}, {1, 5, 3, 0, true}, {5, 3, 3, 0, true}, {4, 1, 3, 0, true},
		{3, 4, 3, 0, false}, {0, 2, 3, 0, false}, {3, 3, 3, 0, false}, {3, 2, 3, 0, false},
		// Again every 3 from 3: 3, 6, 9 and on.
		{5, 6, 3, 3, true}, {8, 9, 3, 3, true}, {6, 7, 3, 3, false}, {4, 5, 3, 3, false},
		{0, 1, 3, 3, false}, {2, 3, 3, 3, true}, {1, 20, 3, 3, true},
		{7, 6, 3, 3, true}, {10, 9, 3, 3, true}, {6, 5, 3, 3, false}, {4, 2, 3, 3, true}, {3, 1, 3, 3, false},
		{2, 1, 3, 3, false},
		// Below the first mark nothing repeats.
		{-6, -3, 3, 3, false}, {-1, 0, 0, 2, true}, {1, 2, 0, 2, true}, {2, 3, 0, 2, false},
		// Every 1 is every value from the mark up.
		{4, 5, 3, 1, true}, {1, 2, 3, 1, false},
	}
	for _, tt := range tests {
		if got := crossesMark(tt.from, tt.to, tt.at, tt.every); got != tt.want {
			t.Errorf("from %d to %d, at %d every %d = %v, want %v", tt.from, tt.to, tt.at, tt.every, got, tt.want)
		}
	}
}

func TestTrackerReachedEvery(t *testing.T) {
	tests := []struct {
		args string
		from int
		to   int
		want bool
	}{
		{`{"value":3,"every":3}`, 5, 6, true},
		{`{"value":3,"every":3}`, 6, 7, false},
		{`{"value":3}`, 5, 6, false},
		{`{"value":3,"every":0}`, 5, 6, false},
	}
	for _, tt := range tests {
		payload, _ := json.Marshal(trackerMove{From: tt.from, To: tt.to})
		got, err := trackerReached(json.RawMessage(tt.args), payload)
		if err != nil || got != tt.want {
			t.Errorf("%s from %d to %d = %v, %v", tt.args, tt.from, tt.to, got, err)
		}
	}
}

func TestTrackerReachedEveryCheck(t *testing.T) {
	svc, _ := trackerService(t)
	parts := []homebrew.Part{{Key: "heat", Kind: "tracker", Label: "Heat", Data: json.RawMessage(`{"shape":"count"}`)}}
	for args, ok := range map[string]bool{
		`{"value":3,"every":3}`:       true,
		`{"value":3}`:                 true,
		`{"value":3,"every":0}`:       false,
		`{"value":3,"every":-2}`:      false,
		`{"value":3,"every":1000001}`: false,
		`{"value":3,"every":"3"}`:     false,
		`{"every":3}`:                 false,
	} {
		rule := homebrew.Rule{When: homebrew.When{Event: "tracker.reached", Part: "heat", Args: json.RawMessage(args)},
			Then: []homebrew.Do{{Action: "set", Part: "heat", Args: json.RawMessage(`{"to":0}`)}}}
		_, err := saveDoc(t, svc, homebrew.Doc{Parts: parts, Rules: []homebrew.Rule{rule}})
		if ok && err != nil {
			t.Errorf("%s: %v", args, err)
		}
		if !ok && hbCode(err) != 422 {
			t.Errorf("%s: err = %v, want 422", args, err)
		}
	}
}
