// package_registry.go — central lookup for the "one foundry-module
// per Chronicle instance" assumption. C-FMC-ADMIN-UX-AUDIT Chunk 1.
//
// Pre-audit, the assumption lived in three independent sites:
//
//   1. `internal/plugins/packages/packages.templ:315–342` — the per-
//      version "Campaigns" expand button's DOM ID format
//      `fvtt-campaigns-trigger-<sanitized-version>` (no package-ID
//      prefix; assumes one foundry-module package).
//   2. `internal/plugins/packages/packages.templ:170–191` — the
//      `data-fvtt-versions-trigger="true"` attribute the auto-pin
//      banner's IIFE locates via `document.querySelector`.
//   3. This file's `FoundryPackage` (previously `service.go`
//      `FindFoundryPackage`) — the DB lookup that returns the first
//      `PackageTypeFoundryModule` row.
//
// The audit calls this a refactor with no behavior change. The
// purpose is: a future "support multiple foundry-module packages"
// dispatch only has to revise this file (and the cross-referenced
// templ comments) instead of hunting through three independent sites.
//
// The DOM constants used by packages.templ also live here — see the
// `Fvtt*` exported names below — so the canonical name for any
// foundry-module-specific DOM target binds to this package, not to
// packages.templ. packages.templ continues to inline the string
// values for templ-render reasons (cross-plugin imports point one-way
// from foundry_vtt → packages, never the reverse) but its comments
// cross-reference back here.
//
// Cache invalidation is a stub for Chunk 1. The registry currently
// fetches on every call (same cost as the pre-refactor function).
// A future PR can add a TTL or invalidation-on-package-install cache
// without changing the call sites that go through this file.

package foundry_vtt

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// FvttVersionsTriggerAttr is the data attribute packages.templ
// stamps on the foundry-module package's "Versions" button so the
// auto-pin banner's IIFE can locate it. Site 2 of the
// one-foundry-module assumption sites. Exported so future packages
// templ work can reference the constant instead of duplicating the
// string.
const FvttVersionsTriggerAttr = "data-fvtt-versions-trigger"

// FvttCampaignsTriggerIDPrefix is the prefix packages.templ uses for
// the per-version "Campaigns" expand button. The full ID is
// `<prefix><sanitized-version>` — see FvttCampaignsTriggerID. Site 1
// of the one-foundry-module assumption sites.
const FvttCampaignsTriggerIDPrefix = "fvtt-campaigns-trigger-"

// FvttCampaignsTriggerID returns the canonical DOM ID for the per-
// version "Campaigns" expand button. The sanitization rule (dots /
// plus / slash → hyphen) MUST match packages.sanitizeForID; if either
// drifts, the auto-pin banner's IIFE silently fails to find its
// target. Cross-tested in onclick_handlers_test.go (sanitizer
// lock-step) + this file's tests.
func FvttCampaignsTriggerID(version string) string {
	return FvttCampaignsTriggerIDPrefix + sanitizeVersionForDOMID(version)
}

// PackageRegistry centralizes the "one foundry-module per Chronicle"
// assumption. All foundry_vtt code that needs to find the foundry-
// module package goes through `FoundryPackage`. The pre-refactor
// `FindFoundryPackage` is now a thin wrapper around `FoundryPackage`
// (kept for backward compat with existing call sites; new code should
// prefer the registry directly).
type PackageRegistry struct {
	pkgs PackageReader
}

// NewPackageRegistry constructs a registry backed by the given
// PackageReader. Caller is the service constructor in service.go —
// the registry shares the service's existing packages adapter.
func NewPackageRegistry(pkgs PackageReader) *PackageRegistry {
	return &PackageRegistry{pkgs: pkgs}
}

// FoundryPackage returns the first foundry-module-typed package the
// catalog returns. Pre-refactor this lived as
// `service.FindFoundryPackage`; behavior is identical (first match
// wins). Returns (nil, nil) when no foundry-module package exists —
// callers treat that as the "no package registered" empty state.
//
// Multiple foundry-module packages would be unusual; the assumption
// is documented at the file header. If a future dispatch supports
// multiple, the change lands here and the call sites stay unchanged.
func (r *PackageRegistry) FoundryPackage(ctx context.Context) (*packages.Package, error) {
	if r == nil || r.pkgs == nil {
		return nil, nil
	}
	all, err := r.pkgs.ListPackages(ctx)
	if err != nil {
		return nil, ErrInternal("list_packages", err)
	}
	for i := range all {
		if all[i].Type == packages.PackageTypeFoundryModule {
			return &all[i], nil
		}
	}
	return nil, nil
}

// FoundryPackageID returns just the package ID of the foundry-module
// package, or empty string if none exists. Convenience for callers
// that only need the ID (e.g. template rendering) without the full
// Package struct. Errors are swallowed to empty string — templ code
// paths render the "no package registered" empty state on empty IDs
// anyway, so propagating the error doesn't change behavior.
func (r *PackageRegistry) FoundryPackageID(ctx context.Context) string {
	pkg, err := r.FoundryPackage(ctx)
	if err != nil || pkg == nil {
		return ""
	}
	return pkg.ID
}

// Invalidate is a no-op stub for Chunk 1. Future PRs can wire this
// to a TTL cache + the packages plugin's post-install hook so the
// registry doesn't re-query on every call. Kept here as a named
// extension point so call sites don't have to be re-edited when
// caching turns on.
func (r *PackageRegistry) Invalidate() {
	// Intentional no-op; placeholder for future cache layer.
}

// sanitizeVersionForDOMID lives in onclick_handlers.go; declared
// here as a documentation pointer. Both sides MUST stay in lock-step
// with packages.sanitizeForID — the contract test in
// onclick_handlers_test.go pins the alignment.
var _ = sanitizeVersionForDOMID // ensure the helper stays linked even if all call sites move

// docHelper — silence the "strings imported but not used" if all
// references migrate elsewhere. The import is retained for forward
// compat in case future helpers in this file need string utilities.
var _ = strings.Builder{}
