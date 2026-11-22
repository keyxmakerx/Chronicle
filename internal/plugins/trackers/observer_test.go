package trackers

import (
	"context"
	"testing"
)

// touches counts how often an observer was told, per campaign.
type touches struct{ n map[string]int }

func (o *touches) TrackersChanged(campaignID string) { o.n[campaignID]++ }

// TestChangeObserver pins that every write that can change what a card's tests
// read tells the observer, for any kind and not only owned ones, and that a
// refused write tells it nothing.
func TestChangeObserver(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	o := &touches{n: map[string]int{}}
	r.svc.(*service).SetChangeObserver(o)
	k := r.addKind(t, Kind{ID: "k-hand", Name: "Hand", Shape: ShapeCount})
	name := "Renamed"

	// A refused write is silent.
	if _, err := r.svc.Place(ctx, cmp, pageA, player, PlaceInput{KindID: k.ID}); err == nil {
		t.Fatal("a player placed a tracker")
	}
	if o.n[cmp] != 0 {
		t.Fatalf("a refused write was told: %d", o.n[cmp])
	}

	tv, err := r.svc.Place(ctx, cmp, pageA, dm, PlaceInput{KindID: k.ID})
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		name string
		do   func() error
	}{
		{"Log", func() error {
			_, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: tv.ID, Op: OpAdd, Amount: 1})
			return err
		}},
		{"Patch", func() error {
			_, err := r.svc.Patch(ctx, cmp, tv.ID, dm, TrackerPatch{Name: &name})
			return err
		}},
		{"Strike", func() error {
			got, err := r.svc.Page(ctx, cmp, pageA, dm)
			if err != nil {
				return err
			}
			_, err = r.svc.Strike(ctx, cmp, got.Trackers[0].Log[0].ID, true, dm)
			return err
		}},
		{"UpdateKind", func() error {
			_, err := r.svc.UpdateKind(ctx, cmp, k.ID, dm, KindInput{Name: "Hand", Shape: ShapeCount, Max: five(), Version: 1})
			return err
		}},
		{"Remove", func() error { return r.svc.Remove(ctx, cmp, tv.ID, dm) }},
		{"DeleteKind", func() error { return r.svc.DeleteKind(ctx, cmp, k.ID, dm) }},
	}
	// Place told it once already.
	want := 1
	if o.n[cmp] != want {
		t.Fatalf("Place told the observer %d times", o.n[cmp])
	}
	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		want++
		if o.n[cmp] != want {
			t.Errorf("%s: observer told %d times, want %d", s.name, o.n[cmp], want)
		}
	}
}
