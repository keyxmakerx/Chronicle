package foundry_vtt

import "testing"

// TestModuleSource pins the WebSocket source identifier to its
// wire-protocol value. The literal "foundry-module" is what the
// Foundry module sends as `?client=foundry-module` on its WS upgrade
// URL; changing it without updating the module would silently break
// Foundry-presence tracking on every existing installation.
//
// Regression pin for the magic-string consolidation (NW-2.2 Chunk B).
// Lives next to the const so a future contributor who edits const.go
// sees the assertion immediately.
func TestModuleSource(t *testing.T) {
	if ModuleSource != "foundry-module" {
		t.Errorf("ModuleSource = %q, want %q (wire-protocol value used by Foundry module)", ModuleSource, "foundry-module")
	}
}
