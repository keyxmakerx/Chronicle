// Tests for the descriptor loader — the two paths the operator
// cares about most:
//
//  1. Missing chronicle-package.json → fallback to defaults
//     (the normal case; modules without FM-PKG-DESCRIPTOR shipped yet)
//  2. Present chronicle-package.json → schema-validated parse
//     (the new case; modules opting into descriptor-driven serve)
//
// Plus: present-but-invalid descriptor fails LOUDLY (the operator's
// fail-loud contract from C-FMC-5a). No silent fallback on parse
// failure or schema mismatch.
package foundry_vtt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDescriptor_Missing — no chronicle-package.json on disk.
// Expected: returns defaultDescriptor() + errDescriptorNotFound
// sentinel so callers (the hook) can distinguish "fall back to
// defaults" from "fail loudly".
func TestLoadDescriptor_Missing(t *testing.T) {
	dir := t.TempDir() // empty dir; no descriptor

	desc, err := loadDescriptor(dir)
	if !errors.Is(err, errDescriptorNotFound) {
		t.Fatalf("expected errDescriptorNotFound, got: %v", err)
	}
	// Verify the returned descriptor matches the documented defaults.
	if desc.Package.ModuleJSONPath != "module.json" {
		t.Errorf("default moduleJsonPath should be 'module.json', got %q", desc.Package.ModuleJSONPath)
	}
	if got := desc.Serving.RewriteFields; len(got) != 2 || got[0] != "manifest" || got[1] != "download" {
		t.Errorf("default rewriteFields should be [manifest, download], got %v", got)
	}
	if !desc.Serving.PerCampaignSignedToken {
		t.Errorf("default perCampaignSignedToken should be true")
	}
	if desc.SchemaVersion != currentSchemaVersion {
		t.Errorf("default schemaVersion should be %d, got %d", currentSchemaVersion, desc.SchemaVersion)
	}
}

// TestLoadDescriptor_PresentAndValid — full v1 descriptor on disk.
// Expected: parsed values surface verbatim; nil error.
func TestLoadDescriptor_PresentAndValid(t *testing.T) {
	dir := t.TempDir()
	body := `{
		"schemaVersion": 1,
		"package": {
			"id": "chronicle-sync",
			"kind": "foundry-module",
			"moduleJsonPath": "dist/module.json"
		},
		"serving": {
			"rewriteFields": ["manifest", "download", "esmodules"],
			"manifestEndpoint": "/custom/manifest/{campaign_id}?t={token}",
			"downloadEndpoint": "/custom/dl/{campaign_id}?t={token}",
			"perCampaignSignedToken": true,
			"zipContentRoot": "dist"
		}
	}`
	writeFile(t, dir, descriptorFilename, body)

	desc, err := loadDescriptor(dir)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if desc.Package.ID != "chronicle-sync" {
		t.Errorf("Package.ID mismatch: %q", desc.Package.ID)
	}
	if desc.Package.ModuleJSONPath != "dist/module.json" {
		t.Errorf("ModuleJSONPath mismatch: %q", desc.Package.ModuleJSONPath)
	}
	if len(desc.Serving.RewriteFields) != 3 {
		t.Errorf("RewriteFields length mismatch: %v", desc.Serving.RewriteFields)
	}
	if desc.Serving.ManifestEndpoint != "/custom/manifest/{campaign_id}?t={token}" {
		t.Errorf("ManifestEndpoint not preserved: %q", desc.Serving.ManifestEndpoint)
	}
}

// TestLoadDescriptor_MinimalDescriptorInheritsDefaults — modules can
// ship a minimal descriptor (just schemaVersion + package.id) and
// inherit defaults for everything else. Lets module authors opt
// into the descriptor contract without specifying boilerplate.
func TestLoadDescriptor_MinimalDescriptorInheritsDefaults(t *testing.T) {
	dir := t.TempDir()
	body := `{"schemaVersion": 1, "package": {"id": "chronicle-sync"}}`
	writeFile(t, dir, descriptorFilename, body)

	desc, err := loadDescriptor(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Package.ModuleJSONPath != "module.json" {
		t.Errorf("missing moduleJsonPath should default to 'module.json', got %q", desc.Package.ModuleJSONPath)
	}
	if len(desc.Serving.RewriteFields) != 2 {
		t.Errorf("missing rewriteFields should default to length 2, got %v", desc.Serving.RewriteFields)
	}
}

// TestLoadDescriptor_InvalidJSON — descriptor present but malformed.
// Expected: ErrDescriptorInvalid (foundry_vtt.Error) — hook FAILS
// the install, doesn't silently fall back. Validates the fail-loud
// contract for upstream packaging bugs.
func TestLoadDescriptor_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, descriptorFilename, `{not valid json`)

	_, err := loadDescriptor(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	fe := AsError(err)
	if fe == nil {
		t.Fatalf("expected *Error, got %T", err)
	}
	if fe.Code != "descriptor_invalid" {
		t.Errorf("expected code 'descriptor_invalid', got %q", fe.Code)
	}
	if fe.Category != ErrCategoryValidation {
		t.Errorf("expected validation category, got %q", fe.Category)
	}
}

// TestLoadDescriptor_UnknownMajorVersion — schemaVersion=99 is an
// unrecognized major. Hook MUST refuse rather than guess at v1
// semantics. Future v2 requires a Chronicle build upgrade.
func TestLoadDescriptor_UnknownMajorVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, descriptorFilename, `{"schemaVersion": 99, "package": {"id": "x"}}`)

	_, err := loadDescriptor(dir)
	if err == nil {
		t.Fatal("expected error for unknown schemaVersion, got nil")
	}
	fe := AsError(err)
	if fe == nil || fe.Code != "descriptor_invalid" {
		t.Errorf("expected descriptor_invalid error, got %v", err)
	}
}

// TestLoadDescriptor_MissingSchemaVersion — schemaVersion is the
// load-bearing field. Absent (zero-valued) must fail loudly, NOT
// silently apply v1 semantics. A module that forgets schemaVersion
// has an upstream bug worth surfacing.
func TestLoadDescriptor_MissingSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, descriptorFilename, `{"package": {"id": "x"}}`)

	_, err := loadDescriptor(dir)
	if err == nil {
		t.Fatal("expected error for missing schemaVersion, got nil")
	}
}

// TestLoadDescriptor_WrongKind — package.kind != "foundry-module"
// (e.g. "system") fails validation. The hook only fires for
// foundry-module installs, but the descriptor's self-declaration
// mismatch indicates the operator may have packaged the wrong kind.
func TestLoadDescriptor_WrongKind(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, descriptorFilename,
		`{"schemaVersion": 1, "package": {"id": "x", "kind": "system"}}`)

	_, err := loadDescriptor(dir)
	if err == nil {
		t.Fatal("expected error for wrong package.kind, got nil")
	}
}

// writeFile is a tiny helper that creates a file with the given
// content under dir. Keeps the test bodies focused on the test
// scenario rather than os.WriteFile noise.
func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
