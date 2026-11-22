package records

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
)

// liveHome is a home fake whose Save and Reset change what Home reads, so a
// put-back can be seen to land.
type liveHome struct{ *fakeHome }

func (f liveHome) Save(_ context.Context, _, aud string, l HomeLayout) error {
	f.homes[aud] = l
	return nil
}
func (f liveHome) Reset(_ context.Context, _, aud string) error {
	delete(f.homes, aud)
	f.reset = append(f.reset, aud)
	return nil
}

// The home kind keeps the audience's home, fingerprints it after the change,
// and puts the same arrangement back; a borrowed home is given back, not
// saved.
func TestHomeKind_Undo(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	ctx := context.Background()
	f := liveHome{playersFake()}
	k := HomeKind{Svc: f}
	was, _, _ := f.Home(ctx, "c", "player")
	r := homeRec(ActionUpdate, "players", []any{row(col(12, "party"))})

	snap, err := k.Before(ctx, "c", owner, r)
	if err != nil || snap == nil || snap.Target != "player" || snap.Name != "players home" {
		t.Fatalf("Before = %+v, %v", snap, err)
	}
	if err := k.Apply(ctx, "c", owner, r); err != nil {
		t.Fatal(err)
	}
	after, err := k.Fingerprint(ctx, "c", owner, *snap)
	if err != nil || after == "" {
		t.Fatalf("Fingerprint: %q %v", after, err)
	}
	if again, _ := k.Fingerprint(ctx, "c", owner, *snap); again != after {
		t.Error("the fingerprint is not stable")
	}
	if err := k.Restore(ctx, "c", owner, *snap); err != nil {
		t.Fatal(err)
	}
	now, saved, _ := f.Home(ctx, "c", "player")
	if !saved || aiplug.Hash(now) != aiplug.Hash(was) {
		t.Errorf("home = %+v, want it back as %+v", now, was)
	}
	if again, _ := k.Fingerprint(ctx, "c", owner, *snap); again == after {
		t.Error("the fingerprint did not move when the home was put back")
	}

	// A home that borrowed another is given back to borrowing.
	snap, _ = k.Before(ctx, "c", owner, homeRec(ActionUpdate, "scribes", []any{row(col(12, "party"))}))
	f.homes["scribe"] = HomeLayout{Rows: []HomeRow{{ID: "x"}}}
	if err := k.Restore(ctx, "c", owner, *snap); err != nil {
		t.Fatal(err)
	}
	if _, saved, _ := f.Home(ctx, "c", "scribe"); saved {
		t.Error("a borrowed home was saved as its own")
	}
}

func TestHomeKind_NoUndoForTiles(t *testing.T) {
	f := playersFake()
	f.homes["player"] = HomeLayout{Tiles: true, Rows: f.homes["player"].Rows}
	snap, err := (HomeKind{Svc: f}).Before(context.Background(), "c", Actor{UserID: "own", Role: 3}, homeRec(ActionUpdate, "players", []any{row(col(12, "party"))}))
	if snap != nil || err != nil {
		t.Errorf("a tile home kept %+v, %v", snap, err)
	}
}

// A tracker kind's settings go back as they were; the version in the store
// moves on its own and must not count as a change, and a kind that was
// edited since is seen as such.
func TestTrackerKind_Undo(t *testing.T) {
	f := newFakeTrackers()
	two, ten := 2, 10
	f.kinds = []TrackerSetup{{ID: "k1", Name: "Debt", Shape: trackerCount, Min: &two, Max: &ten, Visibility: trackerHidden, Version: 3}}
	k := TrackerKind{Svc: f}
	a := Actor{UserID: "own", Role: 3}
	ctx := context.Background()
	r := Record{Kind: "tracker", Action: ActionUpdate, Name: "Debt", Fields: map[string]any{"max": 20}}

	snap, err := k.Before(ctx, "c", a, r)
	if err != nil || snap == nil || snap.Target != "k1" || snap.Name != "Debt" {
		t.Fatalf("Before = %+v, %v", snap, err)
	}
	if err := k.Apply(ctx, "c", a, r); err != nil {
		t.Fatal(err)
	}
	if *f.kinds[0].Max != 20 {
		t.Fatalf("apply left max %d", *f.kinds[0].Max)
	}
	after, _ := k.Fingerprint(ctx, "c", a, *snap)
	// Someone only bumps the version: still the state the request left.
	f.kinds[0].Version++
	f.kinds[0].InUse = 4
	if got, _ := k.Fingerprint(ctx, "c", a, *snap); got != after {
		t.Error("a version or usage change counted as an edit")
	}
	if err := k.Restore(ctx, "c", a, *snap); err != nil {
		t.Fatal(err)
	}
	if *f.kinds[0].Max != 10 {
		t.Errorf("max = %d, want 10 back", *f.kinds[0].Max)
	}
	// An edit of the settings is a change.
	f.kinds[0].Visibility = trackerShown
	if got, _ := k.Fingerprint(ctx, "c", a, *snap); got == after {
		t.Error("an edited tracker looked untouched")
	}
}

func TestTrackerKind_UndoOnlyForSettingsChanges(t *testing.T) {
	f := newFakeTrackers()
	f.kinds = []TrackerSetup{{ID: "k1", Name: "Debt", Shape: trackerCount, Visibility: trackerHidden, Version: 1}}
	k := TrackerKind{Svc: f}
	a := Actor{UserID: "own", Role: 3}
	for name, r := range map[string]Record{
		"a create":              {Kind: "tracker", Action: ActionCreate, Name: "New", Fields: map[string]any{"shape": "count"}},
		"a delete":              {Kind: "tracker", Action: ActionDelete, Name: "Debt"},
		"a log line, no change": {Kind: "tracker", Action: ActionUpdate, Name: "Debt", Fields: map[string]any{"page": "Orrin", "add": 1}},
	} {
		if snap, _ := k.Before(context.Background(), "c", a, r); snap != nil {
			t.Errorf("%s kept %+v", name, snap)
		}
	}
}
