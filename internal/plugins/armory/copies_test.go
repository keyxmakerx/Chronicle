package armory

import (
	"context"
	"reflect"
	"testing"
)

func TestPlanTake(t *testing.T) {
	fine := Copy{ID: 1, ItemID: "p", Quantity: 1, Memory: Memory{Quality: QualityFine}}
	moon := Copy{ID: 2, ItemID: "p", Quantity: 2, Memory: Memory{Marks: []string{"moonlit"}}}
	tests := []struct {
		name        string
		copies      []Copy
		total, take int
		pick        []int64
		want        []int64 // ids leaving, with quantities in wantQty
		wantQty     []int
	}{
		{"plain first", []Copy{fine, moon}, 5, 2, nil, nil, nil},
		{"plain runs out, oldest remembered next", []Copy{fine, moon}, 5, 3, nil, []int64{1}, []int{1}},
		{"all of it", []Copy{fine, moon}, 5, 5, nil, []int64{1, 2}, []int{1, 2}},
		{"a pick goes first", []Copy{fine, moon}, 5, 1, []int64{2}, []int64{2}, []int{1}},
		{"a pick, then plain", []Copy{fine, moon}, 5, 3, []int64{1}, []int64{1}, []int{1}},
		{"an unknown pick is ignored", []Copy{fine, moon}, 5, 1, []int64{9}, nil, nil},
		{"copies over the count are fitted first", []Copy{fine, moon}, 2, 2, nil, []int64{1, 2}, []int{1, 1}},
		{"nothing remembered", nil, 4, 2, nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planTake(tt.copies, tt.total, tt.take, tt.pick)
			var ids []int64
			var qty []int
			for _, c := range got {
				ids, qty = append(ids, c.ID), append(qty, c.Quantity)
			}
			if !reflect.DeepEqual(ids, tt.want) || !reflect.DeepEqual(qty, tt.wantQty) {
				t.Errorf("leaving %v × %v, want %v × %v", ids, qty, tt.want, tt.wantQty)
			}
		})
	}
}

func TestFitCopies(t *testing.T) {
	cs := []Copy{{ID: 1, Quantity: 2}, {ID: 2, Quantity: 3}}
	if got := fitCopies(cs, 3); len(got) != 2 || got[1].Quantity != 1 {
		t.Errorf("fit to 3 = %+v", got)
	}
	if got := fitCopies(cs, 0); len(got) != 0 {
		t.Errorf("fit to 0 = %+v", got)
	}
}

func TestMemoryClean(t *testing.T) {
	m := Memory{Marks: []string{" Moonlit ", "moonlit", "", "Picked at night"}, Quality: "great", How: "stolen", By: "x", OffOn: "never"}.Clean()
	want := Memory{Marks: []string{"Moonlit", "Picked at night"}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("Clean = %+v, want %+v", m, want)
	}
	if !m.HasMark("MOONLIT") || m.HasMark("fresh") {
		t.Error("HasMark is case-blind and exact")
	}
	if !(Memory{}).Empty() || m.Empty() {
		t.Error("Empty")
	}
	made := Memory{How: "made", By: "Asha", On: "3 Frostmoon", OffDay: 40, OffOn: "10 Frostmoon", Becomes: "murky", Quality: QualityFine}
	if got := decodeMemory(encodeMemory(made)); !reflect.DeepEqual(got, made) {
		t.Errorf("round trip = %+v", got)
	}
	if got := decodeMemory("{nope"); !got.Empty() {
		t.Error("an unreadable row is plain")
	}
}

// fakeCopies is CopyStore in memory.
type fakeCopies struct {
	rows  []HeldCopy
	next  int64
	picks map[int64]int64
}

func (f *fakeCopies) List(_ context.Context, _, kind, id, item string) ([]Copy, error) {
	var out []Copy
	for _, r := range f.rows {
		if r.HolderKind == kind && r.HolderID == id && (item == "" || r.ItemID == item) {
			out = append(out, r.Copy)
		}
	}
	return out, nil
}
func (f *fakeCopies) Add(_ context.Context, _, kind, id string, c Copy) (int64, error) {
	f.next++
	c.ID, c.Memory = f.next, c.Memory.Clean()
	f.rows = append(f.rows, HeldCopy{Copy: c, HolderKind: kind, HolderID: id})
	return c.ID, nil
}
func (f *fakeCopies) at(id int64) int {
	for i, r := range f.rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}
