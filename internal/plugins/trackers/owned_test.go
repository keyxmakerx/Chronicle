package trackers

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"testing"
)

func five() *int { n := 5; return &n }

func owned(owner, name, shape string) OwnedKind {
	s := Shape{Shape: shape}
	if shape != ShapeSteps {
		s.Max = five()
	} else {
		s.Steps = []Step{{Name: "a"}, {Name: "b"}}
	}
	return OwnedKind{Owner: owner, Name: name, Shape: s, Visibility: SeeEveryone}
}

// byOwner lists the campaign's kinds keyed by owner ("" for hand-made).
func byOwner(r *rig) map[string]Kind {
	out := map[string]Kind{}
	for _, k := range r.repo.kinds {
		out[k.Owner+"|"+k.Name] = k
	}
	return out
}

func TestSyncOwned(t *testing.T) {
	ctx := context.Background()
	const pre = "homebrew:r1:"

	t.Run("creates, updates in place and keeps a category", func(t *testing.T) {
		r := newRig(t)
		if err := r.svc.SyncOwned(ctx, cmp, pre, []OwnedKind{owned(pre+"need", "Need", ShapeCount)}); err != nil {
			t.Fatal(err)
		}
		var id string
		for _, k := range r.repo.kinds {
			id = k.ID
			if k.Owner != pre+"need" || !k.Managed || k.Version != 1 || *k.Max != 5 {
				t.Fatalf("created %+v", k)
			}
			k.EveryTypeID = typeHero
			r.repo.kinds[k.ID] = k
		}
		w := owned(pre+"need", "Remnant need", ShapeCount)
		w.Shape.Max = nil
		if err := r.svc.SyncOwned(ctx, cmp, pre, []OwnedKind{w}); err != nil {
			t.Fatal(err)
		}
		k := r.repo.kinds[id]
		if len(r.repo.kinds) != 1 || k.Name != "Remnant need" || k.Max != nil || k.Version != 2 || k.EveryTypeID != typeHero {
			t.Fatalf("updated %+v", k)
		}
	})

	t.Run("a part no longer wanted is deleted, or let go when on pages", func(t *testing.T) {
		r := newRig(t)
		if err := r.svc.SyncOwned(ctx, cmp, pre, []OwnedKind{owned(pre+"a", "A", ShapeCount), owned(pre+"b", "B", ShapeClock)}); err != nil {
			t.Fatal(err)
		}
		for _, k := range r.repo.kinds {
			if k.Name == "B" {
				r.addTracker(t, Tracker{ID: "t1", KindID: k.ID, Name: "B"})
			}
		}
		if err := r.svc.SyncOwned(ctx, cmp, pre, nil); err != nil {
			t.Fatal(err)
		}
		got := byOwner(r)
		if len(got) != 1 {
			t.Fatalf("kinds = %v", got)
		}
		if k, ok := got["|B"]; !ok || k.Managed {
			t.Fatalf("B should stay as a hand-made kind: %v", got)
		}
	})

	t.Run("other prefixes and hand-made kinds are left alone", func(t *testing.T) {
		r := newRig(t)
		r.addKind(t, Kind{ID: "hand", Name: "Gold", Shape: ShapeCount})
		r.addKind(t, Kind{ID: "other", Name: "Ink", Shape: ShapeCount, Owner: "homebrew:r2:ink", Managed: true})
		if err := r.svc.SyncOwned(ctx, cmp, pre, nil); err != nil {
			t.Fatal(err)
		}
		if len(r.repo.kinds) != 2 {
			t.Fatalf("kinds = %v", r.repo.kinds)
		}
	})

	refusals := []struct {
		name  string
		setup func(t *testing.T, r *rig)
		want  []OwnedKind
	}{
		{"a name a hand-made kind has", func(t *testing.T, r *rig) { r.addKind(t, Kind{ID: "hand", Name: "Need", Shape: ShapeCount}) },
			[]OwnedKind{owned(pre+"need", "Need", ShapeCount)}},
		{"a name another recipe's kind has", func(t *testing.T, r *rig) {
			r.addKind(t, Kind{ID: "o", Name: "Need", Shape: ShapeCount, Owner: "homebrew:r2:need"})
		}, []OwnedKind{owned(pre+"need", "need", ShapeCount)}},
		{"two parts with one name", nil, []OwnedKind{owned(pre+"a", "Need", ShapeCount), owned(pre+"b", "Need", ShapeClock)}},
		{"a bad shape", nil, []OwnedKind{{Owner: pre + "a", Name: "A", Shape: Shape{Shape: ShapeClock}, Visibility: SeeEveryone}}},
		{"a shape change under placed trackers", func(t *testing.T, r *rig) {
			r.addKind(t, Kind{ID: "k", Name: "Need", Shape: ShapeCount, Owner: pre + "need"})
			r.addTracker(t, Tracker{ID: "t1", KindID: "k", Name: "Need"})
		}, []OwnedKind{owned(pre+"need", "Need", ShapeClock)}},
		{"a let-go kind keeps its name", func(t *testing.T, r *rig) {
			r.addKind(t, Kind{ID: "k", Name: "Need", Shape: ShapeCount, Owner: pre + "old"})
			r.addTracker(t, Tracker{ID: "t1", KindID: "k", Name: "Need"})
		}, []OwnedKind{owned(pre+"new", "Need", ShapeCount)}},
	}
	for _, tt := range refusals {
		t.Run("refuses "+tt.name, func(t *testing.T) {
			r := newRig(t)
			if tt.setup != nil {
				tt.setup(t, r)
			}
			before := len(r.repo.kinds)
			if err := r.svc.CheckOwned(ctx, cmp, pre, tt.want); code(err) != http.StatusUnprocessableEntity {
				t.Fatalf("check: %v", err)
			}
			if err := r.svc.SyncOwned(ctx, cmp, pre, tt.want); code(err) != http.StatusUnprocessableEntity {
				t.Fatalf("sync: %v", err)
			}
			if len(r.repo.kinds) != before {
				t.Fatal("a refused sync wrote something")
			}
		})
	}

	t.Run("a renamed part frees its old name for another part", func(t *testing.T) {
		r := newRig(t)
		if err := r.svc.SyncOwned(ctx, cmp, pre, []OwnedKind{owned(pre+"a", "Need", ShapeCount)}); err != nil {
			t.Fatal(err)
		}
		if err := r.svc.SyncOwned(ctx, cmp, pre, []OwnedKind{owned(pre+"b", "Need", ShapeClock)}); err != nil {
			t.Fatal(err)
		}
		if got := byOwner(r); len(got) != 1 || got[pre+"b|Need"].Shape != ShapeClock {
			t.Fatalf("kinds = %v", got)
		}
	})

	t.Run("a bad owner is a programming error", func(t *testing.T) {
		r := newRig(t)
		if err := r.svc.SyncOwned(ctx, cmp, pre, []OwnedKind{owned("homebrew:r2:a", "A", ShapeCount)}); err == nil {
			t.Fatal("an owner outside the prefix was accepted")
		}
		if err := r.svc.SyncOwned(ctx, cmp, "", nil); err == nil {
			t.Fatal("an empty prefix was accepted")
		}
	})
}

