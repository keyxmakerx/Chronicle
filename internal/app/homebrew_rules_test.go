package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

func TestTrackerReached(t *testing.T) {
	tests := []struct {
		at, from, to int
		want         bool
	}{
		{3, 2, 3, true}, {3, 1, 5, true}, {3, 5, 3, true}, {3, 4, 1, true},
		{3, 3, 4, false}, {3, 0, 2, false}, {3, 3, 3, false},
	}
	for _, tt := range tests {
		args, _ := json.Marshal(map[string]int{"value": tt.at})
		payload, _ := json.Marshal(trackerMove{From: tt.from, To: tt.to})
		got, err := trackerReached(args, payload)
		if err != nil || got != tt.want {
			t.Errorf("reach %d from %d to %d = %v, %v", tt.at, tt.from, tt.to, got, err)
		}
	}
	if _, err := trackerReached(json.RawMessage(`{}`), json.RawMessage(`{"from":1,"to":2}`)); err == nil {
		t.Error("args without a value should be an error")
	}
}

// loggedTrackers records LogOwned calls and answers with views.
type loggedTrackers struct {
	trackers.Service
	calls []string
	views []trackers.TrackerView
}

func (l *loggedTrackers) LogOwned(_ context.Context, campaignID, entityID, owner string, v trackers.Viewer, op string, amount int, note string) ([]trackers.TrackerView, error) {
	l.calls = append(l.calls, campaignID+"|"+entityID+"|"+owner+"|"+v.UserID+"|"+op+"|"+note+"|"+string(rune('0'+amount)))
	return l.views, nil
}

func TestTrackerRun(t *testing.T) {
	if trackerRun(nil, trackers.OpAdd, "by") != nil {
		t.Fatal("with no trackers service the action must leave rules to the Director")
	}
	svc := &loggedTrackers{}
	act := homebrew.Act{RecipeID: "r1", Part: &homebrew.Part{Key: "doom", Kind: "clock", Label: "Doom"}, Scope: "page",
		By: homebrew.Viewer{UserID: "dm", Director: true}, Note: "Rule from Dread"}
	step := trackerRun(svc, trackers.OpAdd, "by")
	tick := trackerRun(svc, trackers.OpAdd, "")

	act.Args = json.RawMessage(`{"by":2}`)
	if line, err := step(context.Background(), "camp", act); err != nil || line != "Doom is on no page yet, so nothing changed." {
		t.Errorf("no page: %q, %v", line, err)
	}
	svc.views = []trackers.TrackerView{{Value: 3, Now: "The well opens"}}
	if line, _ := tick(context.Background(), "camp", act); line != "Doom is now 3: The well opens." {
		t.Errorf("one page: %q", line)
	}
	svc.views = append(svc.views, trackers.TrackerView{Value: 1})
	if line, _ := step(context.Background(), "camp", act); line != "Doom changed on 2 pages." {
		t.Errorf("two pages: %q", line)
	}
	want := []string{
		"camp|page|homebrew:r1:clock:doom|dm|add|Rule from Dread|2",
		"camp|page|homebrew:r1:clock:doom|dm|add|Rule from Dread|1",
		"camp|page|homebrew:r1:clock:doom|dm|add|Rule from Dread|2",
	}
	if !reflect.DeepEqual(svc.calls, want) {
		t.Errorf("calls = %v", svc.calls)
	}
	act.Args = json.RawMessage(`{}`)
	if _, err := step(context.Background(), "camp", act); err == nil {
		t.Error("a step with no amount should be an error")
	}
	act.Part = nil
	if _, err := tick(context.Background(), "camp", act); err == nil {
		t.Error("a tracker action with no part should be an error")
	}
}

// firedRecipes records Fire calls.
type firedRecipes struct {
	homebrew.Service
	fired []homebrew.Fired
	by    []homebrew.Viewer
}

func (f *firedRecipes) Fire(_ context.Context, _ string, v homebrew.Viewer, ev homebrew.Fired) error {
	f.fired = append(f.fired, ev)
	f.by = append(f.by, v)
	return nil
}

func TestTrackerRulesFire(t *testing.T) {
	svc := &firedRecipes{}
	l := &trackerRules{svc: svc}
	v := trackers.Viewer{UserID: "dm", Director: true, VisibilityRole: 3}
	for _, owner := range []string{"homebrew:r1:tracker:need", "other:r1:tracker:need", "homebrew:r1"} {
		if err := l.TrackerChanged(context.Background(), "camp", v, trackers.Change{Owner: owner, EntityID: "page", From: 1, To: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if len(svc.fired) != 1 {
		t.Fatalf("fired %+v", svc.fired)
	}
	f := svc.fired[0]
	if f.Event != "tracker.reached" || f.RecipeID != "r1" || f.Part != "need" || f.Scope != "page" || string(f.Payload) != `{"from":1,"to":2}` {
		t.Errorf("fired %+v", f)
	}
	if svc.by[0] != (homebrew.Viewer{UserID: "dm", Director: true, VisibilityRole: 3}) {
		t.Errorf("by %+v", svc.by[0])
	}
}
