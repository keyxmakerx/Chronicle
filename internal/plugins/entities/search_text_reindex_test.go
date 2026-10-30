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

func (f *fakeReindexStore) ListSearchCandidates(_ context.Context, typeID int, afterID string, limit int) ([]SearchTextCandidate, error) {
	var out []SearchTextCandidate
	for _, r := range f.rows {
		if r.ID > afterID && len(out) < limit && (typeID == 0 || r.EntityTypeID == typeID) {
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

// reindexTypes is category 1, with a GM-only and an owner-only field; any
// other category can't be read.
func reindexTypes() *mockEntityTypeRepo {
	return &mockEntityTypeRepo{findByIDFn: func(_ context.Context, id int) (*EntityType, error) {
		if id != 1 {
			return nil, fmt.Errorf("no such category")
		}
		return &EntityType{ID: 1, Fields: []FieldDefinition{{Key: "gm_notes", GMOnly: true}, {Key: "backstory", OwnerOnly: true}, {Key: "title"}}}, nil
	}}
}

func TestReindexSearchText(t *testing.T) {
	secretHTML := `<p>The tavern</p><p><span data-secret="true">the mayor is a lich</span></p>`
	clean := buildSearchText(secretHTML, nil)
	if strings.Contains(clean, "lich") {
		t.Fatalf("buildSearchText kept secret text: %q", clean)
	}

	mentionHTML := `<p>The tavern of <a data-mention-id="aaaaaaaa-0000-4000-8000-000000000009">The lich king</a></p>`
	fields := map[string]any{"gm_notes": "a lich", "backstory": "lich-born", "title": "Barkeep"}
	tests := []struct {
		name       string
		row        SearchTextCandidate
		wantWrite  bool
		wantAbsent string
	}{
		{"stale row with secret text is rewritten", SearchTextCandidate{ID: "e1", EntityTypeID: 1, EntryHTML: secretHTML, SearchText: "The tavern the mayor is a lich"}, true, "lich"},
		{"row already clean is left alone", SearchTextCandidate{ID: "e2", EntityTypeID: 1, EntryHTML: secretHTML, SearchText: clean}, false, "lich"},
		{"mention name is dropped", SearchTextCandidate{ID: "e3", EntityTypeID: 1, EntryHTML: mentionHTML, SearchText: "The tavern of The lich king"}, true, "lich"},
		{"GM-only and owner-only values are dropped", SearchTextCandidate{ID: "e4", EntityTypeID: 1, FieldsData: fields, SearchText: "a lich lich-born Barkeep"}, true, "lich"},
		{"a field added to the page as GM-only is dropped", SearchTextCandidate{ID: "e5", EntityTypeID: 1, FieldsData: map[string]any{"secret_name": "Lich"}, FieldOverrides: &FieldOverrides{Added: []FieldDefinition{{Key: "secret_name", GMOnly: true}}}, SearchText: "Lich"}, true, "Lich"},
		{"a category that can't be read indexes no values", SearchTextCandidate{ID: "e6", EntityTypeID: 2, FieldsData: fields, SearchText: "a lich"}, true, "lich"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeReindexStore{rows: []SearchTextCandidate{tt.row}, writes: map[string]string{}}
			n, err := ReindexSearchText(context.Background(), store, reindexTypes(), 0)
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
			if n2, _ := ReindexSearchText(context.Background(), store, reindexTypes(), 0); n2 != 0 {
				t.Fatalf("second run rewrote %d rows", n2)
			}
		})
	}
}

// Pages past one batch are all reached.
func TestReindexSearchText_Batches(t *testing.T) {
	store := &fakeReindexStore{writes: map[string]string{}}
	for i := 0; i < 450; i++ {
		store.rows = append(store.rows, SearchTextCandidate{
			ID: fmt.Sprintf("%03d", i), EntryHTML: `<span data-secret="1">x</span>`, SearchText: "x",
		})
	}
	n, err := ReindexSearchText(context.Background(), store, reindexTypes(), 0)
	if err != nil || n != 450 {
		t.Fatalf("rewrote %d (err %v), want 450", n, err)
	}
}

// Open fields still reach search; only the restricted ones stay out.
func TestReindexSearchText_KeepsOpenFields(t *testing.T) {
	store := &fakeReindexStore{writes: map[string]string{}, rows: []SearchTextCandidate{
		{ID: "e1", EntityTypeID: 1, FieldsData: map[string]any{"gm_notes": "spy", "title": "Barkeep"}, SearchText: "spy Barkeep"},
	}}
	if _, err := ReindexSearchText(context.Background(), store, reindexTypes(), 1); err != nil {
		t.Fatal(err)
	}
	if got := store.writes["e1"]; got != "Barkeep" {
		t.Errorf("search_text = %q, want %q", got, "Barkeep")
	}
}
