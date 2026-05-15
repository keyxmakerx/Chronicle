package foundry_modules

import (
	"encoding/json"
	"time"
)

// ModuleStatus mirrors the SQL ENUM. Held as a typed string so the
// service layer can rely on the compiler to spot bad transitions
// (no magic strings in business logic).
type ModuleStatus string

const (
	// StatusAvailable — the version is install-able and resolves to
	// "the latest version" for unpinned campaigns. The default state on upload.
	StatusAvailable ModuleStatus = "available"

	// StatusDeprecated — existing pinned campaigns still resolve to
	// this version (so the world doesn't break mid-session) but the
	// owner-side selector renders a warning and the LatestAvailable
	// lookup skips deprecated versions. Reversible.
	StatusDeprecated ModuleStatus = "deprecated"

	// StatusWithdrawn — the version is gone. The manifest endpoint
	// 404s for pinned campaigns; the version disappears from every
	// selector. Used for security revocations; reversible only by
	// re-uploading with a bumped version number.
	StatusWithdrawn ModuleStatus = "withdrawn"
)

// IsValidStatus reports whether s is one of the three defined states.
// Used at the handler boundary to reject hand-typed status strings.
func IsValidStatus(s ModuleStatus) bool {
	switch s {
	case StatusAvailable, StatusDeprecated, StatusWithdrawn:
		return true
	}
	return false
}

// VersionSource records where a catalog row came from. Manual uploads
// are the original path (admin clicks "Upload New Version"); github_release
// is the auto-fetch path (background poller pulls GitHub releases).
type VersionSource string

const (
	// SourceManualUpload — admin clicked "Upload New Version" and
	// chose a .zip file. Used for forks, pre-release testing, and
	// any version that doesn't have a GitHub release behind it.
	SourceManualUpload VersionSource = "manual_upload"

	// SourceGitHubRelease — background poller discovered a GitHub
	// release with a chronicle-sync.zip asset and ingested it. The
	// github_release_id + github_release_tag columns are populated;
	// uploaded_by_user_id is NULL.
	SourceGitHubRelease VersionSource = "github_release"
)

// ModuleVersion is one row of foundry_module_versions — a Foundry VTT
// module .zip the admin has uploaded, plus the parsed manifest metadata.
type ModuleVersion struct {
	ID                    string          `json:"id"`
	Version               string          `json:"version"`
	FilePath              string          `json:"-"` // server path; never exposed to clients
	FileSize              int64           `json:"file_size"`
	ContentSHA256         string          `json:"content_sha256"`
	ManifestJSON          json.RawMessage `json:"manifest"`
	CompatibilityMinimum  string          `json:"compatibility_minimum,omitempty"`
	CompatibilityVerified string          `json:"compatibility_verified,omitempty"`
	CompatibilityMaximum  string          `json:"compatibility_maximum,omitempty"`
	Status                ModuleStatus    `json:"status"`
	ReleaseNotes          string          `json:"release_notes,omitempty"`
	// UploadedByUserID is NULL for github_release-sourced rows
	// (poller has no user identity), so it's modeled as a nullable
	// pointer. Manual uploads always populate this.
	UploadedByUserID *string         `json:"uploaded_by_user_id,omitempty"`
	UploadedAt       time.Time       `json:"uploaded_at"`

	// Source records the origin (manual_upload vs github_release)
	// so the catalog UI can render a "Source: GitHub vX.Y.Z" link
	// or "Manual upload" tag, and so the operator can tell whether
	// turning off the poller would leave a row orphaned.
	Source VersionSource `json:"source"`

	// GitHubReleaseTag is the tag name (e.g. "v0.1.5") of the GitHub
	// release this row was sourced from. Empty for manual uploads.
	GitHubReleaseTag string `json:"github_release_tag,omitempty"`

	// GitHubReleaseID is the GitHub API's numeric release ID. Used
	// for the UNIQUE constraint that makes the poller idempotent —
	// a second poll of the same release ID hits the key and the
	// INSERT errors out. Zero / nil for manual uploads.
	GitHubReleaseID *int64 `json:"github_release_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CampaignToken is one row of foundry_module_campaign_tokens — the per-
// campaign version counter that gates manifest URL signatures. Rotating
// = bumping TokenVersion, which invalidates every URL signed against
// the previous value.
type CampaignToken struct {
	CampaignID   string    `json:"campaign_id"`
	TokenVersion int       `json:"token_version"`
	RotatedAt    time.Time `json:"rotated_at"`
}

// CampaignUsage is the renderable row for the admin's
// "campaigns using this version" panel. Joined fields are pulled in
// the repository's CampaignsUsingVersion query.
type CampaignUsage struct {
	CampaignID   string     `json:"campaign_id"`
	CampaignName string     `json:"campaign_name"`
	OwnerUserID  string     `json:"owner_user_id"`
	OwnerName    string     `json:"owner_name"`
	PinnedTo     string     `json:"pinned_to"`              // version or "" when latest-tracking
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
}

// ParsedManifest is the subset of a Foundry module.json the admin
// upload step needs to populate the catalog row. The full manifest
// is preserved verbatim in ManifestJSON; this struct is for parsing
// only.
type ParsedManifest struct {
	ID            string             `json:"id"`
	Version       string             `json:"version"`
	Title         string             `json:"title,omitempty"`
	Description   string             `json:"description,omitempty"`
	Compatibility *ManifestCompatibility `json:"compatibility,omitempty"`
}

// ManifestCompatibility mirrors the Foundry "compatibility" block.
type ManifestCompatibility struct {
	Minimum  string `json:"minimum,omitempty"`
	Verified string `json:"verified,omitempty"`
	Maximum  string `json:"maximum,omitempty"`
}
