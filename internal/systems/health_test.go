package systems

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("function playEntrance(){}\n")
	if err := os.WriteFile(filepath.Join(dir, "widgets", "character-sheet.js"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	got := fingerprintFiles(dir, []string{"widgets/character-sheet.js", "widgets/missing.js"})
	if len(got) != 2 {
		t.Fatalf("expected 2 fingerprints, got %d", len(got))
	}

	present := got[0]
	if !present.Exists {
		t.Error("expected the existing file to be marked Exists")
	}
	if present.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", present.Size, len(content))
	}
	if len(present.SHA256) != 16 {
		t.Errorf("sha256 = %q, want 16 hex chars", present.SHA256)
	}
	if present.ModTime == "" {
		t.Error("expected a non-empty mtime")
	}

	missing := got[1]
	if missing.Exists || missing.SHA256 != "" {
		t.Errorf("missing file should be Exists=false with no hash, got %+v", missing)
	}
}

func TestFingerprintFiles_ContentSensitive(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.js"), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.js"), []byte("new-and-different"), 0o644)
	fps := fingerprintFiles(dir, []string{"a.js", "b.js"})
	if fps[0].SHA256 == fps[1].SHA256 {
		t.Error("different content must produce different hashes (the whole point of the diagnostic)")
	}
}

func TestHealthFilePaths_DedupesAndIncludesManifest(t *testing.T) {
	m := &SystemManifest{
		Widgets: []WidgetDef{
			{Slug: "character-sheet", ScriptFile: "widgets/character-sheet.js"},
			{Slug: "dup", ScriptFile: "widgets/character-sheet.js"}, // duplicate path
		},
		TextRenderers: []TextRendererDef{{Slug: "ref", File: "widgets/reference-renderer.js"}},
	}
	paths := healthFilePaths(m)
	// manifest.json always first, then unique script/file paths.
	if paths[0] != "manifest.json" {
		t.Errorf("first path = %q, want manifest.json", paths[0])
	}
	count := map[string]int{}
	for _, p := range paths {
		count[p]++
	}
	if count["widgets/character-sheet.js"] != 1 {
		t.Errorf("duplicate widget path not deduped: %v", paths)
	}
	if count["widgets/reference-renderer.js"] != 1 {
		t.Errorf("text-renderer file missing: %v", paths)
	}
}

func TestSystemLoaderHealth(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"id":"drawsteel"}`), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "widgets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "widgets", "character-sheet.js"), []byte("// sheet"), 0o644)

	l := NewSystemLoader("")
	l.modules["drawsteel"] = &loadedSystem{
		manifest: &SystemManifest{
			ID: "drawsteel", Name: "Draw Steel", Version: "0.13.0",
			Widgets: []WidgetDef{{Slug: "character-sheet", ScriptFile: "widgets/character-sheet.js"}},
		},
		dir:    dir,
		source: "package",
	}

	h := l.Health()
	if len(h) != 1 {
		t.Fatalf("expected 1 system, got %d", len(h))
	}
	sys := h[0]
	if sys.ID != "drawsteel" || sys.Version != "0.13.0" || sys.Source != "package" || sys.Dir != dir {
		t.Errorf("unexpected system health: %+v", sys)
	}
	// Both the manifest and the widget file should be fingerprinted + present.
	byPath := map[string]FileFingerprint{}
	for _, f := range sys.Files {
		byPath[f.Path] = f
	}
	if !byPath["manifest.json"].Exists || !byPath["widgets/character-sheet.js"].Exists {
		t.Errorf("expected manifest.json + widget to exist: %+v", sys.Files)
	}
}
