package extensions

import (
	"os"
	"testing"
)

// TestExampleExtensionManifests validates the bundled example extensions.
func TestExampleExtensionManifests(t *testing.T) {
	examples := []struct {
		name string
		path string
	}{
		{"harptos-calendar", "../../extensions/harptos-calendar/manifest.json"},
		{"dnd5e-character-sheet", "../../extensions/dnd5e-character-sheet/manifest.json"},
	}

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			data, err := os.ReadFile(ex.path)
			if err != nil {
				t.Fatalf("failed to read manifest: %v", err)
			}

			m, err := ParseManifest(data)
			if err != nil {
				t.Fatalf("failed to parse manifest: %v", err)
			}

			if m.ID != ex.name {
				t.Errorf("expected ID %q, got %q", ex.name, m.ID)
			}

			if m.Contributes == nil {
				t.Error("expected contributes section")
			}
		})
	}
}
