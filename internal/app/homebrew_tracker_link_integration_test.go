package app

// homebrew_tracker_link_integration_test.go runs a recipe whose parts use
// page trackers over the real homebrew and trackers services on a scratch
// database. Skipped under -short and without a database.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

func TestRecipeUsesPageTrackers(t *testing.T) {
	w := newStoryWorld(t)
	ctx := context.Background()
	tv := trackers.Viewer{UserID: w.owner, Director: true, VisibilityRole: 3, MemberRole: 3}

	reg := homebrew.NewRegistry()
	w.must(registerTrackerParts(reg, w.trackers))
	w.must(reg.RegisterEvent(homebrew.EventDef{Name: respiteEvent, Label: "A respite is logged", Press: "Log a respite",
		Check: func(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		}}))
	hb := homebrew.NewService(homebrew.NewRepository(w.db), reg, nil, homebrew.NewRunStore(w.db))
	w.trackers.(interface{ SetChangeListener(trackers.ChangeListener) }).SetChangeListener(&trackerRules{svc: hb})

	// The campaign already keeps both counts by hand on Tam's page; Orrin
	// has Hunger too, but no respite count.
	zero, three := 0, 3
	resp, err := w.trackers.CreateKind(ctx, w.camp, tv, trackers.KindInput{Name: "Respites since dose", Shape: trackers.ShapeCount, Min: &zero, Max: &three, Visibility: trackers.SeeDirectors})
	w.must(err)
	hunger, err := w.trackers.CreateKind(ctx, w.camp, tv, trackers.KindInput{Name: "The Hunger", Shape: trackers.ShapeCount, Min: &zero, Max: &three,
		Bands: []trackers.Band{{At: 1, Text: "Restless"}}, Visibility: trackers.SeeDirectors})
	w.must(err)
	for _, p := range []struct{ page, kind string }{{w.tamHero, resp.ID}, {w.tamHero, hunger.ID}, {w.orrinHero, hunger.ID}} {
		_, err := w.trackers.Place(ctx, w.camp, p.page, tv, trackers.PlaceInput{KindID: p.kind})
		w.must(err)
	}

	r, err := hb.Create(ctx, w.camp, w.hbDirector, homebrew.RecipeInput{Name: "The Red Tide", Doc: homebrew.Doc{
		Parts: []homebrew.Part{
			{Key: "respites", Kind: "tracker", Label: "Respites", See: homebrew.SeeDirector, Data: json.RawMessage(`{"link":"respites since dose"}`)},
			{Key: "hunger", Kind: "tracker", Label: "Hunger", See: homebrew.SeeDirector, Data: json.RawMessage(`{"link":"The Hunger"}`)},
		},
		Rules: []homebrew.Rule{
			{When: homebrew.When{Event: respiteEvent}, Timing: homebrew.TimingNow,
				Then: []homebrew.Do{{Action: "step", Part: "respites", Args: json.RawMessage(`{"by":1}`)}}},
			{When: homebrew.When{Event: "tracker.reached", Part: "respites", Args: json.RawMessage(`{"value":3}`)}, Timing: homebrew.TimingNow,
				Then: []homebrew.Do{{Action: "step", Part: "hunger", Args: json.RawMessage(`{"by":1}`)}, {Action: "set", Part: "respites", Args: json.RawMessage(`{"to":0}`)}}},
		}}})
	w.must(err)
	var d trackerPartData
	w.must(json.Unmarshal(r.Doc.Parts[0].Data, &d))
	if d.Link != "Respites since dose" || d.Max == nil || *d.Max != 3 {
		t.Fatalf("stored part data %s", r.Doc.Parts[0].Data)
	}
	// The recipe keeps no kinds of its own.
	kinds, _, err := w.trackers.Kinds(ctx, w.camp, tv)
	w.must(err)
	if len(kinds) != 2 {
		t.Fatalf("kinds = %+v, want only the two made by hand", kinds)
	}

	find := func(page, name string) trackers.TrackerView {
		t.Helper()
		pv, err := w.trackers.Page(ctx, w.camp, page, tv)
		w.must(err)
		for _, tr := range pv.Trackers {
			if tr.Name == name {
				return tr
			}
		}
		t.Fatalf("%s not on the page", name)
		return trackers.TrackerView{}
	}
	value := func(page, name string) int { t.Helper(); return find(page, name).Value }
	tamResp := find(w.tamHero, "Respites since dose").ID

	// Three respites: the respite count reaches 3, which steps Hunger and
	// sets the count back to 0. The reach came from Tam's page, so only Tam's
	// Hunger moves; Orrin has no respite count to reach anything.
	for i := 0; i < 3; i++ {
		_, err := hb.Press(ctx, w.camp, respiteEvent, "", w.hbDirector)
		w.must(err)
	}
	if got := value(w.tamHero, "Respites since dose"); got != 0 {
		t.Errorf("respites = %d, want 0", got)
	}
	if got := value(w.tamHero, "The Hunger"); got != 1 {
		t.Errorf("Tam's hunger = %d, want 1", got)
	}
	if got := value(w.orrinHero, "The Hunger"); got != 0 {
		t.Errorf("Orrin's hunger = %d, want 0", got)
	}

	// A Director moving the page tracker by hand sets the rules off too.
	_, err = w.trackers.Log(ctx, w.camp, w.tamHero, tv, trackers.LogInput{TrackerID: tamResp, Op: trackers.OpSet, Amount: 3, Note: "By hand"})
	w.must(err)
	if got := value(w.tamHero, "The Hunger"); got != 2 {
		t.Errorf("after a hand change, Tam's hunger = %d, want 2", got)
	}
	if got := value(w.orrinHero, "The Hunger"); got != 0 {
		t.Errorf("a hand change on Tam's page moved Orrin's hunger to %d", got)
	}

	// Renaming the kind leaves the link pointing at nothing: the run fails
	// with a line the Director can act on, and nothing else moves.
	hunger.Name = "The Thirst"
	_, err = w.trackers.UpdateKind(ctx, w.camp, hunger.ID, tv, trackers.KindInput{Name: "The Thirst", Shape: trackers.ShapeCount, Min: &zero, Max: &three, Visibility: trackers.SeeDirectors, Version: hunger.Version})
	w.must(err)
	_, err = w.trackers.Log(ctx, w.camp, w.tamHero, tv, trackers.LogInput{TrackerID: tamResp, Op: trackers.OpSet, Amount: 0})
	w.must(err)
	_, err = w.trackers.Log(ctx, w.camp, w.tamHero, tv, trackers.LogInput{TrackerID: tamResp, Op: trackers.OpSet, Amount: 3})
	w.must(err)
	dr, err := hb.Drawer(ctx, w.camp, w.hbDirector)
	w.must(err)
	// Runs written in the same second share a time, so look for the one.
	const want = "Step a tracker did not run: Hunger uses The Hunger, which is no longer a tracker made on a page; choose it again in the Workshop"
	failed := 0
	for _, run := range dr.Recent {
		if run.Status == homebrew.RunFailed {
			failed++
			if len(run.Lines) == 0 || run.Lines[len(run.Lines)-1] != want {
				t.Errorf("failed run lines = %q", run.Lines)
			}
		}
	}
	if failed != 1 {
		t.Errorf("%d failed runs, want 1: %+v", failed, dr.Recent)
	}
	// A placed tracker keeps the name it was placed with.
	if got := value(w.tamHero, "The Hunger"); got != 2 {
		t.Errorf("the renamed kind's tracker moved to %d", got)
	}
}
