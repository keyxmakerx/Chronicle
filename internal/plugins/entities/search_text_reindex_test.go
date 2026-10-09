package entities

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// fakeReindexStore serves candidates from memory and records writes. It
// embeds EntityRepository only so it can be passed where one is expected.
type fakeReindexStore struct {
	EntityRepository
	rows   []SearchTextCandidate
	writes map[string]string
}

func (f *fakeReindexStore) ListSecretSearchCandidates(_ context.Context, afterID string, limit int) ([]SearchTextCandidate, error) {
	var out []SearchTextCandidate
	for _, r := range f.rows {
		if r.ID > afterID && len(out) < limit {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeReindexStore) SetSearchText(_ context.Context, id, text string) error {
	f.writes[id] = text
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].SearchText = text
		}
	}
	return nil
}

func TestReindexSecretSearchText(t *testing.T) {
	secretHTML := `<p>The tavern</p><p><span data-secret="true">the mayor is a lich</span></p>`
	clean := buildSearchText(secretHTML, nil)
	if strings.Contains(clean, "lich") {
		t.Fatalf("buildSearchText kept secret text: %q", clean)
	}

	tests := []struct {
		name       string
		row        SearchTextCandidate
		wantWrite  bool
		wantAbsent string
	}{
		{"stale row with secret text is rewritten", SearchTextCandidate{ID: "e1", EntryHTML: secretHTML, SearchText: "The tavern the mayor is a lich"}, true, "lich"},
		{"row already clean is left alone", SearchTextCandidate{ID: "e2", EntryHTML: secretHTML, SearchText: clean}, false, "lich"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeReindexStore{rows: []SearchTextCandidate{tt.row}, writes: map[string]string{}}
			n, err := ReindexSecretSearchText(context.Background(), store)
			if err != nil {
				t.Fatalf("reindex: %v", err)
			}
			got, wrote := store.writes[tt.row.ID]
			if wrote != tt.wantWrite || (n == 1) != tt.wantWrite {
				t.Fatalf("wrote=%v n=%d, want write %v", wrote, n, tt.wantWrite)
			}
			if wrote && strings.Contains(got, tt.wantAbsent) {
				t.Fatalf("search_text still holds %q: %q", tt.wantAbsent, got)
			}
			// A second run has nothing to do.
			if n2, _ := ReindexSecretSearchText(context.Background(), store); n2 != 0 {
				t.Fatalf("second run rewrote %d rows", n2)
			}
		})
	}
}

// Pages past one batch are all reached.
func TestReindexSecretSearchText_Batches(t *testing.T) {
	store := &fakeReindexStore{writes: map[string]string{}}
	for i := 0; i < 450; i++ {
		store.rows = append(store.rows, SearchTextCandidate{
			ID: fmt.Sprintf("%03d", i), EntryHTML: `<span data-secret="1">x</span>`, SearchText: "x",
		})
	}
	n, err := ReindexSecretSearchText(context.Background(), store)
	if err != nil || n != 450 {
		t.Fatalf("rewrote %d (err %v), want 450", n, err)
	}
}
