// Package settings manages site-wide configuration and storage limit overrides.
// It provides a key-value store for global settings (stored in site_settings),
// per-user upload/storage overrides, and per-campaign storage/file-count overrides.
// The resolution order for effective limits is: per-campaign > per-user > global,
// with 0 meaning unlimited at any tier.
package settings

import "time"

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
}

// CampaignStorageLimit represents a per-campaign storage override. NULL fields
// mean "fall back to user or global default". Stored in campaign_storage_limits.
type CampaignStorageLimit struct {
	CampaignID      string    `json:"campaign_id"`
	MaxTotalStorage *int64    `json:"max_total_storage"` // NULL = use global/user default.
	MaxFiles        *int      `json:"max_files"`          // NULL = use global default.
	UpdatedAt       time.Time `json:"updated_at"`
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
)
