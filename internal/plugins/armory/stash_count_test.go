package armory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestSetCarried(t *testing.T) {
	ctx := context.Background()
	// c1 (Thorin, u1's) holds 3 × i1, equipped; c2 (Mira, u2's) holds none.
	tests := []struct {
		name      string
		char      string
		qty       int
		want      int
		wantLine  string // "" for a give line, else the stored reason
		wantLines int
		wantBell  bool
	}{
		{"raising is a give", "c1", 5, 5, "", 1, true},
		{"a first count is a give", "c2", 2, 2, "", 1, true},
		{"lowering takes away", "c1", 1, 1, "The GM took away 2 × Potion", 1, false},
		{"zero takes all of it", "c1", 0, 0, "The GM took away 3 × Potion", 1, false},
		{"the same count does nothing", "c1", 3, 3, "", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newGiveFx()
			out, err := g.svc.SetCarried(ctx, "camp", gm, tc.char, "i1", tc.qty)
			if err != nil {
				t.Fatal(err)
			}
			if out.After != tc.want || g.carried(tc.char, "i1") != tc.want {
				t.Fatalf("holds %d, outcome %+v, want %d", g.carried(tc.char, "i1"), out, tc.want)
			}
			if len(g.repo.moves) != tc.wantLines {
				t.Fatalf("history lines %d, want %d", len(g.repo.moves), tc.wantLines)
			}
			if tc.wantLines > 0 {
				m := g.repo.moves[0]
				if !m.IsGive() || m.Reason != tc.wantLine {
					t.Fatalf("line %+v, want reason %q", m, tc.wantLine)
				}
			}
			if got := len(g.notify.users) > 0; got != tc.wantBell {
				t.Fatalf("bell %v, want %v", got, tc.wantBell)
			}
			if tc.qty == 0 {
				for _, r := range g.rels.rels[tc.char] {
					if r.ItemEntityID == "i1" {
						t.Fatal("an emptied line stays")
					}
				}
			}
		})
	}
}

func TestSetCarried_KeepsTheLinesFields(t *testing.T) {
	g := newGiveFx()
	if _, err := g.svc.SetCarried(context.Background(), "camp", gm, "c1", "i1", 2); err != nil {
		t.Fatal(err)
	}
	if meta := string(g.rels.rels["c1"][0].Metadata); !strings.Contains(meta, `"equipped":true`) {
		t.Fatalf("metadata %s", meta)
	}
}

func TestSetCarried_TakesPlainCopiesFirst(t *testing.T) {
	g := newGiveFx()
	cs := &fakeCopies{}
	g.svc = NewStashService(StashDeps{
		Repo: g.repo, Directory: g.dir, Visibility: g.vis, Actor: fakeActor{g.fx.dir},
		Fields: g.fields, Relations: g.rels, Events: g.events, Handouts: g.hand, Notifier: g.notify, Copies: cs,
	})
	ctx := context.Background()
	// Thorin's 3 potions: 1 fine, 2 plain.
	mustAdd(t, cs, "camp", HolderCharacter, "c1", Copy{ItemID: "i1", Quantity: 1, Memory: Memory{Quality: QualityFine}})
	if _, err := g.svc.SetCarried(ctx, "camp", gm, "c1", "i1", 1); err != nil {
		t.Fatal(err)
	}
	if got := cs.held(HolderCharacter, "c1"); got["fine"] != 1 {
		t.Fatalf("left %v, want the fine one kept", got)
	}
	if _, err := g.svc.SetCarried(ctx, "camp", gm, "c1", "i1", 0); err != nil {
		t.Fatal(err)
	}
	if got := cs.held(HolderCharacter, "c1"); got["fine"] != 0 {
		t.Fatalf("left %v after taking all", got)
	}
}

func TestSetCarried_Refusals(t *testing.T) {
	tests := []struct {
		name     string
		who      Actor
		char     string
		item     string
		qty      int
		wantCode int
	}{
		{"a scribe may not", Actor{"s", rScribe}, "c1", "i1", 1, 403},
		{"a player may not", Actor{"u1", rPlayer}, "c1", "i1", 1, 403},
		{"below zero", gm, "c1", "i1", -1, 400},
		{"not a character", gm, "i1", "i1", 1, 404},
		{"not an item", gm, "c1", "c2", 1, 404},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := newGiveFx()
			_, err := g.svc.SetCarried(context.Background(), "camp", tc.who, tc.char, tc.item, tc.qty)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != tc.wantCode {
				t.Fatalf("err %v, want %d", err, tc.wantCode)
			}
			if len(g.repo.moves) != 0 {
				t.Fatal("a refusal wrote history")
			}
		})
	}
}
