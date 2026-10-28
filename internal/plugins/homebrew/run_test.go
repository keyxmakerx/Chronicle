package homebrew

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeRuns is an in-memory RunStore with the SQL one's semantics.
type fakeRuns struct {
	runs   map[string]Run
	order  []string
	pruned int
}

func (f *fakeRuns) CreateRun(_ context.Context, r Run) error {
	f.runs[r.ID] = r
	f.order = append(f.order, r.ID)
	return nil
}

func (f *fakeRuns) GetRun(_ context.Context, c, id string) (Run, bool, error) {
	r, ok := f.runs[id]
	if !ok || r.CampaignID != c {
		return Run{}, false, nil
	}
	return r, true, nil
}

func (f *fakeRuns) FinishRun(_ context.Context, r Run, from string) (bool, error) {
	got, ok := f.runs[r.ID]
	if !ok || got.CampaignID != r.CampaignID || got.Status != from {
		return false, nil
	}
	got.Status, got.Lines, got.DoneBy, got.DoneAt = r.Status, r.Lines, r.DoneBy, r.DoneAt
	f.runs[r.ID] = got
	return true, nil
}

func (f *fakeRuns) ListRuns(_ context.Context, c string, held bool, limit int) ([]Run, error) {
	out := []Run{}
	for i := len(f.order) - 1; i >= 0 && len(out) < limit; i-- {
		r := f.runs[f.order[i]]
		if r.CampaignID == c && (r.Status == RunHeld) == held {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRuns) PruneRuns(context.Context, string, int) error { f.pruned++; return nil }

// rig is a service over a registry with a numbered "counter" part whose
// add action changes an in-memory value and fires counter.reached, so
// rules can set each other off as tracker rules do.
type runRig struct {
	svc    *service
	runs   *fakeRuns
	values map[string]int
	acts   []Act
}

func newRunRig(t *testing.T) *runRig {
	t.Helper()
	rg := &runRig{runs: &fakeRuns{runs: map[string]Run{}}, values: map[string]int{}}
	reg := NewRegistry()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	whole := []Field{{Name: "value", Label: "Reaches", Type: FieldWhole}}
	must(reg.RegisterPart(PartKind{Name: "counter", Label: "Counter", Check: okCheck}))
	must(reg.RegisterEvent(EventDef{Name: "counter.reached", Label: "A counter reaches", PartKinds: []string{"counter"}, Fields: whole, Check: okCheck,
		Match: func(args, payload json.RawMessage) (bool, error) {
			var a struct{ Value int }
			var p struct{ From, To int }
			if err := json.Unmarshal(args, &a); err != nil {
				return false, err
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return false, err
			}
			return p.From < a.Value && p.To >= a.Value, nil
		}}))
	must(reg.RegisterEvent(EventDef{Name: "rest", Label: "A rest", Press: "Log a rest", Check: okCheck}))
	must(reg.RegisterEvent(EventDef{Name: "quiet", Label: "Quiet", Check: okCheck}))
	must(reg.RegisterAction(ActionDef{Name: "add", Label: "Add one", PartKinds: []string{"counter"}, Check: okCheck,
		Run: func(ctx context.Context, campaignID string, a Act) (string, error) {
			rg.acts = append(rg.acts, a)
			from := rg.values[a.Part.Key]
			rg.values[a.Part.Key] = from + 1
			payload, _ := json.Marshal(map[string]int{"from": from, "to": from + 1})
			if err := rg.svc.Fire(ctx, campaignID, a.By, Fired{Event: "counter.reached", RecipeID: a.RecipeID, Part: a.Part.Key, Scope: a.Scope, Payload: payload}); err != nil {
				return "", err
			}
			return fmt.Sprintf("%s is now %d.", a.Part.Label, from+1), nil
		}}))
	must(reg.RegisterAction(ActionDef{Name: "boom", Label: "Boom", Check: okCheck,
		Run: func(context.Context, string, Act) (string, error) {
			return "", apperror.NewValidation("the boom is out of powder")
		}}))
	must(reg.RegisterAction(ActionDef{Name: "secret-boom", Label: "Secret boom", Check: okCheck,
		Run: func(context.Context, string, Act) (string, error) { return "", errDB }}))
	must(reg.RegisterAction(ActionDef{Name: "hand", Label: "Do it by hand", Check: okCheck}))
	must(reg.RegisterAction(ActionDef{Name: "browser", Label: "Run in the browser", ClientRun: true, Check: okCheck}))
	rg.svc = NewService(newFakeRepo(), reg, nil, rg.runs).(*service)
	n := 0
	rg.svc.newID = func() string { n++; return fmt.Sprintf("id-%d", n) }
	return rg
}

func counter(key string) Part { return Part{Key: key, Kind: "counter", Label: "C " + key, See: SeeParty} }

func (rg *runRig) save(t *testing.T, name string, parts []Part, rules ...Rule) *Recipe {
	t.Helper()
	r, err := rg.svc.Create(context.Background(), camp, dm, RecipeInput{Name: name, Doc: Doc{Parts: parts, Rules: rules}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func reached(part string, at int, timing string, then ...Do) Rule {
	return Rule{When: When{Event: "counter.reached", Part: part, Args: raw(fmt.Sprintf(`{"value":%d}`, at))}, Timing: timing, Then: then}
}

func (rg *runRig) statuses() []string {
	var out []string
	for _, id := range rg.runs.order {
		out = append(out, rg.runs.runs[id].Status)
	}
	return out
}

func TestFire(t *testing.T) {
	ctx := context.Background()

	t.Run("a rule timed now runs, and its change sets off the next", func(t *testing.T) {
		rg := newRunRig(t)
		r := rg.save(t, "Chain", []Part{counter("a"), counter("b")},
			Rule{When: When{Event: "rest"}, Timing: TimingNow, Then: []Do{{Action: "add", Part: "a"}}},
			reached("a", 1, TimingNow, Do{Action: "add", Part: "b"}))
		if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "rest", Scope: "page-1"}); err != nil {
			t.Fatal(err)
		}
		if rg.values["a"] != 1 || rg.values["b"] != 1 {
			t.Fatalf("values = %v", rg.values)
		}
		if got := strings.Join(rg.statuses(), ","); got != "done,done" {
			t.Fatalf("runs = %s", got)
		}
		first := rg.runs.runs["id-3"]
		if first.What != "A counter reaches: C a, reaches 1" || first.RecipeName != "Chain" || first.Scope != "page-1" {
			t.Errorf("chained run = %+v", first)
		}
		for _, a := range rg.acts {
			if a.Note != "Rule from Chain" || a.By.UserID != dm.UserID || a.RecipeID != r.ID || a.Scope != "page-1" {
				t.Errorf("act = %+v", a)
			}
		}
		if rg.runs.pruned == 0 {
			t.Error("finished runs were never pruned")
		}
	})

	t.Run("Match and the part decide which rules start", func(t *testing.T) {
		rg := newRunRig(t)
		r := rg.save(t, "Pick", []Part{counter("a"), counter("b")},
			reached("a", 3, TimingNow, Do{Action: "boom"}),
			reached("b", 1, TimingNow, Do{Action: "boom"}))
		for _, p := range []string{`{"from":0,"to":2}`, `{"from":3,"to":4}`} {
			if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "counter.reached", RecipeID: r.ID, Part: "a", Payload: raw(p)}); err != nil {
				t.Fatal(err)
			}
		}
		if len(rg.runs.order) != 0 {
			t.Fatalf("no rule should have started: %v", rg.statuses())
		}
		if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "counter.reached", RecipeID: "someone-else", Part: "a", Payload: raw(`{"from":2,"to":3}`)}); err != nil {
			t.Fatal(err)
		}
		if len(rg.runs.order) != 0 {
			t.Fatal("another recipe's firing started this one")
		}
	})

	t.Run("later, tested, by-hand and browser rules wait", func(t *testing.T) {
		rg := newRunRig(t)
		rest := func(timing, test string, then ...Do) Rule {
			return Rule{When: When{Event: "rest", Test: test}, Timing: timing, Then: then}
		}
		rg.save(t, "Waits", []Part{counter("a")},
			rest(TimingHeld, "", Do{Action: "add", Part: "a"}),
			rest(TimingEncounterEnd, "", Do{Action: "add", Part: "a"}),
			rest(TimingNow, "only if the hero is winded", Do{Action: "add", Part: "a"}),
			rest(TimingNow, "", Do{Action: "add", Part: "a"}, Do{Action: "hand"}),
			rest(TimingNow, "", Do{Action: "browser"}))
		if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "rest", Scope: "page1"}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rg.statuses(), ","); got != "held,held,held,held,held" || rg.values["a"] != 0 {
			t.Fatalf("runs = %s, values = %v", got, rg.values)
		}
		d, err := rg.svc.Drawer(ctx, camp, dm)
		if err != nil {
			t.Fatal(err)
		}
		var waits []string
		for _, w := range d.Waiting {
			waits = append(waits, w.Waits)
		}
		sort.Strings(waits)
		want := []string{"Only if: only if the hero is winded", "Part of it is yours to run.", "Part of it is yours to run.",
			"Set to wait for the end of the encounter.", "Set to wait for you."}
		if strings.Join(waits, "|") != strings.Join(want, "|") {
			t.Errorf("waits = %v", waits)
		}
		w := d.Waiting[0]
		if len(w.Steps) != 1 || !w.Steps[0].Browser || w.Steps[0].Label != "Run in the browser" {
			t.Errorf("newest waiting = %+v", w)
		}
		if w.RecipeID == "" || w.Scope != "page1" {
			t.Errorf("waiting run names recipe %q and page %q, want both", w.RecipeID, w.Scope)
		}
		if len(d.Presses) != 1 || d.Presses[0].Label != "Log a rest" || d.Presses[0].Rules != 5 {
			t.Errorf("presses = %+v", d.Presses)
		}
	})

	t.Run("a failing action stops its rule and says why, plainly", func(t *testing.T) {
		rg := newRunRig(t)
		rg.save(t, "Fails", []Part{counter("a")},
			Rule{When: When{Event: "rest"}, Timing: TimingNow, Then: []Do{{Action: "boom"}, {Action: "add", Part: "a"}}},
			Rule{When: When{Event: "rest"}, Timing: TimingNow, Then: []Do{{Action: "secret-boom"}}})
		if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "rest"}); err != nil {
			t.Fatal(err)
		}
		a, b := rg.runs.runs["id-2"], rg.runs.runs["id-3"]
		if a.Status != RunFailed || strings.Join(a.Lines, "|") != "Boom did not run: the boom is out of powder" || rg.values["a"] != 0 {
			t.Errorf("first = %+v", a)
		}
		if b.Status != RunFailed || strings.Join(b.Lines, "|") != "Secret boom did not run: something went wrong." {
			t.Errorf("second = %+v", b)
		}
	})

	t.Run("rules that keep setting each other off stop", func(t *testing.T) {
		rg := newRunRig(t)
		rg.save(t, "Loop", []Part{counter("a")},
			Rule{When: When{Event: "rest"}, Timing: TimingNow, Then: []Do{{Action: "add", Part: "a"}}},
			Rule{When: When{Event: "counter.reached", Part: "a", Args: raw(`{"value":0}`)}, Timing: TimingNow, Then: []Do{{Action: "add", Part: "a"}}})
		// value 0 is never reached from below, so make every step reach it.
		rg.svc.reg.events["counter.reached"] = EventDef{Name: "counter.reached", Label: "A counter reaches", PartKinds: []string{"counter"}, Check: okCheck,
			Match: func(json.RawMessage, json.RawMessage) (bool, error) { return true, nil }}
		if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "rest"}); err != nil {
			t.Fatal(err)
		}
		st := rg.statuses()
		if rg.values["a"] != maxRunSteps || st[len(st)-1] != RunDone {
			t.Fatalf("value = %d, %d runs", rg.values["a"], len(st))
		}
		stopped := 0
		for _, s := range st {
			if s == RunStopped {
				stopped++
			}
		}
		if stopped != 1 {
			t.Errorf("stopped runs = %d, want 1", stopped)
		}
	})

	t.Run("only a Director, and nothing without a store", func(t *testing.T) {
		rg := newRunRig(t)
		if err := rg.svc.Fire(ctx, camp, player, Fired{Event: "rest"}); codeOf(err) != 403 {
			t.Errorf("player: %v", err)
		}
		if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "nope"}); err == nil {
			t.Error("an unknown event should be an error")
		}
		rg.svc.runs = nil
		if err := rg.svc.Fire(ctx, camp, dm, Fired{Event: "rest"}); err != nil {
			t.Error(err)
		}
	})
}

