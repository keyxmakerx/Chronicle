package systems

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBookWidgets_RetiredFromPalette pins that a widget the package book
// places is still listed (so a page that holds one draws it by name) but
// marked Retired, so the Customizer no longer offers it, and that an edit
// to the book is picked up.
func TestBookWidgets_RetiredFromPalette(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("book/book.yaml", "title: Rules\nparts:\n  - title: One\n    chapters: [credits, missing]\n")
	write("book/chapters/credits.yaml", "title: Credits\npages:\n  - blocks:\n      - type: widget\n        widget: book-credits\n")

	var m SystemManifest
	raw := `{"widgets":[{"slug":"book-credits","name":"Credits","script_file":"w/c.js"},{"slug":"stat-block","name":"Stat block","script_file":"w/s.js"}]}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	retired := func() map[string]bool {
		out := map[string]bool{}
		for _, meta := range placeableWidgetMetas(&m, bookWidgetSlugs(dir)) {
			out[meta.WidgetSlug] = meta.Retired
		}
		return out
	}

	got := retired()
	if len(got) != 2 || !got["book-credits"] || got["stat-block"] {
		t.Fatalf("retired = %v, want book-credits retired and stat-block offered", got)
	}

	// The book stops placing the widget: it is offered again.
	write("book/chapters/credits.yaml", "title: Credits\npages:\n  - blocks:\n      - type: text\n        text: Thanks\n")
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(dir, "book/chapters/credits.yaml"), later, later); err != nil {
		t.Fatal(err)
	}
	if got := retired(); got["book-credits"] {
		t.Errorf("after the edit, book-credits is still retired: %v", got)
	}

	if s := bookWidgetSlugs(t.TempDir()); len(s) != 0 {
		t.Errorf("a system with no book places no widgets, got %v", s)
	}
}
