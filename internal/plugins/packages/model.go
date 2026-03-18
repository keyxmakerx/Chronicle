// Package packages manages external package repositories for Chronicle.
// It provides version management, auto-updates, and admin UI for game
// system packs and the Foundry VTT module pulled from GitHub repos.
package packages

import (
	"crypto/rand"
	"fmt"
	"io"
	"time"
)

// PackageType distinguishes system packs from Foundry modules.
type PackageType string

const (
	// PackageTypeSystem is a game system content pack (manifest.json + data/*.json).
	PackageTypeSystem PackageType = "system"

	// PackageTypeFoundryModule is the Foundry VTT sync module.
	PackageTypeFoundryModule PackageType = "foundry-module"
)

// UpdatePolicy controls automatic update behavior.
type UpdatePolicy string

const (
	// UpdateOff disables automatic updates.
	UpdateOff UpdatePolicy = "off"

	// UpdateNightly checks for updates once per day.
	UpdateNightly UpdatePolicy = "nightly"

	// UpdateWeekly checks for updates once per week.
	UpdateWeekly UpdatePolicy = "weekly"

	// UpdateOnRelease checks every hour for new releases.
	UpdateOnRelease UpdatePolicy = "on_release"
)

// PackageStatus tracks the lifecycle state of a submitted package.
type PackageStatus string

const (
	// StatusPending means the package is awaiting admin approval.
	StatusPending PackageStatus = "pending"

	// StatusApproved means the package has been approved and is active.
	StatusApproved PackageStatus = "approved"

	// StatusRejected means an admin declined the package submission.
	StatusRejected PackageStatus = "rejected"

	// StatusArchived means the package has been archived (hidden but preserved).
	StatusArchived PackageStatus = "archived"

	// StatusDeprecated means the package is nearing end-of-life.
	StatusDeprecated PackageStatus = "deprecated"
)

// Package represents an external repository tracked by the package manager.
type Package struct {
	ID               string
	Type             PackageType
	Slug             string
	Name             string
	RepoURL          string
	Description      string
	InstalledVersion string
	PinnedVersion    string
	AutoUpdate       UpdatePolicy
	LastCheckedAt    *time.Time
	LastInstalledAt  *time.Time
	InstallPath      string
	SubmittedBy      string        // UserID of submitter (empty = admin-created).
	Status           PackageStatus // Lifecycle state: pending/approved/rejected/archived/deprecated.
	ReviewedBy       string        // Admin who approved/rejected.
	ReviewedAt       *time.Time
	ReviewNote       string // Reason for rejection or review comment.
	DeprecatedAt     *time.Time
	DeprecationMsg   string // Message shown to users about EOL.
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PackageVersion represents a single release from GitHub.
type PackageVersion struct {
	ID           string
	PackageID    string
	Version      string
	TagName      string
	ReleaseURL   string
	DownloadURL  string
	ReleaseNotes string
	Prerelease   bool // True for pre-release/beta versions from GitHub.
	PublishedAt  time.Time
	DownloadedAt *time.Time
	FileSize     int64
	CreatedAt    time.Time
}

// OrphanedInstall represents a package directory on disk that has no
// corresponding record in the database (e.g. after a DB wipe).
type OrphanedInstall struct {
	Path    string // Full path to the orphaned directory.
	Slug    string // Inferred slug (directory name).
	Version string // Inferred version (subdirectory name).
	Size    int64  // Total size in bytes.
}

// PackageUsage shows which campaigns use a package's systems.
type PackageUsage struct {
	CampaignID   string
	CampaignName string
	SystemID     string
	EnabledAt    time.Time
}

// AddPackageInput is the request to add a new package repository.
type AddPackageInput struct {
	RepoURL string `json:"repo_url" form:"repo_url"`
	Name    string `json:"name" form:"name"`
	Type    string `json:"type" form:"type"`
}

// UpdatePolicyInput is the request to change auto-update policy.
type UpdatePolicyInput struct {
	Policy string `json:"policy" form:"policy"`
}

// InstallVersionInput is the request to install a specific version.
type InstallVersionInput struct {
	Version string `json:"version" form:"version"`
}

// PinVersionInput is the request to pin to a specific version.
type PinVersionInput struct {
	Version string `json:"version" form:"version"`
}

// SubmitPackageInput is the request from a campaign owner to submit a repo.
type SubmitPackageInput struct {
	RepoURL string `json:"repo_url" form:"repo_url"`
	Name    string `json:"name" form:"name"`
	Type    string `json:"type" form:"type"`
}

// ReviewPackageInput is the admin's approval or rejection decision.
type ReviewPackageInput struct {
	Action string `json:"action" form:"action"` // "approve" or "reject".
	Note   string `json:"note" form:"note"`
}

// DeprecateInput is the request to mark a package as nearing end-of-life.
type DeprecateInput struct {
	Message string `json:"message" form:"message"`
}

// UpdateRepoURLInput is the request to change a package's repository URL.
type UpdateRepoURLInput struct {
	RepoURL string `json:"repo_url" form:"repo_url"`
}

// PackageSecuritySettings holds the parsed security settings for packages.
type PackageSecuritySettings struct {
	RepoPolicy       string // "github_only", "any_git", or "allow_all".
	RequireApproval  bool
	MaxFileSize      int64 // Bytes.
	ValidateManifest bool
	ScanContent      bool
}

// RepoPolicyGitHubOnly restricts submissions to GitHub repos only.
const RepoPolicyGitHubOnly = "github_only"

// RepoPolicyAnyGit allows any public git host.
const RepoPolicyAnyGit = "any_git"

// RepoPolicyAllowAll allows any HTTPS URL.
const RepoPolicyAllowAll = "allow_all"

// generateUUID creates a new v4 UUID string using crypto/rand.
// Panics if the system entropy source fails, as this indicates a
// catastrophic system problem that would compromise all security.
func generateUUID() string {
	uuid := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, uuid); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
