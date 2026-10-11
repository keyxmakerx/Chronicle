package entities

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeTrashRepo serves only what the note counts read; the embedded nil
// interface makes any other repository call fail loudly.
type fakeTrashRepo struct {
	PageSafetyRepository
	items []TrashItem
	ids   map[string][]string
	err   error
}

func (f *fakeTrashRepo) ListTrash(context.Context, string) ([]TrashItem, error) {
	return append([]TrashItem(nil), f.items...), nil
}

func (f *fakeTrashRepo) TrashedPageIDs(_ context.Context, _ string, rootIDs ...string) (map[string][]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if len(rootIDs) == 0 {
		return f.ids, nil
	}
	out := map[string][]string{}
	for _, r := range rootIDs {
		if v, ok := f.ids[r]; ok {
			out[r] = v
		}
	}
	return out, nil
}

type fakeNoteCounter struct {
	counts map[string]int
	err    error
	calls  int
}

func (f *fakeNoteCounter) CountByEntities(context.Context, string, []string) (map[string]int, error) {
	f.calls++
	return f.counts, f.err
}

func TestPageSafety_TrashNoteCounts(t *testing.T) {
	ids := map[string][]string{"root": {"root", "sub1", "sub2"}, "other": {"other"}}
	cases := []struct {
		name     string
		counter  *fakeNoteCounter // nil: no counter wired
		repoErr  error
		wantRoot int
		wantOth  int
	}{
		{"notes on page and sub-pages are summed", &fakeNoteCounter{counts: map[string]int{"root": 1, "sub2": 3, "other": 2}}, nil, 4, 2},
		{"no notes anywhere", &fakeNoteCounter{counts: map[string]int{}}, nil, 0, 0},
		{"no counter wired", nil, nil, 0, 0},
		{"counter failure is swallowed", &fakeNoteCounter{err: errors.New("db down")}, nil, 0, 0},
		{"page lookup failure is swallowed", &fakeNoteCounter{counts: map[string]int{"root": 5}}, errors.New("db down"), 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeTrashRepo{
				items: []TrashItem{{ID: "root"}, {ID: "other"}},
				ids:   ids,
				err:   c.repoErr,
			}
			svc := &pageSafetyService{repo: repo}
			if c.counter != nil {
				svc.SetNoteCounter(c.counter)
			}
			ctx := context.Background()

			items, err := svc.ListTrash(ctx, "camp")
			if err != nil {
				t.Fatalf("ListTrash: %v", err)
			}
			if items[0].NoteCount != c.wantRoot || items[1].NoteCount != c.wantOth {
				t.Errorf("ListTrash counts = %d, %d; want %d, %d", items[0].NoteCount, items[1].NoteCount, c.wantRoot, c.wantOth)
			}
			if got := svc.TrashedNoteCount(ctx, "camp", "root"); got != c.wantRoot {
				t.Errorf("TrashedNoteCount = %d, want %d", got, c.wantRoot)
			}
		})
	}
}

func TestTrashedRedirectURL(t *testing.T) {
	cases := []struct {
		name  string
		notes int
		want  string
	}{
		{"no notes leaves the URL as before", 0, "/campaigns/c1/entities?trashed=e%2F1"},
		{"notes are carried as a count", 4, "/campaigns/c1/entities?trashed=e%2F1&notes=4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := trashedRedirectURL("c1", "e/1", c.notes); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestTrashNotesLine(t *testing.T) {
	deleted := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		notes int
		want  string
	}{
		{"none", 0, ""},
		{"one", 1, "Removed for good on 10 Nov, with the 1 note people wrote on it"},
		{"several", 4, "Removed for good on 10 Nov, with the 4 notes people wrote on it"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := trashNotesLine(TrashItem{DeletedAt: deleted, NoteCount: c.notes}, 30)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
