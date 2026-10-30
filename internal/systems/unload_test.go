package systems

import (
	"path/filepath"
	"testing"
)

// Unloading a package's folder drops the systems loaded from it or from a
// folder inside it, and nothing else.
func TestUnloadPackageDirsUnder(t *testing.T) {
	root := filepath.Join("media", "store", "systems", "pack")
	l := &SystemLoader{
		modules: map[string]*loadedSystem{
			"pack":        {manifest: &SystemManifest{ID: "pack"}, dir: filepath.Join(root, "1.0.0"), source: "package"},
			"pack-extra":  {manifest: &SystemManifest{ID: "pack-extra"}, dir: filepath.Join(root, "1.0.0", "extra"), source: "package"},
			"packaged":    {manifest: &SystemManifest{ID: "packaged"}, dir: root + "ed/1.0.0", source: "package"},
			"bundled-one": {manifest: &SystemManifest{ID: "bundled-one"}, dir: filepath.Join(root, "1.0.0"), source: "bundled"},
		},
		systemInstances: map[string]System{},
	}
	got := l.unloadPackageDirsUnder(root)
	if len(got) != 2 {
		t.Fatalf("dropped %v, want pack and pack-extra", got)
	}
	for _, id := range []string{"pack", "pack-extra"} {
		if l.modules[id] != nil {
			t.Errorf("%s is still loaded", id)
		}
	}
	for _, id := range []string{"packaged", "bundled-one"} {
		if l.modules[id] == nil {
			t.Errorf("%s was dropped", id)
		}
	}
}