func TestManagedKindRefusesHandEdits(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	r.addKind(t, Kind{ID: "k", Name: "Need", Shape: ShapeCount, Owner: "homebrew:r1:need", Managed: true})
	if _, err := r.svc.UpdateKind(ctx, cmp, "k", dm, KindInput{Name: "X", Shape: ShapeCount, Version: 1}); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("update: %v", err)
	}
	if err := r.svc.DeleteKind(ctx, cmp, "k", dm); code(err) != http.StatusUnprocessableEntity {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := r.repo.kinds["k"]; !ok {
		t.Fatal("the managed kind was removed")
	}
}

func TestOwnedPage(t *testing.T) {
	ctx := context.Background()
	const pre = "homebrew:r1:"
	setup := func(t *testing.T) (*rig, Kind, Kind) {
		r := newRig(t)
		need := r.addKind(t, Kind{ID: "k-need", Name: "Need", Shape: ShapeCount, Owner: pre + "tracker:need"})
		secret := r.addKind(t, Kind{ID: "k-doom", Name: "Doom", Shape: ShapeClock, Max: five(), Owner: pre + "clock:doom", Visibility: SeeDirectors})
		r.addKind(t, Kind{ID: "k-other", Name: "Other", Shape: ShapeCount, Owner: "homebrew:r10:tracker:x"})
		r.addKind(t, Kind{ID: "k-hand", Name: "Hand", Shape: ShapeCount, EveryTypeID: typeHero})
		return r, need, secret
	}
	names := func(v *PageView) []string {
		var out []string
		for _, t := range v.Trackers {
			out = append(out, t.Name)
		}
		sort.Strings(out)
		return out
	}

	t.Run("shows only the prefix's kinds, unplaced ones as virtual", func(t *testing.T) {
		r, _, _ := setup(t)
		v, err := r.svc.OwnedPage(ctx, cmp, pageA, pre, dm)
		if err != nil {
			t.Fatal(err)
		}
		if got := names(v); !reflect.DeepEqual(got, []string{"Doom", "Need"}) || v.Kinds != nil || !v.CanEdit {
			t.Fatalf("view = %v kinds=%v", got, v.Kinds)
		}
		v, err = r.svc.OwnedPage(ctx, cmp, pageA, pre, player)
		if err != nil {
			t.Fatal(err)
		}
		if got := names(v); !reflect.DeepEqual(got, []string{"Need"}) {
			t.Fatalf("player sees %v", got)
		}
	})

	t.Run("the plain view is unchanged", func(t *testing.T) {
		r, _, _ := setup(t)
		v, err := r.svc.Page(ctx, cmp, pageA, dm)
		if err != nil {
			t.Fatal(err)
		}
		if got := names(v); !reflect.DeepEqual(got, []string{"Hand"}) || len(v.Kinds) != 4 {
			t.Fatalf("view = %v kinds=%d", got, len(v.Kinds))
		}
	})

	t.Run("a first change creates the row only under the prefix", func(t *testing.T) {
		r, need, _ := setup(t)
		tv, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{KindID: need.ID, Owner: pre, Op: OpAdd, Amount: 2})
		if err != nil {
			t.Fatal(err)
		}
		if tv.ID == "" || tv.Value != 2 {
			t.Fatalf("logged %+v", tv)
		}
		for _, in := range []LogInput{
			{KindID: "k-other", Owner: pre, Op: OpAdd, Amount: 1},
			{KindID: "k-other", Owner: "homebrew:r1", Op: OpAdd, Amount: 1},
			{KindID: need.ID, Op: OpAdd, Amount: 1},
		} {
			if _, err := r.svc.Log(ctx, cmp, pageA, dm, in); code(err) != http.StatusNotFound {
				t.Fatalf("%+v: err = %v", in, err)
			}
		}
	})

	t.Run("a bad prefix or a hidden page is not found", func(t *testing.T) {
		r, _, _ := setup(t)
		for _, c := range []struct{ page, prefix string }{{pageA, ""}, {pageA, "homebrew:r1"}, {pageHid, pre}} {
			if _, err := r.svc.OwnedPage(ctx, cmp, c.page, c.prefix, player); code(err) != http.StatusNotFound {
				t.Fatalf("%v: err = %v", c, err)
			}
		}
	})
}