func (f *fakeCopies) SetQuantity(_ context.Context, _ string, id int64, q int) error {
	if i := f.at(id); i >= 0 {
		if q <= 0 {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
		} else {
			f.rows[i].Quantity = q
		}
	}
	return nil
}
func (f *fakeCopies) Rehome(_ context.Context, _ string, id int64, kind, hid string) error {
	if i := f.at(id); i >= 0 {
		f.rows[i].HolderKind, f.rows[i].HolderID = kind, hid
	}
	return nil
}
func (f *fakeCopies) Remember(_ context.Context, _ string, id int64, m Memory) error {
	if i := f.at(id); i >= 0 {
		f.rows[i].Memory = m.Clean()
	}
	return nil
}
func (f *fakeCopies) DueOff(_ context.Context, _ string, day int64) ([]HeldCopy, error) {
	var out []HeldCopy
	for _, r := range f.rows {
		if r.Memory.OffDay > 0 && r.Memory.OffDay <= day {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeCopies) Pick(_ context.Context, move, copyID int64) error {
	if f.picks == nil {
		f.picks = map[int64]int64{}
	}
	f.picks[move] = copyID
	return nil
}
func (f *fakeCopies) Picked(_ context.Context, move int64) (int64, error) { return f.picks[move], nil }

// held sums what a holder's copies of an item remember, by quality or first mark.
func (f *fakeCopies) held(kind, id string) map[string]int {
	out := map[string]int{}
	for _, r := range f.rows {
		if r.HolderKind == kind && r.HolderID == id {
			k := r.Memory.Quality
			if k == "" && len(r.Memory.Marks) > 0 {
				k = r.Memory.Marks[0]
			}
			out[k] += r.Quantity
		}
	}
	return out
}

func newCopyFx(t *testing.T) (*fx, *fakeCopies) {
	f := newFx()
	cs := &fakeCopies{}
	f.svc = NewStashService(StashDeps{Repo: f.repo, Directory: f.dir, Visibility: f.vis, Actor: fakeActor{f.dir}, Fields: f.fields, Relations: f.rels, Copies: cs})
	// Thorin carries 3 potions: one fine, one moonlit, one plain.
	cs.Add(context.Background(), "camp", HolderCharacter, "c1", Copy{ItemID: "i1", Quantity: 1, Memory: Memory{Quality: QualityFine}})
	cs.Add(context.Background(), "camp", HolderCharacter, "c1", Copy{ItemID: "i1", Quantity: 1, Memory: Memory{Marks: []string{"moonlit"}}})
	return f, cs
}

func TestMove_CopiesFollowTheCount(t *testing.T) {
	ctx := context.Background()
	gm := Actor{"gm", rOwner}
	move := func(f *fx, q int, pick int64, to Endpoint) {
		t.Helper()
		if _, err := f.svc.Move(ctx, "camp", gm, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: q, From: charEP("c1"), To: to, Pick: pick}); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("plain goes first", func(t *testing.T) {
		f, cs := newCopyFx(t)
		move(f, 1, 0, charEP("c2"))
		if got := cs.held(HolderCharacter, "c2"); len(got) != 0 {
			t.Errorf("Mira got remembered copies %v", got)
		}
		if got := cs.held(HolderCharacter, "c1"); got["fine"] != 1 || got["moonlit"] != 1 {
			t.Errorf("Thorin keeps %v", got)
		}
	})
	t.Run("a pick goes, and into a stash", func(t *testing.T) {
		f, cs := newCopyFx(t)
		sid := f.stash(t, "Chest", "c1")
		move(f, 1, 1, StashEndpoint(sid))
		if got := cs.held(HolderStash, StashEndpoint(sid).ID); got["fine"] != 1 {
			t.Errorf("the chest holds %v", got)
		}
		if got := cs.held(HolderCharacter, "c1"); got["fine"] != 0 || got["moonlit"] != 1 {
			t.Errorf("Thorin keeps %v", got)
		}
	})
	t.Run("past the plain ones, the oldest remembered goes", func(t *testing.T) {
		f, cs := newCopyFx(t)
		move(f, 2, 0, charEP("c2"))
		if got := cs.held(HolderCharacter, "c2"); got["fine"] != 1 || len(got) != 1 {
			t.Errorf("Mira got %v", got)
		}
	})
	t.Run("a waiting request keeps its pick", func(t *testing.T) {
		f, cs := newCopyFx(t)
		out, err := f.svc.Move(ctx, "camp", Actor{"u1", rPlayer}, MoveInput{Kind: MoveKindItem, ItemEntityID: "i1", Quantity: 1, From: charEP("c1"), To: charEP("c2"), Pick: 2})
		if err != nil || out.Applied {
			t.Fatalf("want a waiting request, got %+v, %v", out, err)
		}
		if _, err := f.svc.Approve(ctx, "camp", gm, out.Move.ID); err != nil {
			t.Fatal(err)
		}
		if got := cs.held(HolderCharacter, "c2"); got["moonlit"] != 1 {
			t.Errorf("Mira got %v", got)
		}
	})
}
