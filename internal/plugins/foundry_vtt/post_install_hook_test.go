// Tests for PostInstallHook — the operator's version-stale bug fix.
// The hook reads chronicle-package.json (or falls back) and rewrites
// module.json's version field. These tests pin the contract:
//
//   - Hook reports the right PackageType.
//   - Fallback path: missing descriptor → defaults → module.json
//     version rewritten.
//   - Descriptor-driven path: descriptor declares moduleJsonPath →
//     hook rewrites at the declared path, not "module.json".
//   - Failure path: descriptor present-but-invalid → fail loudly.
package foundry_vtt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// TestPostInstallHook_PackageType pins the hook's PackageType
// declaration. Failure here would cause the packages plugin's
// dispatcher to skip the hook entirely, masking version-stale bugs.
func TestPostInstallHook_PackageType(t *testing.T) {
	h := NewPostInstallHook()
	if h.PackageType() != packages.PackageTypeFoundryModule {
		t.Errorf("expected PackageTypeFoundryModule, got %q", h.PackageType())
	}
}

// TestPostInstallHook_FallbackRewritesModuleJSON — no descriptor on
// disk → defaults apply → hook reads "module.json" at the install
// root and overwrites the version field with the installed version.
//
// This is THE test for the version-stale bug fix. If this fails,
// Foundry will continue to see the upstream GitHub release's
// version string instead of what Chronicle reports as installed.
func TestPostInstallHook_FallbackRewritesModuleJSON(t *testing.T) {
	dir := t.TempDir()
	// Upstream module.json with the stale upstream version baked in.
	// This is exactly the shape that triggered the operator's bug
	// (DB says v0.1.10, served manifest says v0.0.1).
	original := `{
		"id": "chronicle-sync",
		"version": "v0.0.1-upstream-stale",
		"manifest": "https://github.com/example/upstream/releases/latest/module.json",
		"download": "https://github.com/example/upstream/releases/download/v0.0.1/module.zip"
	}`
	writeFile(t, dir, "module.json", original)

	h := NewPostInstallHook()
	pkg := &packages.Package{Type: packages.PackageTypeFoundryModule, Slug: "chronicle-sync"}
	if err := h.AfterInstall(context.Background(), pkg, "v0.1.10", "", dir); err != nil {
		t.Fatalf("AfterInstall returned error: %v", err)
	}

	got := readJSON(t, filepath.Join(dir, "module.json"))
	if got["version"] != "v0.1.10" {
		t.Errorf("expected version 'v0.1.10' after rewrite, got %q", got["version"])
	}
	// Other fields preserved verbatim — the hook ONLY touches version.
	if got["id"] != "chronicle-sync" {
		t.Errorf("id field should be preserved, got %q", got["id"])
	}
	if got["manifest"] != "https://github.com/example/upstream/releases/latest/module.json" {
		t.Errorf("manifest field should NOT be rewritten by the hook (rewriter handles that at serve time)")
	}
}

// TestPostInstallHook_DescriptorOverridesModuleJSONPath — descriptor
// declares moduleJsonPath="dist/module.json". Hook must rewrite at
// THAT path, not the root "module.json" (which doesn't even exist
// in this layout).
func TestPostInstallHook_DescriptorOverridesModuleJSONPath(t *testing.T) {
	dir := t.TempDir()
	// chronicle-package.json at root declares nested module.json.
	writeFile(t, dir, descriptorFilename,
		`{"schemaVersion": 1, "package": {"id": "x", "moduleJsonPath": "dist/module.json"}}`)
	// module.json at the descriptor-declared path; intentionally
	// NOT at the root (would be a hook bug if root path is read).
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("dist", "module.json"),
		`{"id": "x", "version": "stale"}`)

	h := NewPostInstallHook()
	pkg := &packages.Package{Type: packages.PackageTypeFoundryModule, Slug: "x"}
	if err := h.AfterInstall(context.Background(), pkg, "v1.2.3", "", dir); err != nil {
		t.Fatalf("AfterInstall returned error: %v", err)
	}

	got := readJSON(t, filepath.Join(dir, "dist", "module.json"))
	if got["version"] != "v1.2.3" {
		t.Errorf("expected nested module.json rewritten to v1.2.3, got %q", got["version"])
	}
}

// TestPostInstallHook_InvalidDescriptorFailsLoudly — descriptor
// present but parse-fails. Hook MUST return an error so packages'
// install fails + cleans up destDir. Silent fallback would hide
// the upstream packaging bug for hours.
func TestPostInstallHook_InvalidDescriptorFailsLoudly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, descriptorFilename, `not valid json`)
	writeFile(t, dir, "module.json", `{"id": "x", "version": "stale"}`)

	h := NewPostInstallHook()
	pkg := &packages.Package{Type: packages.PackageTypeFoundryModule}
	err := h.AfterInstall(context.Background(), pkg, "v1.0.0", "", dir)
	if err == nil {
		t.Fatal("expected hook to fail on invalid descriptor, got nil error")
	}
	// Verify the categorized error shape — Foundry's FM-CSU-DIAG
	// will read this format.
	fe := AsError(err)
	if fe == nil {
		t.Fatalf("expected *Error, got %T", err)
	}
	if fe.Code != "descriptor_invalid" {
		t.Errorf("expected code 'descriptor_invalid', got %q", fe.Code)
	}
}

// TestPostInstallHook_MissingModuleJSON — destDir exists but
// module.json (at the descriptor-declared path) is missing. Hook
// surfaces this as the "module_json_missing" error so the operator
// learns the upstream zip is malformed instead of getting silent
// install failures.
func TestPostInstallHook_MissingModuleJSON(t *testing.T) {
	dir := t.TempDir() // empty — no descriptor, no module.json

	h := NewPostInstallHook()
	pkg := &packages.Package{Type: packages.PackageTypeFoundryModule}
	err := h.AfterInstall(context.Background(), pkg, "v1.0.0", "", dir)
	if err == nil {
		t.Fatal("expected hook to fail when module.json is missing, got nil error")
	}
	fe := AsError(err)
	if fe == nil || fe.Code != "module_json_missing" {
		t.Errorf("expected module_json_missing error, got %v", err)
	}
}

// readJSON unmarshals a file as a generic map. Used to assert on
// individual fields after the rewrite without re-encoding the
// whole struct.
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return m
}
