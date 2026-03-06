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
		{"dice-roller", "../../extensions/dice-roller/manifest.json"},
		{"example-wasm-rust", "../../extensions/example-wasm-rust/manifest.json"},
		{"example-wasm-go", "../../extensions/example-wasm-go/manifest.json"},
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

// TestExampleWASMPluginManifests validates WASM-specific fields in example plugins.
func TestExampleWASMPluginManifests(t *testing.T) {
	examples := []struct {
		name            string
		path            string
		expectedSlug    string
		expectedCaps    int
		expectedHooks   int
		hasConfig       bool
	}{
		{
			name:          "example-wasm-rust",
			path:          "../../extensions/example-wasm-rust/manifest.json",
			expectedSlug:  "auto-tagger",
			expectedCaps:  4, // log, entity_read, tag_read, tag_write
			expectedHooks: 1, // entity.created
			hasConfig:     true,
		},
		{
			name:          "example-wasm-go",
			path:          "../../extensions/example-wasm-go/manifest.json",
			expectedSlug:  "session-logger",
			expectedCaps:  4, // log, entity_read, calendar_write, kv_store
			expectedHooks: 2, // entity.created, entity.updated
			hasConfig:     true,
		},
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

			if m.Contributes == nil || len(m.Contributes.WASMPlugins) == 0 {
				t.Fatal("expected WASM plugins in contributes")
			}

			wp := m.Contributes.WASMPlugins[0]

			if wp.Slug != ex.expectedSlug {
				t.Errorf("expected slug %q, got %q", ex.expectedSlug, wp.Slug)
			}

			if len(wp.Capabilities) != ex.expectedCaps {
				t.Errorf("expected %d capabilities, got %d: %v", ex.expectedCaps, len(wp.Capabilities), wp.Capabilities)
			}

			if len(wp.Hooks) != ex.expectedHooks {
				t.Errorf("expected %d hooks, got %d: %v", ex.expectedHooks, len(wp.Hooks), wp.Hooks)
			}

			if ex.hasConfig && len(wp.Config) == 0 {
				t.Error("expected config fields")
			}

			if wp.MemoryLimitMB <= 0 || wp.MemoryLimitMB > 256 {
				t.Errorf("memory limit out of range: %d", wp.MemoryLimitMB)
			}

			if wp.TimeoutSecs <= 0 || wp.TimeoutSecs > 300 {
				t.Errorf("timeout out of range: %d", wp.TimeoutSecs)
			}
		})
	}
}
