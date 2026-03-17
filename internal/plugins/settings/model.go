// Package settings manages site-wide configuration and storage limit overrides.
// It provides a key-value store for global settings (stored in site_settings),
// per-user upload/storage overrides, and per-campaign storage/file-count overrides.
// The resolution order for effective limits is: per-campaign > per-user > global,
// with 0 meaning unlimited at any tier.
package settings

import (
	"time"
)

// --- Database Models ---

// SiteSetting represents a single row in the site_settings key-value table.
// Settings are stored as string values and parsed into typed structs by the service layer.
type SiteSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserStorageLimit represents a per-user storage override. NULL fields mean
// "fall back to global default". Stored in the user_storage_limits table.
type UserStorageLimit struct {
	UserID          string    `json:"user_id"`
	MaxUploadSize   *int64    `json:"max_upload_size"`   // NULL = use global default.
	MaxTotalStorage *int64    `json:"max_total_storage"`  // NULL = use global default.
	UpdatedAt       time.Time `json:"updated_at"`

	// Bypass fields: temporary time-limited overrides that take highest priority.
	BypassMaxUpload *int64     `json:"bypass_max_upload,omitempty"`
	BypassExpiresAt *time.Time `json:"bypass_expires_at,omitempty"`
	BypassReason    *string    `json:"bypass_reason,omitempty"`
	BypassGrantedBy *string    `json:"bypass_granted_by,omitempty"`
}

// HasActiveBypass returns true if the user has a non-expired bypass.
func (ul *UserStorageLimit) HasActiveBypass() bool {
	return ul != nil && ul.BypassExpiresAt != nil && ul.BypassExpiresAt.After(time.Now())
}

// CampaignStorageLimit represents a per-campaign storage override. NULL fields
// mean "fall back to user or global default". Stored in campaign_storage_limits.
type CampaignStorageLimit struct {
	CampaignID      string    `json:"campaign_id"`
	MaxTotalStorage *int64    `json:"max_total_storage"` // NULL = use global/user default.
	MaxFiles        *int      `json:"max_files"`          // NULL = use global default.
	UpdatedAt       time.Time `json:"updated_at"`

	// Bypass fields: temporary time-limited overrides that take highest priority.
	BypassMaxStorage *int64     `json:"bypass_max_storage,omitempty"`
	BypassMaxFiles   *int       `json:"bypass_max_files,omitempty"`
	BypassExpiresAt  *time.Time `json:"bypass_expires_at,omitempty"`
	BypassReason     *string    `json:"bypass_reason,omitempty"`
	BypassGrantedBy  *string    `json:"bypass_granted_by,omitempty"`
}

// HasActiveBypass returns true if the campaign has a non-expired bypass.
func (cl *CampaignStorageLimit) HasActiveBypass() bool {
	return cl != nil && cl.BypassExpiresAt != nil && cl.BypassExpiresAt.After(time.Now())
}

// --- Display Models (with JOINed names) ---

// UserStorageLimitWithName extends UserStorageLimit with the user's display name
// for rendering in admin tables.
type UserStorageLimitWithName struct {
	UserStorageLimit
	DisplayName string `json:"display_name"`
}

// CampaignStorageLimitWithName extends CampaignStorageLimit with the campaign
// name for rendering in admin tables.
type CampaignStorageLimitWithName struct {
	CampaignStorageLimit
	CampaignName string `json:"campaign_name"`
}

// --- Service DTOs ---

// GlobalStorageLimits holds the parsed global storage settings from the
// site_settings table. Values of 0 mean "unlimited" (no cap enforced).
type GlobalStorageLimits struct {
	MaxUploadSize         int64 `json:"max_upload_size"`           // Bytes. Per-file upload cap.
	MaxStoragePerUser     int64 `json:"max_storage_per_user"`      // Bytes. Total across all campaigns.
	MaxStoragePerCampaign int64 `json:"max_storage_per_campaign"`  // Bytes. Total per campaign.
	MaxFilesPerCampaign   int   `json:"max_files_per_campaign"`    // File count cap per campaign.
	RateLimitUploadsPerMin int  `json:"rate_limit_uploads_per_min"` // Uploads per minute per IP.
}

// EffectiveLimits represents the resolved storage limits for a specific
// user + campaign context. This is the final result after merging global,
// per-user, and per-campaign overrides. Values of 0 mean unlimited.
type EffectiveLimits struct {
	MaxUploadSize   int64 `json:"max_upload_size"`   // Per-file upload cap in bytes.
	MaxTotalStorage int64 `json:"max_total_storage"`  // Total storage cap in bytes.
	MaxFiles        int   `json:"max_files"`           // Max file count.
}

// --- Setting Key Constants ---

// Setting keys used in the site_settings table.
const (
	KeyMaxUploadSize         = "storage.max_upload_size"
	KeyMaxStoragePerUser     = "storage.max_storage_per_user"
	KeyMaxStoragePerCampaign = "storage.max_storage_per_campaign"
	KeyMaxFilesPerCampaign   = "storage.max_files_per_campaign"
	KeyRateLimitUploadsPerMin = "storage.rate_limit_uploads_per_min"

	// KeyCORSAllowedOrigins stores a comma-separated list of additional origins
	// permitted to make cross-origin API requests (e.g., Foundry VTT servers).
	// BaseURL is always allowed implicitly. Managed from admin panel.
	KeyCORSAllowedOrigins = "cors.allowed_origins"

	// --- Package Security Settings ---

	// KeyPackageRepoPolicy controls which repository sources are allowed.
	// Values: "github_only" (default), "any_git", "allow_all".
	KeyPackageRepoPolicy = "packages.repo_policy"

	// KeyPackageRequireApproval controls whether owner-submitted packages
	// need admin sign-off before being fetched. Values: "true" (default), "false".
	KeyPackageRequireApproval = "packages.require_approval"

	// KeyPackageMaxFileSize is the maximum download size in bytes for package
	// ZIP assets. Default: 52428800 (50 MB).
	KeyPackageMaxFileSize = "packages.max_file_size"

	// KeyPackageValidateManifest controls whether downloaded packages must
	// contain a valid manifest.json. Values: "true" (default), "false".
	KeyPackageValidateManifest = "packages.validate_manifest"

	// KeyPackageScanContent controls whether downloaded packages are scanned
	// for suspicious files (executables, symlinks). Values: "true" (default), "false".
	KeyPackageScanContent = "packages.scan_content"
)
