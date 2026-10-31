package main

import (
	"strings"
	"testing"
)

// TestRegisteredPluginsOrder pins the plugin migration run order and that
// every slug is registered once.
func TestRegisteredPluginsOrder(t *testing.T) {
	var slugs []string
	seen := map[string]bool{}
	for _, p := range registeredPlugins() {
		if seen[p.Slug] {
			t.Errorf("plugin slug %q registered twice", p.Slug)
		}
		seen[p.Slug] = true
		slugs = append(slugs, p.Slug)
	}
	want := "bestiary calendar maps sessions timeline widgetbindings systemstate rolltables armory quests pageblocks rumours fieldrules trackers homebrew encounters page_tests dmscreen entity_standing page_layouts page_pins syncapi packages foundry_vtt ai_import_memory ai_connector crafting rolls"
	if got := strings.Join(slugs, " "); got != want {
		t.Errorf("run order =\n%s\nwant\n%s", got, want)
	}
}
