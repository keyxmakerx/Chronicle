// Tests for the bake-on-import zip repack that rewrites the Foundry-module
// manifest's manifest/download URLs without touching any other field.
package packages

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFoundryZip writes a fresh zip with the given module.json contents and
// the provided extra files at the given paths. Returns the zip path on disk.
func buildFoundryZip(t *testing.T, dir string, moduleJSON []byte, extras map[string][]byte) string {
	t.Helper()
	zipPath := filepath.Join(dir, "test-module.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	mw, err := w.Create("module.json")
	if err != nil {
		t.Fatalf("create module.json entry: %v", err)
	}
	if _, err := mw.Write(moduleJSON); err != nil {
		t.Fatalf("write module.json: %v", err)
	}
	for name, data := range extras {
		ew, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %q entry: %v", name, err)
		}
		if _, err := ew.Write(data); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return zipPath
}

// readZipEntry returns the bytes of the named entry inside zipPath. Fails
// the test if the entry is missing.
func readZipEntry(t *testing.T, zipPath, entryName string) []byte {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer r.Close()
	for _, e := range r.File {
		if e.Name != entryName {
			continue
		}
		f, err := e.Open()
		if err != nil {
			t.Fatalf("open entry: %v", err)
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("read entry: %v", err)
		}
		return data
	}
	t.Fatalf("entry %q not found in zip", entryName)
	return nil
}

// TestRepackFoundryZip_RewritesManifestAndDownload confirms the bake-on-import
// repack changes only the manifest+download fields and leaves everything else
// (including non-module.json files) untouched.
func TestRepackFoundryZip_RewritesManifestAndDownload(t *testing.T) {
	src := map[string]any{
		"id":            "drawsteel-foundry",
		"version":       "1.2.3",
		"manifest":      "https://github.com/owner/repo/releases/latest/download/module.json",
		"download":      "https://github.com/owner/repo/releases/download/v1.2.3/module.zip",
		"esmodules":     []string{"scripts/module.js"},
		"styles":        []string{"styles/module.css"},
		"compatibility": map[string]any{"minimum": "12", "verified": "12.331"},
	}
	moduleJSON, err := json.MarshalIndent(src, "", "  ")
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}

	scriptBytes := []byte("// payload\nconsole.log('hi');\n")
	zipPath := buildFoundryZip(t, t.TempDir(), moduleJSON, map[string][]byte{
		"scripts/module.js": scriptBytes,
	})

	const baseURL = "https://chronicle.example.test"
	if err := repackFoundryZip(zipPath, baseURL); err != nil {
		t.Fatalf("repack: %v", err)
	}

	// The repacked module.json should have manifest/download rewritten and
	// every other field preserved by value.
	rewritten := readZipEntry(t, zipPath, "module.json")
	var got map[string]any
	if err := json.Unmarshal(rewritten, &got); err != nil {
		t.Fatalf("unmarshal rewritten module.json: %v", err)
	}

	if want := baseURL + "/foundry-module/module.json"; got["manifest"] != want {
		t.Errorf("manifest = %q, want %q", got["manifest"], want)
	}
	if want := baseURL + "/foundry-module/download"; got["download"] != want {
		t.Errorf("download = %q, want %q", got["download"], want)
	}
	if got["id"] != "drawsteel-foundry" {
		t.Errorf("id changed: got %v", got["id"])
	}
	if got["version"] != "1.2.3" {
		t.Errorf("version changed: got %v", got["version"])
	}
	// esmodules and styles round-trip through JSON as []any of strings.
	if esm, ok := got["esmodules"].([]any); !ok || len(esm) != 1 || esm[0] != "scripts/module.js" {
		t.Errorf("esmodules changed: got %v", got["esmodules"])
	}
	if compat, ok := got["compatibility"].(map[string]any); !ok || compat["verified"] != "12.331" {
		t.Errorf("compatibility changed: got %v", got["compatibility"])
	}

	// The non-module.json file must round-trip byte-for-byte.
	if gotScript := readZipEntry(t, zipPath, "scripts/module.js"); !bytes.Equal(gotScript, scriptBytes) {
		t.Errorf("scripts/module.js corrupted: got %q want %q", gotScript, scriptBytes)
	}
}

// TestRepackFoundryZip_TrimsTrailingSlash confirms the baked URLs don't
// contain a double slash if BASE_URL was configured with a trailing one.
func TestRepackFoundryZip_TrimsTrailingSlash(t *testing.T) {
	moduleJSON := []byte(`{"id":"x","manifest":"old","download":"old"}`)
	zipPath := buildFoundryZip(t, t.TempDir(), moduleJSON, nil)

	if err := repackFoundryZip(zipPath, "https://chronicle.example.test/"); err != nil {
		t.Fatalf("repack: %v", err)
	}

	rewritten := readZipEntry(t, zipPath, "module.json")
	if strings.Contains(string(rewritten), "//foundry-module/") {
		t.Errorf("trailing slash leaked into baked URL:\n%s", rewritten)
	}
}

// TestRepackFoundryZip_RejectsEmptyBaseURL confirms we refuse to bake the
// zip with a blank URL — that would silently produce a broken on-disk
// manifest in Foundry.
func TestRepackFoundryZip_RejectsEmptyBaseURL(t *testing.T) {
	moduleJSON := []byte(`{"id":"x","manifest":"old","download":"old"}`)
	zipPath := buildFoundryZip(t, t.TempDir(), moduleJSON, nil)

	if err := repackFoundryZip(zipPath, ""); err == nil {
		t.Fatal("expected error on empty baseURL, got nil")
	}
}

// TestRepackFoundryZip_NoModuleJSON confirms we fail loudly if the zip is
// shaped wrong (no module.json at root) — better to surface the mismatch
// than to silently ship a broken zip to Foundry.
func TestRepackFoundryZip_NoModuleJSON(t *testing.T) {
	zipPath := buildFoundryZip(t, t.TempDir(), nil, map[string][]byte{
		"README.md": []byte("nope"),
	})
	// buildFoundryZip always writes a module.json — overwrite the zip with a
	// custom one that has no module.json at all.
	zipPath = filepath.Join(filepath.Dir(zipPath), "no-module.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	w := zip.NewWriter(f)
	other, _ := w.Create("README.md")
	_, _ = other.Write([]byte("hi"))
	_ = w.Close()
	_ = f.Close()

	if err := repackFoundryZip(zipPath, "https://chronicle.example.test"); err == nil {
		t.Fatal("expected error when module.json missing, got nil")
	}
}

// TestRepackFoundryZip_AtomicReplace confirms the repack uses a tmp file and
// rename rather than truncating the original in place — this matters because
// FoundryModuleZipPath does an os.Stat and serves the file by path, and any
// reader that opens the original mid-repack must see a consistent zip.
func TestRepackFoundryZip_AtomicReplace(t *testing.T) {
	moduleJSON := []byte(`{"id":"x","manifest":"old","download":"old"}`)
	dir := t.TempDir()
	zipPath := buildFoundryZip(t, dir, moduleJSON, nil)

	if err := repackFoundryZip(zipPath, "https://chronicle.example.test"); err != nil {
		t.Fatalf("repack: %v", err)
	}

	// No leftover .repack.tmp file should remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".repack.tmp") {
			t.Errorf("leftover temp file after repack: %s", e.Name())
		}
	}
}
