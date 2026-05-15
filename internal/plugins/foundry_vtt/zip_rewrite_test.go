// Tests for the C-FMC-7 per-campaign zip-rewrite path. The load-
// bearing contract: when the download endpoint streams a zip, the
// embedded module.json carries the per-campaign Chronicle URLs —
// NOT the upstream GitHub URLs from the source zip. Foundry's
// update checks read this on-disk module.json forever after
// extraction, so getting these bytes right is the difference
// between update checks hitting Chronicle vs. hitting GitHub.
//
// Coverage:
//
//   1. zipDirToWriterWithRewrite replaces the descriptor-declared
//      module.json with the rewritten bytes; every other file
//      copies byte-for-byte.
//   2. chronicle-package.json is excluded from the zip output.
//   3. Nested moduleJSONPath ("dist/module.json") works.
//   4. Path normalization: descriptor variants like "./module.json"
//      still match.
//
// The end-to-end "BuildDownloadParams returns per-campaign URLs"
// path is covered by the existing manifest-rewrite tests since
// both paths share resolveCampaignManifest.
package foundry_vtt

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestZipDirToWriterWithRewrite_ReplacesModuleJSON — the load-
// bearing test. Set up an install dir with a stale module.json +
// other files; rewrite to a payload simulating Chronicle URLs; verify
// the streamed zip has the rewritten module.json and unmodified
// siblings.
func TestZipDirToWriterWithRewrite_ReplacesModuleJSON(t *testing.T) {
	dir := t.TempDir()
	stale := `{"id":"x","manifest":"https://github.com/example/upstream/module.json","download":"https://github.com/example/upstream/module.zip"}`
	writeFile(t, dir, "module.json", stale)
	writeFile(t, dir, "LICENSE", "MIT")
	if err := os.Mkdir(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("scripts", "init.js"), "console.log('hi');")

	rewritten := []byte(`{"id":"x","manifest":"https://chronicle.test/api/v1/campaigns/CAMP1/foundry-vtt/module.json?token=TOK","download":"https://chronicle.test/api/v1/campaigns/CAMP1/foundry-vtt/module.zip?token=TOK"}`)

	var buf bytes.Buffer
	if err := zipDirToWriterWithRewrite(dir, "module.json", rewritten, &buf); err != nil {
		t.Fatalf("rewrite error: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip parse: %v", err)
	}
	entries := zipEntries(t, r)

	// module.json content matches the rewritten payload (not the stale source).
	if got := entries["module.json"]; got != string(rewritten) {
		t.Errorf("module.json in zip should be the rewritten bytes\nwant: %s\ngot:  %s", rewritten, got)
	}
	// CRITICAL: zip's module.json MUST NOT contain the GitHub URL.
	// If this regresses, Foundry's update check reverts to GitHub.
	if strings.Contains(entries["module.json"], "github.com") {
		t.Error("module.json in zip still contains a github.com URL — Foundry update checks will bypass Chronicle")
	}
	if !strings.Contains(entries["module.json"], "chronicle.test") {
		t.Error("module.json in zip should contain a chronicle.test URL")
	}

	// Other files preserved byte-for-byte.
	if got := entries["LICENSE"]; got != "MIT" {
		t.Errorf("LICENSE not preserved verbatim: %q", got)
	}
	if got := entries["scripts/init.js"]; got != "console.log('hi');" {
		t.Errorf("scripts/init.js not preserved verbatim: %q", got)
	}
}

// TestZipDirToWriterWithRewrite_ExcludesDescriptor — the
// chronicle-package.json file in the install dir should NOT appear
// in the streamed zip. Foundry has no use for it; leaking it into
// the client filesystem is a small information disclosure (the
// descriptor schema becomes visible to anyone who downloads the
// module) but mainly it's clutter.
func TestZipDirToWriterWithRewrite_ExcludesDescriptor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "module.json", `{"id":"x"}`)
	writeFile(t, dir, descriptorFilename,
		`{"schemaVersion":1,"package":{"id":"x"}}`)

	var buf bytes.Buffer
	if err := zipDirToWriterWithRewrite(dir, "module.json", []byte(`{"id":"x"}`), &buf); err != nil {
		t.Fatalf("rewrite error: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := zipEntries(t, r)
	if _, present := entries[descriptorFilename]; present {
		t.Error("chronicle-package.json should NOT appear in the streamed zip")
	}
}

// TestZipDirToWriterWithRewrite_NestedModuleJSONPath — descriptor
// declares moduleJsonPath="dist/module.json"; the file is at that
// nested path. The rewriter must find + replace at the nested path,
// NOT touch a root-level module.json if one happened to coexist.
func TestZipDirToWriterWithRewrite_NestedModuleJSONPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("dist", "module.json"),
		`{"id":"x","manifest":"https://github.com/upstream"}`)
	// A decoy file at the root — must NOT be replaced.
	writeFile(t, dir, "module.json", "decoy at root")

	rewritten := []byte(`{"id":"x","manifest":"https://chronicle.test/m"}`)
	var buf bytes.Buffer
	if err := zipDirToWriterWithRewrite(dir, "dist/module.json", rewritten, &buf); err != nil {
		t.Fatalf("rewrite error: %v", err)
	}

	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := zipEntries(t, r)

	// Nested file got the rewrite.
	if got := entries["dist/module.json"]; got != string(rewritten) {
		t.Errorf("nested module.json should be rewritten, got: %s", got)
	}
	// Root decoy was NOT touched.
	if got := entries["module.json"]; got != "decoy at root" {
		t.Errorf("root module.json should be left as a verbatim copy when the descriptor points elsewhere, got: %s", got)
	}
}

// TestZipDirToWriterWithRewrite_PathNormalization — descriptors may
// declare module.json's path as "./module.json" rather than the
// cleaned form. The rewriter must still find the file via
// filepath.Clean normalization.
func TestZipDirToWriterWithRewrite_PathNormalization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "module.json", "stale")

	rewritten := []byte("rewritten")
	var buf bytes.Buffer
	if err := zipDirToWriterWithRewrite(dir, "./module.json", rewritten, &buf); err != nil {
		t.Fatalf("rewrite error: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	entries := zipEntries(t, r)
	if got := entries["module.json"]; got != string(rewritten) {
		t.Errorf("path normalization failed: ./module.json didn't match module.json. Got: %q", got)
	}
}

// zipEntries reads a zip archive into a map of {filename: content}
// for easy assertion-by-key. Test helper.
func zipEntries(t *testing.T, r *zip.Reader) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %q: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %q: %v", f.Name, err)
		}
		out[f.Name] = string(body)
	}
	return out
}
