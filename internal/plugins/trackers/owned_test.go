package trackers

import (
	"context"
	"net/http"
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
