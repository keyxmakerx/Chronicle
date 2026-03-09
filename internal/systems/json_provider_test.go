package systems

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeTestData creates a JSON file in the given directory with the
// specified items. Returns the file path.
func writeTestData(t *testing.T, dir, category string, items []ReferenceItem) {
	t.Helper()
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal test data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, category+".json"), data, 0o644); err != nil {
		t.Fatalf("write test data: %v", err)
	}
}

func TestNewJSONProvider(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		wantCats  int
		wantErr   bool
	}{
		{
			name: "loads valid data directory",
			setup: func(t *testing.T, dir string) {
				writeTestData(t, dir, "spells", []ReferenceItem{
					{ID: "fireball", Name: "Fireball", Summary: "A ball of fire"},
					{ID: "shield", Name: "Shield", Summary: "A protective barrier"},
				})
				writeTestData(t, dir, "monsters", []ReferenceItem{
					{ID: "goblin", Name: "Goblin", Summary: "A small creature"},
				})
			},
			wantCats: 2,
		},
		{
			name:     "empty directory returns no categories",
			setup:    func(t *testing.T, dir string) {},
			wantCats: 0,
		},
		{
			name: "invalid JSON returns error",
			setup: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not valid json"), 0o644); err != nil {
					t.Fatalf("write bad.json: %v", err)
				}
			},
			wantErr: true,
		},
		{
			name: "non-json files are skipped",
			setup: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o644); err != nil {
					t.Fatalf("write readme.txt: %v", err)
				}
				writeTestData(t, dir, "items", []ReferenceItem{
					{ID: "sword", Name: "Sword"},
				})
			},
			wantCats: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			p, err := NewJSONProvider("test-mod", dir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			cats := p.Categories()
			if len(cats) != tt.wantCats {
				t.Errorf("categories: got %d, want %d", len(cats), tt.wantCats)
			}
		})
	}
}

func TestJSONProvider_List(t *testing.T) {
	dir := t.TempDir()
	writeTestData(t, dir, "spells", []ReferenceItem{
		{ID: "fireball", Name: "Fireball"},
		{ID: "shield", Name: "Shield"},
	})

	p, err := NewJSONProvider("test", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	items, err := p.List("spells")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}

	// Verify module ID is stamped.
	for _, item := range items {
		if item.SystemID != "test" {
			t.Errorf("item %s: system_id = %q, want %q", item.ID, item.SystemID, "test")
		}
	}

	// Unknown category returns empty slice.
	empty, err := p.List("nonexistent")
	if err != nil {
		t.Fatalf("list nonexistent: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty slice, got %d items", len(empty))
	}
}

func TestJSONProvider_Get(t *testing.T) {
	dir := t.TempDir()
	writeTestData(t, dir, "spells", []ReferenceItem{
		{ID: "fireball", Name: "Fireball"},
		{ID: "shield", Name: "Shield"},
	})

	p, err := NewJSONProvider("test", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	tests := []struct {
		name     string
		category string
		id       string
		wantName string
		wantNil  bool
	}{
		{name: "existing item", category: "spells", id: "fireball", wantName: "Fireball"},
		{name: "missing item", category: "spells", id: "nonexistent", wantNil: true},
		{name: "missing category", category: "monsters", id: "goblin", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, err := p.Get(tt.category, tt.id)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if tt.wantNil {
				if item != nil {
					t.Errorf("expected nil, got %+v", item)
				}
				return
			}
			if item == nil {
				t.Fatal("expected item, got nil")
			}
			if item.Name != tt.wantName {
				t.Errorf("name = %q, want %q", item.Name, tt.wantName)
			}
		})
	}
}

func TestJSONProvider_Search(t *testing.T) {
	dir := t.TempDir()
	writeTestData(t, dir, "spells", []ReferenceItem{
		{ID: "fireball", Name: "Fireball", Summary: "A ball of fire", Tags: []string{"evocation", "fire"}},
		{ID: "shield", Name: "Shield", Summary: "A protective barrier", Tags: []string{"abjuration"}},
		{ID: "cure-wounds", Name: "Cure Wounds", Summary: "Heal a creature", Tags: []string{"healing"}},
	})

	p, err := NewJSONProvider("test", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{name: "match by name", query: "fire", wantCount: 1},
		{name: "match by summary", query: "protective", wantCount: 1},
		{name: "match by tag", query: "healing", wantCount: 1},
		{name: "case insensitive", query: "FIRE", wantCount: 1},
		{name: "empty query", query: "", wantCount: 0},
		{name: "no matches", query: "zzz", wantCount: 0},
		{name: "multiple matches", query: "a", wantCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := p.Search(tt.query)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}
