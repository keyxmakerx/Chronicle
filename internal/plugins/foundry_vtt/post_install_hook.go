package foundry_vtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// PostInstallHook implements packages.PostInstallHook for
// PackageTypeFoundryModule. After the packages plugin extracts
// a foundry-module install, this hook:
//
//  1. Loads chronicle-package.json (or applies hardcoded defaults
//     when absent). Fails the install loudly if the descriptor is
//     PRESENT but invalid — silent fallback would mask an upstream
//     contract violation.
//  2. Rewrites the version field in the on-disk module.json (at
//     the descriptor-declared moduleJsonPath) so the served
//     manifest reflects the installed version, not the upstream
//     GitHub release's stale version string.
//
// The hook does NOT rewrite the manifest/download URL fields at
// install-time — those are rewritten per-request at serve-time by
// BuildManifestForCampaign so the URL is per-campaign and per-token-
// version. Baking them in here would defeat per-campaign URLs.
//
// Registration happens at boot in cmd/server/main.go via
// packages.RegisterPostInstallHook (added in C-FMC-5a).
type PostInstallHook struct{}

// NewPostInstallHook constructs the hook. Stateless — kept as a
// constructor for symmetry with the rest of the plugin and to make
// future configuration easy without an API break.
func NewPostInstallHook() *PostInstallHook {
	return &PostInstallHook{}
}

// PackageType identifies which package type this hook handles.
func (h *PostInstallHook) PackageType() packages.PackageType {
	return packages.PackageTypeFoundryModule
}

// AfterInstall is called by the packages plugin after a foundry-
// module package's zip is extracted and the DB row updated.
//
// Errors here fail the install + cause cleanup of destDir
// (handled by the packages plugin's caller per the C-FMC-5a
// fail-loud contract). The operator sees the failure immediately
// instead of debugging "why is Foundry still showing v0.1.0 after
// I installed v0.2.0" hours later.
func (h *PostInstallHook) AfterInstall(ctx context.Context, pkg *packages.Package, version, destDir string) error {
	// 1. Load the descriptor. A missing descriptor is the normal
	//    fallback path; a present-but-invalid descriptor is an
	//    upstream packaging bug we fail loudly.
	desc, err := loadDescriptor(destDir)
	if err != nil && !errors.Is(err, errDescriptorNotFound) {
		// loadDescriptor already wrapped this as *Error.
		return err
	}
	// `desc` is populated regardless — loadDescriptor returns
	// defaultDescriptor() alongside errDescriptorNotFound.

	// 2. Resolve the manifest path inside destDir per the
	//    descriptor's moduleJsonPath.
	manifestPath := filepath.Join(destDir, desc.Package.ModuleJSONPath)

	if err := rewriteModuleJSONVersion(manifestPath, version); err != nil {
		return ErrModuleJSONMissing(manifestPath, err)
	}
	return nil
}

// rewriteModuleJSONVersion reads module.json, sets its "version"
// field to the installed version string, and writes the file back.
// Preserves every other field verbatim (uses a generic
// map[string]any to round-trip unknown keys).
//
// This is the fix for the operator's "Foundry shows v0.1.0 after
// installing v0.2.0" bug — the upstream GitHub release zip ships
// a module.json with a stale version string baked in; Chronicle
// rewrites it on install so the served manifest agrees with the
// DB-tracked version.
//
// File permissions preserved by writing back at 0644 (the
// extraction step already created the file with similar perms).
// File replacement uses os.WriteFile (not rename) — Foundry's
// manifest read is held exclusively by the serve handler, no
// concurrent reader to race with.
func rewriteModuleJSONVersion(path, version string) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	m["version"] = version
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rewritten manifest: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