// heard records what a ChangeListener was told.
type heard struct {
	changes []Change
	err     error
}

func (h *heard) TrackerChanged(_ context.Context, _ string, _ Viewer, c Change) error {
	h.changes = append(h.changes, c)
	return h.err
}

func TestChangeListenerAndLogOwned(t *testing.T) {
	ctx := context.Background()
	const owner = "homebrew:r1:tracker:need"
	setup := func(t *testing.T) (*rig, *heard, Kind) {
		r := newRig(t)
		h := &heard{}
		r.svc.(*service).SetChangeListener(h)
		k := r.addKind(t, Kind{ID: "k-need", Name: "Need", Shape: ShapeCount, Max: five(), Owner: owner})
		r.addKind(t, Kind{ID: "k-hand", Name: "Hand", Shape: ShapeCount})
		return r, h, k
	}

	t.Run("an owned kind's move is heard; a hand-made one's and a no-op aren't", func(t *testing.T) {
		r, h, k := setup(t)
		tv, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{KindID: k.ID, Owner: "homebrew:r1:", Op: OpAdd, Amount: 2})
		if err != nil {
			t.Fatal(err)
		}
		hand := r.addTracker(t, Tracker{ID: "t-hand", KindID: "k-hand", Name: "Hand"})
		if _, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: hand.ID, Op: OpAdd, Amount: 1}); err != nil {
			t.Fatal(err)
		}
		if want := []Change{{Owner: owner, EntityID: pageA, From: 0, To: 2}}; !reflect.DeepEqual(h.changes, want) {
			t.Fatalf("heard %+v", h.changes)
		}
		// Striking the line moves it back, and that is heard too; a
		// listener failure doesn't undo the change.
		h.err = errors.New("listener down")
		got, err := r.svc.Strike(ctx, cmp, tv.Log[0].ID, true, dm)
		if err != nil || got.Value != 0 {
			t.Fatalf("strike = %+v, %v", got, err)
		}
		// Setting it to what it already is moves nothing.
		if _, err := r.svc.Log(ctx, cmp, pageA, dm, LogInput{TrackerID: tv.ID, Op: OpSet, Amount: 0}); err != nil {
			t.Fatal(err)
		}
		want := []Change{{Owner: owner, EntityID: pageA, From: 0, To: 2}, {Owner: owner, EntityID: pageA, From: 2, To: 0}}
		if !reflect.DeepEqual(h.changes, want) {
			t.Errorf("heard %+v", h.changes)
		}
	})

	t.Run("LogOwned writes a rule's line on one page or every page", func(t *testing.T) {
		r, h, _ := setup(t)
		r.repo.trackers["t-b"] = Tracker{ID: "t-b", CampaignID: cmp, EntityID: "page-b", KindID: "k-need", Name: "Need", Visibility: SeeEveryone}
		none, err := r.svc.LogOwned(ctx, cmp, "", owner, dm, OpAdd, 1, "Rule from Hunger")
		if err != nil || len(none) != 1 {
			t.Fatalf("everywhere before pageA has a row: %v, %v", none, err)
		}
		one, err := r.svc.LogOwned(ctx, cmp, pageA, owner, dm, OpAdd, 3, "Rule from Hunger")
		if err != nil || len(one) != 1 || one[0].Value != 3 {
			t.Fatalf("on pageA: %+v, %v", one, err)
		}
		all, err := r.svc.LogOwned(ctx, cmp, "", owner, dm, OpSet, 0, "Rule from Hunger")
		if err != nil || len(all) != 2 {
			t.Fatalf("everywhere: %+v, %v", all, err)
		}
		for _, e := range r.repo.entries {
			if e.Source != SourceRule || e.Note != "Rule from Hunger" {
				t.Errorf("entry %+v", e)
			}
		}
		if len(h.changes) != 4 {
			t.Errorf("heard %d changes, want 4", len(h.changes))
		}
		for name, err := range map[string]error{
			"player":     func() error { _, err := r.svc.LogOwned(ctx, cmp, pageA, owner, player, OpAdd, 1, ""); return err }(),
			"no owner":   func() error { _, err := r.svc.LogOwned(ctx, cmp, pageA, "", dm, OpAdd, 1, ""); return err }(),
			"other kind": func() error { _, err := r.svc.LogOwned(ctx, cmp, pageA, owner+"x", dm, OpAdd, 1, ""); return err }(),
			"bad op":     func() error { _, err := r.svc.LogOwned(ctx, cmp, pageA, owner, dm, "times", 1, ""); return err }(),
			"gone page":  func() error { _, err := r.svc.LogOwned(ctx, cmp, pageGone, owner, dm, OpAdd, 1, ""); return err }(),
		} {
			if err == nil {
				t.Errorf("%s: no error", name)
			}
		}
	})

	t.Run("Pages lists pages with trackers, by name, for Directors", func(t *testing.T) {
		r, _, _ := setup(t)
		r.addTracker(t, Tracker{ID: "t1", KindID: "k-hand", Name: "Hand"})
		r.addTracker(t, Tracker{ID: "t2", KindID: "k-need", Name: "Need"})
		r.repo.trackers["t3"] = Tracker{ID: "t3", CampaignID: cmp, EntityID: "page-gone", KindID: "k-hand", Name: "Hand"}
		got, err := r.svc.Pages(ctx, cmp, dm)
		if err != nil || !reflect.DeepEqual(got, []PageRef{{ID: pageA, Name: "Page " + pageA}}) {
			t.Fatalf("pages = %v, %v", got, err)
		}
		if _, err := r.svc.Pages(ctx, cmp, player); code(err) != http.StatusForbidden {
			t.Errorf("player: %v", err)
		}
	})
}
