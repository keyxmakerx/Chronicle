package campaigns

import (
	"encoding/json"
	"reflect"
	"testing"
)

// nav_editor_test.go pins the contract between the owner's editor
// (static/js/sidebar_editor.js, itemsFromDraft) and the server: what the
// editor saves is accepted as sent and reads back as the very arrangement the
// owner arranged, including rows hidden from players and a turned-off app's
// place.

// editorSave is a body the editor sends: each of the owner's sections ahead
// of its rows, every row naming its section, and every item's visibility.
const editorSave = `{"items":[
	{"type":"app","slug":"characters","visible":true,"section":"pinned"},
	{"type":"category","type_id":2,"visible":true,"section":"pinned"},
	{"type":"app","slug":"maps","visible":false,"section":"apps"},
	{"type":"app","slug":"forge","visible":true,"section":"apps"},
	{"type":"link","id":"lnk_ab12","label":"Wiki","url":"https://example.com/wiki","icon":"fa-globe","visible":true,"section":"apps"},
	{"type":"app","slug":"notes","visible":true,"section":"apps"},
	{"type":"app","slug":"dates","visible":true,"section":"apps"},
	{"type":"category","type_id":1,"visible":true,"section":"categories"},
	{"type":"section","id":"sec_x9","label":"At the table","visible":true},
	{"type":"category","type_id":3,"visible":false,"section":"sec_x9"}
]}`

func TestEditorSave_ReadsBackAsArranged(t *testing.T) {
	var req UpdateSidebarConfigRequest
	if err := json.Unmarshal([]byte(editorSave), &req); err != nil {
		t.Fatalf("editor body is not a sidebar-config request: %v", err)
	}
	items, err := validateSidebarItems(*req.Items)
	if err != nil {
		t.Fatalf("the server refuses what the editor saves: %v", err)
	}
	layout := NormalizeNav(items, navTestApps(), navTestCats())

	want := map[string][]string{
		NavSectionPinned:     {"app:characters", "cat:2"},
		NavSectionApps:       {"app:maps", "app:forge", "link:lnk_ab12", "app:notes", "app:dates"},
		NavSectionCategories: {"cat:1"},
		"sec_x9":             {"cat:3"},
	}
	if got := layoutKeys(layout); !reflect.DeepEqual(got, want) {
		t.Fatalf("arrangement read back as %v, want %v", got, want)
	}
	if got := layout.Sections[3].Label; got != "At the table" {
		t.Errorf("section label = %q", got)
	}
	hidden := map[string]bool{}
	for _, s := range layout.Sections {
		for _, it := range s.Items {
			hidden[it.Key] = it.Hidden
		}
	}
	if !hidden["app:maps"] || !hidden["cat:3"] || hidden["app:notes"] {
		t.Errorf("hidden flags read back wrong: %v", hidden)
	}

	// The turned-off app keeps its place but no one's sidebar shows it, and
	// a player's leaves out both hidden rows.
	player := viewKeys(ViewNav(layout, navTestApps(), navTestCats(), NavViewer{Access: NavAccessMember}))
	wantPlayer := map[string][]string{
		NavSectionPinned:     {"app:characters", "cat:2"},
		NavSectionApps:       {"link:lnk_ab12", "app:notes", "app:dates"},
		NavSectionCategories: {"cat:1"},
		"sec_x9":             {},
	}
	if !reflect.DeepEqual(player, wantPlayer) {
		t.Errorf("player's sidebar = %v, want %v", player, wantPlayer)
	}
}

func TestNavKeyKind(t *testing.T) {
	tests := []struct {
		key, kind, ref string
		ok             bool
	}{
		{"app:maps", NavRowApp, "maps", true},
		{"cat:12", NavRowCategory, "12", true},
		{"link:lnk_1", NavRowLink, "lnk_1", true},
		{"app:", "", "", false},
		{"sub:3", "", "", false},
		{"maps", "", "", false},
	}
	for _, tt := range tests {
		kind, ref, ok := NavKeyKind(tt.key)
		if kind != tt.kind || ref != tt.ref || ok != tt.ok {
			t.Errorf("NavKeyKind(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.key, kind, ref, ok, tt.kind, tt.ref, tt.ok)
		}
	}
}
