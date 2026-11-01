package entities

import (
	"context"
	"errors"
	"testing"
	"time"
)

// purgeRepo records what the purger asked to remove and lists the Trash.
type purgeRepo struct {
	PageSafetyRepository
	items  []TrashItem
	calls  int
	keep   []string
	cutoff time.Time
}

func (r *purgeRepo) PurgeTrashedBefore(_ context.Context, cutoff time.Time, keep []string) (int, error) {
	r.calls++
	r.cutoff, r.keep = cutoff, keep
	return 0, nil
}

func (r *purgeRepo) ListTrash(context.Context, string) ([]TrashItem, error) {
	return append([]TrashItem(nil), r.items...), nil
}

type fakeKeeper struct {
	ids []string
	err error
}

func (k fakeKeeper) KeptPages(context.Context) ([]string, error) { return k.ids, k.err }

func TestLongTermTrash_Purge(t *testing.T) {
	tests := []struct {
		name      string
		keeper    LongTermTrash
		wantCalls int
		wantKeep  []string
	}{
		{"no keeper removes everything due", nil, 1, nil},
		{"kept pages are left alone", fakeKeeper{ids: []string{"tyne"}}, 1, []string{"tyne"}},
		{"a keeper that cannot answer removes nothing", fakeKeeper{err: errors.New("db down")}, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &purgeRepo{}
			p := &pageSafetyService{repo: repo}
			if tt.keeper != nil {
				p.SetLongTermTrash(tt.keeper)
			}
			p.purgeOnce(context.Background())
			if repo.calls != tt.wantCalls {
				t.Fatalf("purge calls = %d, want %d", repo.calls, tt.wantCalls)
			}
			if len(repo.keep) != len(tt.wantKeep) || (len(tt.wantKeep) > 0 && repo.keep[0] != tt.wantKeep[0]) {
				t.Errorf("keep = %v, want %v", repo.keep, tt.wantKeep)
			}
		})
	}
}

func TestLongTermTrash_ListHidesKeptPages(t *testing.T) {
	now := time.Now().UTC()
	repo := &purgeRepo{items: []TrashItem{
		{ID: "recent", DeletedAt: now.AddDate(0, 0, -2)},
		{ID: "kept", DeletedAt: now.AddDate(0, 0, -(DefaultTrashRetentionDays + 10))},
	}}
	p := &pageSafetyService{repo: repo}
	p.SetLongTermTrash(fakeKeeper{ids: []string{"kept"}})
	items, err := p.ListTrash(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "recent" {
		t.Fatalf("Trash lists %+v; a page past its Trash days is brought back elsewhere", items)
	}
}
