// update_mode.go — the per-campaign update choice for a package.
//
// A campaign follows each package it uses in one of three ways. The choice is
// the same for every package type; where it is stored differs by type (see
// CampaignBinding), and everything here stays type-agnostic.
package packages

import (
	"fmt"
	"strings"
)

// UpdateMode is how one campaign follows one package.
type UpdateMode string

const (
	// UpdateModeAutomatic moves the campaign up whenever the site installs a
	// new version. It is what every campaign did before modes existed, so it
	// is the default for anything with no stored choice.
	UpdateModeAutomatic UpdateMode = "automatic"

	// UpdateModePinned keeps the campaign on a chosen version until its owner
	// changes it ("Stay on a version").
	UpdateModePinned UpdateMode = "pinned"

	// UpdateModeApproveFirst keeps the campaign on its current version when a
	// new one is installed, holds the new version, and moves only when the
	// owner (or an admin on their behalf) approves it ("Ask me first").
	UpdateModeApproveFirst UpdateMode = "approve_first"
)

// ParseUpdateMode validates a user-supplied mode. Empty is rejected: callers
// that want "no choice yet" mean UpdateModeAutomatic and must say so.
func ParseUpdateMode(s string) (UpdateMode, error) {
	switch m := UpdateMode(strings.TrimSpace(s)); m {
	case UpdateModeAutomatic, UpdateModePinned, UpdateModeApproveFirst:
		return m, nil
	default:
		return "", fmt.Errorf("unknown update mode %q (want automatic, pinned or approve_first)", s)
	}
}

// Valid reports whether m is one of the three modes.
func (m UpdateMode) Valid() bool {
	_, err := ParseUpdateMode(string(m))
	return err == nil
}

// KeepsVersion reports whether the mode holds a campaign on an explicit
// version (so that version must be protected from clean-up and survive an
// install). Automatic follows the installed version instead.
func (m UpdateMode) KeepsVersion() bool {
	return m == UpdateModePinned || m == UpdateModeApproveFirst
}

// CampaignPackageState is one campaign's standing on one package.
type CampaignPackageState struct {
	CampaignID   string
	CampaignName string
	OwnerName    string
	PackageID    string
	PackageSlug  string
	PackageType  PackageType

	Mode UpdateMode

	// Version is the explicit version the campaign is on. Empty means it has
	// none of its own and follows the installed version (always the case for
	// automatic; transiently possible for the other two until the first
	// install freezes it).
	Version string

	// EffectiveVersion is the version the campaign actually runs: Version, or
	// the package's installed version when Version is empty.
	EffectiveVersion string

	// HeldVersion is a newly installed version waiting for approval. Set only
	// in approve_first mode.
	HeldVersion string

	// Explicit is true when the campaign's mode was chosen through the update
	// modes themselves rather than derived from older stored data. Install
	// refuses to move an explicit campaign it cannot hold; a derived one keeps
	// the older best-effort behaviour.
	Explicit bool
}

// ActorInfo says who is asking, for the audit trail and the admin
// "approve for them" path. Admin is true when a site admin acts on a
// campaign's behalf.
type ActorInfo struct {
	UserID    string
	IP        string
	UserAgent string
	Admin     bool
}

// ReportedPackageMode is how the admin panel reports a package's own mode:
// pinned when it has a pin, or when auto-update is off (so nothing moves it
// either way); automatic otherwise. It is reporting only. It writes nothing
// and changes what no campaign is served, because today an off package still
// serves whatever an admin installs by hand.
func ReportedPackageMode(pkg *Package) UpdateMode {
	if pkg == nil {
		return UpdateModeAutomatic
	}
	if pkg.PinnedVersion != "" || pkg.AutoUpdate == UpdateOff {
		return UpdateModePinned
	}
	return UpdateModeAutomatic
}

// versionLess is the numeric-aware semver comparison shared with clean-up.
func versionLess(a, b string) bool { return pruneVersionLess(a, b) }
