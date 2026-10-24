package systems

import "testing"

// AddonInfos hands the manifest's text to the addon rows as written: every
// renderer escapes it, so escaping or unescaping here would change what
// players read ("D&amp;D", or a literal "&amp;" turned into "&").
func TestAddonInfos_PlainText(t *testing.T) {
	m := &SystemManifest{ID: "dnd5e", Name: "D&D 5th Edition", Description: "Rules & <tables>", Author: "Tom &amp; Jo", Status: StatusAvailable}
	withLoadedSystems(t, map[string]*loadedSystem{"dnd5e": {manifest: m}}, func() {
		infos := AddonInfos()
		if len(infos) != 1 {
			t.Fatalf("got %d infos, want 1", len(infos))
		}
		got := infos[0]
		tests := []struct{ field, got, want string }{
			{"Name", got.Name, "D&D 5th Edition"},
			{"Description", got.Description, "Rules & <tables>"},
			{"Author", got.Author, "Tom &amp; Jo"},
		}
		for _, tt := range tests {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
			}
		}
	})
}
