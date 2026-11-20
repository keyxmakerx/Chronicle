package entities

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestEveryCoreBlockHasAPicture pins that every widget the core registers
// names a drawing the widget list knows, so a new widget cannot reach the
// Customizer's list as a bare stand-in. The other plugins' widgets are held
// to the same in internal/app.
func TestEveryCoreBlockHasAPicture(t *testing.T) {
	reg := coreBlocks(t)
	metas := reg.Types()
	if len(metas) < 40 {
		t.Fatalf("expected the core widgets, got %d", len(metas))
	}
	for _, m := range metas {
		if m.Picture == "" {
			t.Errorf("%s (%q) names no picture kind", m.Type, m.Label)
			continue
		}
		if !PictureKinds[m.Picture] {
			t.Errorf("%s names picture %q, which is not one of PictureKinds", m.Type, m.Picture)
		}
	}
}

// TestPictureKindsAreDrawn pins that the script draws every kind the server
// can name, so a kind added here is drawn there too.
func TestPictureKindsAreDrawn(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "static", "js", "widgets", "widget_pictures.js"))
	if err != nil {
		t.Fatalf("read widget_pictures.js: %v", err)
	}
	for k := range PictureKinds {
		if !regexp.MustCompile(`(?m)^\s+` + k + `: function`).Match(src) {
			t.Errorf("widget_pictures.js draws no %q", k)
		}
	}
}

// TestBlockLabelsAreSentenceCase pins the widget names the list shows: sentence
// case, and "page" where older names said "entity".
func TestBlockLabelsAreSentenceCase(t *testing.T) {
	reg := coreBlocks(t)
	// Words that keep their capitals wherever they fall in a name.
	names := map[string]bool{"NPC": true, "Foundry": true}
	for _, m := range reg.Types() {
		if strings.Contains(strings.ToLower(m.Label), "entity") {
			t.Errorf("%s is called %q; say page", m.Type, m.Label)
		}
		for i, w := range strings.Fields(m.Label) {
			if i == 0 || names[w] {
				continue
			}
			if w != strings.ToLower(w) {
				t.Errorf("%s is called %q; names are in sentence case", m.Type, m.Label)
				break
			}
		}
	}
}

// coreBlocks registers the core widgets into a fresh registry. Registering
// them also fills the package's category-block table, which other tests read
// as empty, so the table is put back as it was once the test ends.
func coreBlocks(t *testing.T) *BlockRegistry {
	t.Helper()
	extraCategoryBlocks.Lock()
	saved := make(map[string]ConfiguredCategoryBlockRenderer, len(extraCategoryBlocks.byType))
	for k, v := range extraCategoryBlocks.byType {
		saved[k] = v
	}
	extraCategoryBlocks.Unlock()
	t.Cleanup(func() {
		extraCategoryBlocks.Lock()
		extraCategoryBlocks.byType = saved
		extraCategoryBlocks.Unlock()
	})
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	return reg
}

// TestOldEditorFallbackNamesMatchRegistry pins the names the old layout
// editor (layout_editor.js, still mounted by GET /entity-types/:etid/customize)
// lists when it cannot fetch the registry: each must be the registry's own,
// so the two lists cannot drift apart.
func TestOldEditorFallbackNamesMatchRegistry(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "static", "js", "widgets", "layout_editor.js"))
	if err != nil {
		t.Fatalf("read layout_editor.js: %v", err)
	}
	labels := map[string]string{}
	for _, m := range coreBlocks(t).Types() {
		labels[m.Type] = m.Label
	}
	entry := regexp.MustCompile(`\{ type: '([a-z_]+)',\s*label: '([^']+)'`)
	found := entry.FindAllStringSubmatch(string(src), -1)
	if len(found) < 20 {
		t.Fatalf("found only %d fallback entries; the scan has gone wrong", len(found))
	}
	for _, f := range found {
		want, ok := labels[f[1]]
		if !ok {
			continue // a type the core no longer registers keeps its old name
		}
		if f[2] != want {
			t.Errorf("layout_editor.js names %s %q; the registry calls it %q", f[1], f[2], want)
		}
	}
}
