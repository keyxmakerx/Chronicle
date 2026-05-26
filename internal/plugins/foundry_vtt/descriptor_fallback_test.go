// Pins Chronicle's loadDescriptor fallback (defaultDescriptor) against
// the canonical chronicle-package.json shipped by the Foundry-Module
// repo. If either side drifts, this test fails — pointing at the
// specific field that diverged so the operator knows whether to update
// Chronicle's defaults or regenerate the snapshot.
//
// Snapshot, not live fetch: testdata/chronicle-package.json is a
// committed copy of the canonical at the time of the test commit.
// Chronicle's CI doesn't reach into the Foundry-Module repo on every
// build; the Foundry-Module-side comment in
// `tools/check-package-descriptor.mjs` instructs maintainers to
// regenerate this snapshot whenever they change the canonical. See
// cordinator/decisions/2026-05-22-loadDescriptor-fallback.md for the
// full canonical-vs-fallback table, the package.id exclusion
// rationale, and the snapshot-limitation tradeoff.
package foundry_vtt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestLoadDescriptor_CanonicalMatchesFallback asserts every field of
// the canonical chronicle-package.json matches defaultDescriptor()
// EXCEPT package.id (descriptor-required field; defaultDescriptor()
// deliberately leaves it empty because the default is only used when
// no descriptor is present — see decision doc §"Why package.id is
// excluded").
//
// Per-field assertions (not deep-equal) so a drift failure tells the
// operator exactly which field diverged.
func TestLoadDescriptor_CanonicalMatchesFallback(t *testing.T) {
	bytes, err := os.ReadFile(filepath.Join("testdata", "chronicle-package.json"))
	if err != nil {
		t.Fatalf("read canonical fixture: %v", err)
	}

	var canonical PackageDescriptor
	if err := json.Unmarshal(bytes, &canonical); err != nil {
		t.Fatalf("parse canonical fixture as PackageDescriptor: %v", err)
	}

	fallback := defaultDescriptor()

	if canonical.SchemaVersion != fallback.SchemaVersion {
		t.Errorf("schemaVersion drift: canonical=%d fallback=%d",
			canonical.SchemaVersion, fallback.SchemaVersion)
	}

	// package.id is deliberately excluded: descriptor-required field,
	// not a default. See decision doc §"Why package.id is excluded".

	if canonical.Package.Kind != fallback.Package.Kind {
		t.Errorf("package.kind drift: canonical=%q fallback=%q",
			canonical.Package.Kind, fallback.Package.Kind)
	}
	if canonical.Package.ModuleJSONPath != fallback.Package.ModuleJSONPath {
		t.Errorf("package.moduleJsonPath drift: canonical=%q fallback=%q",
			canonical.Package.ModuleJSONPath, fallback.Package.ModuleJSONPath)
	}

	if !reflect.DeepEqual(canonical.Serving.RewriteFields, fallback.Serving.RewriteFields) {
		t.Errorf("serving.rewriteFields drift: canonical=%v fallback=%v",
			canonical.Serving.RewriteFields, fallback.Serving.RewriteFields)
	}
	if canonical.Serving.ManifestEndpoint != fallback.Serving.ManifestEndpoint {
		t.Errorf("serving.manifestEndpoint drift: canonical=%q fallback=%q",
			canonical.Serving.ManifestEndpoint, fallback.Serving.ManifestEndpoint)
	}
	if canonical.Serving.DownloadEndpoint != fallback.Serving.DownloadEndpoint {
		t.Errorf("serving.downloadEndpoint drift: canonical=%q fallback=%q",
			canonical.Serving.DownloadEndpoint, fallback.Serving.DownloadEndpoint)
	}
	if canonical.Serving.PerCampaignSignedToken != fallback.Serving.PerCampaignSignedToken {
		t.Errorf("serving.perCampaignSignedToken drift: canonical=%v fallback=%v",
			canonical.Serving.PerCampaignSignedToken, fallback.Serving.PerCampaignSignedToken)
	}
	if canonical.Serving.ZipContentRoot != fallback.Serving.ZipContentRoot {
		t.Errorf("serving.zipContentRoot drift: canonical=%q fallback=%q",
			canonical.Serving.ZipContentRoot, fallback.Serving.ZipContentRoot)
	}
}
