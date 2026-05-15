package foundry_vtt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// descriptorFilename is the canonical name of the descriptor file
// foundry_vtt reads from the extracted install dir. Defined by the
// FM-PKG-DESCRIPTOR contract on the Foundry side; do not change
// without coordinated bump.
const descriptorFilename = "chronicle-package.json"

// errDescriptorNotFound is returned by loadDescriptor when no
// chronicle-package.json exists in the install dir. The hook treats
// this as a signal to fall back to defaultDescriptor() — NOT as an
// install failure.
//
// Distinct from a parse / schema error: a missing descriptor is
// expected (most current Foundry releases don't ship one yet); a
// PRESENT-BUT-INVALID descriptor is an upstream bug and fails the
// install loudly per the C-FMC-5b agreement.
var errDescriptorNotFound = errors.New("chronicle-package.json not found in install dir")

// loadDescriptor reads chronicle-package.json from the extracted
// install dir. Returns:
//   - (descriptor, nil) on success — schema-validated v1 descriptor
//   - (defaultDescriptor(), errDescriptorNotFound) when the file is
//     absent. The hook treats this as a signal to use defaults.
//   - (nil, *Error{Validation/descriptor_invalid}) when the file
//     exists but parses incorrectly or fails schema validation. The
//     hook FAILS THE INSTALL on this path.
//
// Schema enforcement is deliberately strict: if a module ships a
// descriptor, we trust it; if the descriptor is broken, we don't
// silently fall back to defaults because that would mask a real
// upstream bug. Operators learn about the mismatch immediately
// instead of debugging "why are URLs not being rewritten" hours
// later.
func loadDescriptor(installDir string) (PackageDescriptor, error) {
	path := filepath.Join(installDir, descriptorFilename)
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultDescriptor(), errDescriptorNotFound
		}
		return PackageDescriptor{}, ErrDescriptorInvalid(
			fmt.Errorf("read %s: %w", path, err))
	}

	var desc PackageDescriptor
	if err := json.Unmarshal(bytes, &desc); err != nil {
		return PackageDescriptor{}, ErrDescriptorInvalid(
			fmt.Errorf("parse %s as JSON: %w", path, err))
	}

	if err := validateDescriptor(&desc); err != nil {
		return PackageDescriptor{}, err
	}

	// Fill in any optional fields that v1 leaves empty by overlaying
	// defaults. Lets module authors ship a minimal descriptor (e.g.
	// just schemaVersion + package.id) and inherit sensible
	// behavior for everything else.
	applyDefaults(&desc)

	return desc, nil
}

// validateDescriptor enforces the schema v1 contract. Failures are
// returned as foundry_vtt.Error so the hook can surface them with
// the categorized error format.
func validateDescriptor(d *PackageDescriptor) error {
	// Schema version is the load-bearing field. Unknown major =
	// unrecognized contract; refuse rather than guess. Future v2+
	// requires a coordinated Foundry + Chronicle bump.
	if d.SchemaVersion == 0 {
		return ErrDescriptorInvalid(errors.New(
			"schemaVersion field is required and must be 1 for the current v1 schema"))
	}
	if d.SchemaVersion != currentSchemaVersion {
		return ErrDescriptorInvalid(fmt.Errorf(
			"schemaVersion %d is not supported by this Chronicle build "+
				"(only %d is recognized); upgrade Chronicle or downgrade the module's "+
				"descriptor",
			d.SchemaVersion, currentSchemaVersion))
	}
	// Package.kind must be "foundry-module" for v1. The hook only
	// fires for PackageTypeFoundryModule installs, but enforce the
	// descriptor's self-declaration matches — a mismatch indicates
	// an upstream packaging error (wrong kind in the descriptor)
	// the operator should know about.
	if d.Package.Kind != "" && d.Package.Kind != "foundry-module" {
		return ErrDescriptorInvalid(fmt.Errorf(
			"package.kind=%q but only \"foundry-module\" is supported in v1",
			d.Package.Kind))
	}
	return nil
}

// applyDefaults fills in optional fields the descriptor didn't
// specify, using the defaultDescriptor() values. Lets modules ship
// minimal descriptors and inherit everything else.
//
// IMPORTANT: only fills BLANK fields — never overwrites an
// author-specified value. The descriptor's explicit choice always
// wins over the default.
func applyDefaults(d *PackageDescriptor) {
	def := defaultDescriptor()
	if d.Package.ModuleJSONPath == "" {
		d.Package.ModuleJSONPath = def.Package.ModuleJSONPath
	}
	if len(d.Serving.RewriteFields) == 0 {
		d.Serving.RewriteFields = def.Serving.RewriteFields
	}
	if d.Serving.ManifestEndpoint == "" {
		d.Serving.ManifestEndpoint = def.Serving.ManifestEndpoint
	}
	if d.Serving.DownloadEndpoint == "" {
		d.Serving.DownloadEndpoint = def.Serving.DownloadEndpoint
	}
	// PerCampaignSignedToken: bool default is false, but the v1
	// contract default is TRUE. Apply unconditionally — the absence
	// of an explicit false in a v1 descriptor means "use default."
	// Modules wanting unsigned manifests need to land that as a
	// future schema bump.
	d.Serving.PerCampaignSignedToken = true
}

// defaultDescriptor returns the hardcoded fallback used when no
// chronicle-package.json is present in the install dir. Equivalent
// to the canonical Foundry module's published descriptor — this is
// what every existing foundry-module zip without a descriptor
// implicitly declares.
//
// Defined as a function (not a var) so callers receive a fresh
// instance and can't mutate the shared baseline. Slices are
// allocated fresh per call.
func defaultDescriptor() PackageDescriptor {
	return PackageDescriptor{
		SchemaVersion: currentSchemaVersion,
		Package: PackageDescriptorPackage{
			Kind:           "foundry-module",
			ModuleJSONPath: "module.json",
		},
		Serving: PackageDescriptorServing{
			RewriteFields:          []string{"manifest", "download"},
			ManifestEndpoint:       "/api/v1/campaigns/{campaign_id}/foundry-vtt/module.json?token={token}",
			DownloadEndpoint:       "/api/v1/campaigns/{campaign_id}/foundry-vtt/module.zip?token={token}",
			PerCampaignSignedToken: true,
			ZipContentRoot:         "",
		},
	}
}
