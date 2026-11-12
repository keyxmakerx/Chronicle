package armory

import (
	"context"
	"testing"
)

// A player's request is authorised when made; these pin that the apply path
// asks again, and that it settles with the server's own view of DM-only lines.

func TestSettlePending_RechecksRequester(t *testing.T) {
	tests := []struct {
		name string
		// change runs between the request and the apply.
		change func(f *fx, sid int)
		from   func(sid int) Endpoint
		to     func(sid int) Endpoint
		// viaSweep settles by opening downtime instead of Approve.
		viaSweep bool
		want     string
	}{
		{
			name:   "hero changes owner, approve",
			change: func(f *fx, _ int) { f.dir.ents["c1"].OwnerUserID = "u2" },
			from:   func(int) Endpoint { return charEP("c1") },
			to:     func(sid int) Endpoint { return StashEndpoint(sid) },
			want:   "The requester no longer controls that character.",
		},
		{
			name:     "hero changes owner, downtime sweep",
			change:   func(f *fx, _ int) { f.dir.ents["c1"].OwnerUserID = "u2" },
			from:     func(int) Endpoint { return charEP("c1") },
			to:       func(sid int) Endpoint { return StashEndpoint(sid) },
			viaSweep: true,
			want:     "The requester no longer controls that character.",
		},
		{
			name:   "removed from destination stash viewers",
			change: func(f *fx, sid int) { f.repo.viewers[sid] = []string{"c2"} },
			from:   func(int) Endpoint { return charEP("c1") },
			to:     func(sid int) Endpoint { return StashEndpoint(sid) },
			want:   "The requester can no longer see that stash.",
		},
		{
			name:   "removed from source stash viewers",
			change: func(f *fx, sid int) { f.repo.viewers[sid] = []string{"c2"} },
			from:   func(sid int) Endpoint { return StashEndpoint(sid) },
			to:     func(int) Endpoint { return charEP("c1") },
			want:   "The requester can no longer see that stash.",
		},
		{
			name:   "destination hero hidden from the requester",
			change: func(f *fx, _ int) { f.vis.hidden["c2"] = true },
			from:   func(int) Endpoint { return charEP("c1") },
			to:     func(int) Endpoint { return charEP("c2") },
			want:   "The requester can no longer see that character.",
		},
		{
			name: "requester left the campaign",
			change: func(f *fx, _ int) {
				f.svc.(*stashService).Members = fakeMembers{roles: map[string]int{"gm": rOwner}}
			},
			from: func(int) Endpoint { return charEP("c1") },
			to:   func(sid int) Endpoint { return StashEndpoint(sid) },
			want: "The requester is no longer in the campaign.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFx()
			ctx := context.Background()
			sid := f.stash(t, "Chest", "c1")
			f.repo.items[sid] = map[string]int{"i1": 5}
			u1, gm := Actor{"u1", rPlayer}, Actor{"gm", rOwner}
			if _, err := f.svc.Move(ctx, "camp", u1, MoveInput{
				Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: tc.from(sid), To: tc.to(sid)}); err != nil {
				t.Fatal(err)
			}
			held, stash := f.carried("c1", "i1"), f.repo.items[sid]["i1"]
			tc.change(f, sid)

			if tc.viaSweep {
				res, err := f.svc.SetDowntime(ctx, "camp", gm, true)
				if err != nil {
					t.Fatal(err)
				}
				if res.Failed != 1 || res.Applied != 0 {
					t.Fatalf("%+v", res)
				}
			} else if _, err := f.svc.Approve(ctx, "camp", gm, f.repo.moves[0].ID); err != nil {
				t.Fatal(err)
			}
			got := f.repo.moves[0]
			if got.Status != MoveFailed || got.Reason != tc.want {
				t.Fatalf("status %q reason %q, want failed %q", got.Status, got.Reason, tc.want)
			}
			if f.carried("c1", "i1") != held || f.repo.items[sid]["i1"] != stash {
				t.Fatal("a refused move changed a balance")
			}
		})
	}
}

func TestSettlePending_StillAppliesWhenAccessHolds(t *testing.T) {
	f := newFx()
	ctx := context.Background()
	sid := f.stash(t, "Chest", "c1")
	if _, err := f.svc.Move(ctx, "camp", Actor{"u1", rPlayer}, MoveInput{
		Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: StashEndpoint(sid)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Approve(ctx, "camp", Actor{"gm", rOwner}, f.repo.moves[0].ID); err != nil {
		t.Fatal(err)
	}
	if f.repo.moves[0].Status != MoveApplied || f.repo.items[sid]["i1"] != 1 {
		t.Fatalf("status %q stash %d", f.repo.moves[0].Status, f.repo.items[sid]["i1"])
	}
}

// The destination holds the item only as a DM-only line, which the requester
// could not see. The unique key means creating a second line conflicts, so the
// apply must find the hidden one and add to it.
func TestSettlePending_AddsToDestinationDmOnlyLine(t *testing.T) {
	f := newFx()
	ctx := context.Background()
	f.rels.rels["c2"] = []*HasItemRelation{{ID: 200, ItemEntityID: "i1", ItemName: "Potion", DmOnly: true, Metadata: []byte(`{"quantity":2}`)}}
	f.rels.createConflict = true
	if _, err := f.svc.Move(ctx, "camp", Actor{"u1", rPlayer}, MoveInput{
		Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Approve(ctx, "camp", Actor{"gm", rOwner}, f.repo.moves[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := f.repo.moves[0]; got.Status != MoveApplied {
		t.Fatalf("status %q reason %q", got.Status, got.Reason)
	}
	if q := f.carried("c2", "i1"); q != 3 {
		t.Fatalf("destination holds %d, want 3", q)
	}
	if q := f.carried("c1", "i1"); q != 2 {
		t.Fatalf("source holds %d, want 2", q)
	}
}