func TestHeldRuns(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (*runRig, string) {
		rg := newRunRig(t)
		rg.save(t, "Held", []Part{counter("a")},
			Rule{When: When{Event: "rest"}, Timing: TimingHeld, Then: []Do{{Action: "add", Part: "a"}, {Action: "browser"}, {Action: "hand"}}})
		if _, err := rg.svc.Press(ctx, camp, "rest", dm); err != nil {
			t.Fatal(err)
		}
		return rg, "id-2"
	}

	t.Run("Run does it once, as it was held", func(t *testing.T) {
		rg, id := setup(t)
		// The recipe changing after the rule was held doesn't change it.
		r := rg.svc.repo.(*fakeRepo).recipes["id-1"]
		r.Doc.Rules[0].Then = []Do{{Action: "boom"}}
		rg.svc.repo.(*fakeRepo).recipes["id-1"] = r
		v, err := rg.svc.RunHeld(ctx, camp, id, dm)
		if err != nil {
			t.Fatal(err)
		}
		want := "C a is now 1.|Run in the browser: run in your browser.|Do it by hand: for you to do."
		if v.Status != RunDone || strings.Join(v.Lines, "|") != want || rg.values["a"] != 1 {
			t.Fatalf("ran %+v, values %v", v, rg.values)
		}
		if _, err := rg.svc.RunHeld(ctx, camp, id, dm); codeOf(err) != 409 {
			t.Errorf("second run: %v", err)
		}
		if _, err := rg.svc.Dismiss(ctx, camp, id, dm); codeOf(err) != 409 {
			t.Errorf("dismiss after run: %v", err)
		}
		d, _ := rg.svc.Drawer(ctx, camp, dm)
		if len(d.Waiting) != 0 || len(d.Recent) != 1 || d.Recent[0].Status != RunDone {
			t.Errorf("drawer = %+v", d)
		}
	})

	t.Run("Dismiss sets it aside without running it", func(t *testing.T) {
		rg, id := setup(t)
		v, err := rg.svc.Dismiss(ctx, camp, id, dm)
		if err != nil || v.Status != RunDismissed || rg.values["a"] != 0 {
			t.Fatalf("dismissed %+v, %v", v, err)
		}
		if _, err := rg.svc.RunHeld(ctx, camp, id, dm); codeOf(err) != 409 {
			t.Errorf("run after dismiss: %v", err)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		rg, id := setup(t)
		for name, err := range map[string]error{
			"player run":     func() error { _, err := rg.svc.RunHeld(ctx, camp, id, player); return err }(),
			"player dismiss": func() error { _, err := rg.svc.Dismiss(ctx, camp, id, player); return err }(),
			"player drawer":  func() error { _, err := rg.svc.Drawer(ctx, camp, player); return err }(),
			"player press":   func() error { _, err := rg.svc.Press(ctx, camp, "rest", player); return err }(),
		} {
			if codeOf(err) != 403 {
				t.Errorf("%s: %v", name, err)
			}
		}
		for name, err := range map[string]error{
			"other campaign":  func() error { _, err := rg.svc.RunHeld(ctx, "other-camp", id, dm); return err }(),
			"missing":         func() error { _, err := rg.svc.RunHeld(ctx, camp, "nope", dm); return err }(),
			"bad id":          func() error { _, err := rg.svc.Dismiss(ctx, camp, "../x", dm); return err }(),
			"not a button":    func() error { _, err := rg.svc.Press(ctx, camp, "quiet", dm); return err }(),
			"unknown button":  func() error { _, err := rg.svc.Press(ctx, camp, "nope", dm); return err }(),
			"part event push": func() error { _, err := rg.svc.Press(ctx, camp, "counter.reached", dm); return err }(),
		} {
			if codeOf(err) != 404 {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}

func TestRegisterPressAndRun(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterEvent(EventDef{Name: "hit", PartKinds: []string{"widget"}, Press: "Hit", Check: okCheck}); err == nil {
		t.Error("a part's event can't be a drawer button")
	}
	run := func(context.Context, string, Act) (string, error) { return "", nil }
	if err := reg.RegisterAction(ActionDef{Name: "both", ClientRun: true, Run: run, Check: okCheck}); err == nil {
		t.Error("a browser action can't also run on the server")
	}
}

func TestPressTellsTheOwnerFirst(t *testing.T) {
	ctx := context.Background()
	rig := func(t *testing.T, fail error) (*service, *fakeRuns, *[]Viewer) {
		t.Helper()
		var heard []Viewer
		reg := NewRegistry()
		if err := reg.RegisterEvent(EventDef{Name: "rest", Label: "A rest", Press: "Log a rest", Check: okCheck,
			Pressed: func(_ context.Context, campaignID string, by Viewer) error {
				if campaignID != camp {
					t.Errorf("pressed in %q", campaignID)
				}
				heard = append(heard, by)
				return fail
			}}); err != nil {
			t.Fatal(err)
		}
		if err := reg.RegisterAction(ActionDef{Name: "hand", Label: "Do it by hand", Check: okCheck}); err != nil {
			t.Fatal(err)
		}
		runs := &fakeRuns{runs: map[string]Run{}}
		svc := NewService(newFakeRepo(), reg, nil, runs).(*service)
		if _, err := svc.Create(ctx, camp, dm, RecipeInput{Name: "Rest", Doc: Doc{Rules: []Rule{{When: When{Event: "rest"}, Timing: TimingHeld, Then: []Do{{Action: "hand"}}}}}}); err != nil {
			t.Fatal(err)
		}
		return svc, runs, &heard
	}

	t.Run("the owner hears the press, then the rules run", func(t *testing.T) {
		svc, runs, heard := rig(t, nil)
		if _, err := svc.Press(ctx, camp, "rest", dm); err != nil {
			t.Fatal(err)
		}
		if len(*heard) != 1 || (*heard)[0].UserID != dm.UserID || len(runs.runs) != 1 {
			t.Fatalf("heard %+v, runs %d", *heard, len(runs.runs))
		}
	})
	t.Run("a player's press reaches no one", func(t *testing.T) {
		svc, runs, heard := rig(t, nil)
		if _, err := svc.Press(ctx, camp, "rest", player); codeOf(err) != 403 {
			t.Fatalf("player press: %v", err)
		}
		if len(*heard) != 0 || len(runs.runs) != 0 {
			t.Fatalf("heard %+v, runs %d", *heard, len(runs.runs))
		}
	})
	t.Run("the owner failing stops the press before any rule", func(t *testing.T) {
		svc, runs, _ := rig(t, errDB)
		if _, err := svc.Press(ctx, camp, "rest", dm); codeOf(err) != 500 {
			t.Fatalf("press: %v", err)
		}
		if len(runs.runs) != 0 {
			t.Fatalf("a rule ran: %d runs", len(runs.runs))
		}
	})
}
