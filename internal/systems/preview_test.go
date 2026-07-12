// preview_test.go — C-SYSTEMS-REF-SLUG-FIX-R2. Pins two riders on the
// preview/dry-run paths: (1) ZIP preview runs items through the same
// stamp/normalize logic NewJSONProvider applies, so preview counts and
// samples match what installing the package would actually produce; (2)
// preview paths do not mutate the global admin-diagnostics ring as a side
// effect of inspecting a package.
package systems

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// buildTestZIP packages a manifest + one slug-keyed data file into an
// in-memory ZIP shaped like a real system package upload.
func buildTestZIP(t *testing.T, manifestJSON, dataJSON string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mf, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatalf("create manifest entry: %v", err)
	}
	if _, err := mf.Write([]byte(manifestJSON)); err != nil {
		t.Fatalf("write manifest entry: %v", err)
	}

	df, err := zw.Create("data/creatures.json")
	if err != nil {
		t.Fatalf("create data entry: %v", err)
	}
	if _, err := df.Write([]byte(dataJSON)); err != nil {
		t.Fatalf("write data entry: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

const testManifestJSON = `{
	"id": "preview-test-mod",
	"name": "Preview Test Module",
	"api_version": "1",
	"status": "available",
	"categories": [{"slug": "creatures", "name": "Creatures"}]
}`

// TestPreviewFromZIP_NormalizeParity pins the exact case rider 3 calls out:
// a slug-keyed item (id ← slug at load time) must preview with the same ID
// it would install with, and an item skipped at install time (duplicate
// normalized ID) must not inflate the preview's item count either.
func TestPreviewFromZIP_NormalizeParity(t *testing.T) {
	dataJSON := `[
		{"slug": "goblin-sniper", "name": "Goblin Sniper", "summary": "slug-keyed"},
		{"id": "goblin-sniper", "name": "Goblin Sniper (dup)", "summary": "shadowing duplicate"}
	]`
	zipData := buildTestZIP(t, testManifestJSON, dataJSON)

	result, err := PreviewFromZIP(zipData)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid preview, got errors: %v", result.Errors)
	}
	if len(result.Categories) != 1 {
		t.Fatalf("got %d categories, want 1", len(result.Categories))
	}

	cat := result.Categories[0]
	// The duplicate must be dropped from the preview, exactly as it would
	// be dropped from an actual install — otherwise preview overstates the
	// item count and shows a sample the install would never produce.
	if cat.ItemCount != 1 {
		t.Fatalf("preview item count = %d, want 1 (duplicate must be dropped, matching install)", cat.ItemCount)
	}
	if len(cat.Samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(cat.Samples))
	}
	if got := cat.Samples[0].ID; got != "goblin-sniper" {
		t.Errorf("sample ID = %q, want %q (slug must be normalized into ID at preview time too)", got, "goblin-sniper")
	}
	if got := cat.Samples[0].Name; got != "Goblin Sniper" {
		t.Errorf("sample Name = %q, want the first (slug-keyed) item to survive, not the duplicate", got)
	}

	// Now confirm an actual install of the same data produces the same ID —
	// preview and install must agree.
	dir := t.TempDir()
	writeRawTestData(t, dir, "creatures", dataJSON)
	globalEventLog = NewEventLog(10)
	provider, err := NewJSONProvider("preview-test-mod", dir)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	installed, err := provider.List("creatures")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(installed) != cat.ItemCount {
		t.Fatalf("installed count = %d, preview count = %d — must match", len(installed), cat.ItemCount)
	}
	if installed[0].ID != cat.Samples[0].ID {
		t.Errorf("installed ID = %q, preview ID = %q — must match", installed[0].ID, cat.Samples[0].ID)
	}
}

// TestPreviewFromPackage_DoesNotPolluteGlobalDiagnostics pins rider 2: a
// preview is a dry run over a package an admin is merely inspecting (e.g.
// approving from GitHub), and must not evict real load history from the
// fixed-capacity global diagnostics ring just because that package happens
// to contain a bad data item.
func TestPreviewFromPackage_DoesNotPolluteGlobalDiagnostics(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(testManifestJSON), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	// A neither-id-nor-slug item would normally trigger a skip diagnostic.
	writeRawTestData(t, dataDir, "creatures", `[{"name": "Unaddressable", "summary": "s"}]`)

	globalEventLog = NewEventLog(10)
	before := len(DiagnosticEvents())

	result, err := PreviewFromPackage(dir)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid preview, got errors: %v", result.Errors)
	}

	after := len(DiagnosticEvents())
	if after != before {
		t.Errorf("PreviewFromPackage recorded %d global diagnostic event(s); a dry-run preview must not mutate global state", after-before)
	}
}
