package homebrew

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// syncLog records what an owner's PartSync was asked, in order.
type syncLog struct {
	calls   []string
	refuse  bool
	applied map[string][]string // recipe id -> part keys last applied
}

func (l *syncLog) hook(kind string) *PartSync {
	keys := func(ps []Part) []string {
		out := []string{}
		for _, p := range ps {
			out = append(out, p.Key)
		}
		return out
	}
	return &PartSync{
		Check: func(_ context.Context, c, id string, ps []Part) error {
			l.calls = append(l.calls, fmt.Sprintf("check %s %s %v", kind, id, keys(ps)))
			if l.refuse {
				return apperror.NewValidation("the owner refuses")
			}
			return nil
		},
		Apply: func(_ context.Context, c, id string, ps []Part) error {
			l.calls = append(l.calls, fmt.Sprintf("apply %s %s %v", kind, id, keys(ps)))
			l.applied[id] = keys(ps)
			return nil
		},
	}
}

func syncedService(t *testing.T) (Service, *fakeRepo, *syncLog) {
	t.Helper()
	reg := testRegistry(t)
	l := &syncLog{applied: map[string][]string{}}
	if err := reg.RegisterPart(PartKind{Name: "kept", Label: "Kept", Check: okCheck, Sync: l.hook("kept")}); err != nil {
		t.Fatal(err)
	}
	repo := newFakeRepo()
	svc := NewService(repo, reg, nil, nil).(*service)
	svc.newID = func() string { return "rcp-1" }
	return svc, repo, l
}

func TestPartSync(t *testing.T) {
	ctx := context.Background()
	kept := func(key string) Part { return Part{Key: key, Kind: "kept", Label: key, See: SeeParty} }

	t.Run("create checks, stores, then applies only that kind's parts", func(t *testing.T) {
		svc, repo, l := syncedService(t)
		r, err := svc.Create(ctx, camp, dm, RecipeInput{Name: "R", Doc: Doc{Parts: []Part{w("a"), kept("k1"), kept("k2")}}})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"check kept rcp-1 [k1 k2]", "apply kept rcp-1 [k1 k2]"}
		if strings.Join(l.calls, "|") != strings.Join(want, "|") || repo.recipes[r.ID].ID == "" {
			t.Fatalf("calls = %v", l.calls)
		}
	})

	t.Run("a refusal stores nothing", func(t *testing.T) {
		svc, repo, l := syncedService(t)
		l.refuse = true
		if _, err := svc.Create(ctx, camp, dm, RecipeInput{Name: "R", Doc: Doc{Parts: []Part{kept("k1")}}}); err == nil {
			t.Fatal("refused sync still created")
		}
		if len(repo.recipes) != 0 || len(l.applied) != 0 {
			t.Fatal("something was stored")
		}
	})

	t.Run("update applies the new picture; a lost race applies nothing", func(t *testing.T) {
		svc, repo, l := syncedService(t)
		r, err := svc.Create(ctx, camp, dm, RecipeInput{Name: "R", Doc: Doc{Parts: []Part{kept("k1")}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Update(ctx, camp, r.ID, dm, RecipeInput{Name: "R", Version: r.Version, Doc: Doc{Parts: []Part{kept("k2")}}}); err != nil {
			t.Fatal(err)
		}
		if got := l.applied[r.ID]; len(got) != 1 || got[0] != "k2" {
			t.Fatalf("applied %v", got)
		}
		repo.loseRace = true
		n := len(l.calls)
		if _, err := svc.Update(ctx, camp, r.ID, dm, RecipeInput{Name: "R", Version: r.Version + 1, Doc: Doc{}}); err == nil {
			t.Fatal("lost race succeeded")
		}
		for _, c := range l.calls[n:] {
			if strings.HasPrefix(c, "apply") {
				t.Fatalf("applied after a lost race: %v", l.calls[n:])
			}
		}
	})

	t.Run("delete applies an empty picture", func(t *testing.T) {
		svc, _, l := syncedService(t)
		r, err := svc.Create(ctx, camp, dm, RecipeInput{Name: "R", Doc: Doc{Parts: []Part{kept("k1")}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Delete(ctx, camp, r.ID, dm); err != nil {
			t.Fatal(err)
		}
		if got, ok := l.applied[r.ID]; !ok || len(got) != 0 {
			t.Fatalf("applied %v", got)
		}
	})

	t.Run("a Sync needs both halves", func(t *testing.T) {
		err := NewRegistry().RegisterPart(PartKind{Name: "half", Check: okCheck, Sync: &PartSync{Check: (&syncLog{}).hook("x").Check}})
		if err == nil {
			t.Fatal("a Sync without Apply was accepted")
		}
	})
}
