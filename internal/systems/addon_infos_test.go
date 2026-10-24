package systems

import "testing"

// AddonInfos hands plain text to the addon rows: the loader's escaping must
// not reach them, or every renderer escapes it a second time.
func TestAddonInfos_UnescapesLoaderText(t *testing.T) {
	m := &SystemManifest{ID: "dnd5e", Name: "D&D 5th Edition", Description: "Rules & <tables>", Author: "Tom & Jo", Status: StatusAvailable}
	sanitizeManifestStrings(m)
	withLoadedSystems(t, map[string]*loadedSystem{"dnd5e": {manifest: m}}, func() {
		infos := AddonInfos()
		if len(infos) != 1 {
			t.Fatalf("got %d infos, want 1", len(infos))
		}
		got := infos[0]
		tests := []struct{ field, got, want string }{
			{"Name", got.Name, "D&D 5th Edition"},
			{"Description", got.Description, "Rules & <tables>"},
			{"Author", got.Author, "Tom & Jo"},
		}
		for _, tt := range tests {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.field, tt.got, tt.want)
			}
		}
	})
}
