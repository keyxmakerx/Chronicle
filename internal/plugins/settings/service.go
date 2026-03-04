package settings

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SettingsService handles business logic for site settings and storage limits.
// It parses string values from the database into typed structs and resolves
// the override chain (per-campaign > per-user > global) for effective limits.
type SettingsService interface {
	// GetStorageLimits returns the parsed global storage limits.
	GetStorageLimits(ctx context.Context) (*GlobalStorageLimits, error)

	// UpdateStorageLimits validates and persists updated global storage limits.
	UpdateStorageLimits(ctx context.Context, limits *GlobalStorageLimits) error

	// GetEffectiveLimits resolves the final storage limits for a given user and
	// campaign. Override priority: per-campaign > per-user > global. A value of
	// 0 at any tier means unlimited (no cap enforced).
	GetEffectiveLimits(ctx context.Context, userID, campaignID string) (*EffectiveLimits, error)

	// GetUserLimit returns the per-user storage override, or nil if none exists.
	GetUserLimit(ctx context.Context, userID string) (*UserStorageLimit, error)

	// SetUserLimit validates and upserts a per-user storage override.
	SetUserLimit(ctx context.Context, limit *UserStorageLimit) error

	// DeleteUserLimit removes a per-user storage override.
	DeleteUserLimit(ctx context.Context, userID string) error

	// GetCampaignLimit returns the per-campaign storage override, or nil if none exists.
	GetCampaignLimit(ctx context.Context, campaignID string) (*CampaignStorageLimit, error)

	// SetCampaignLimit validates and upserts a per-campaign storage override.
	SetCampaignLimit(ctx context.Context, limit *CampaignStorageLimit) error

	// DeleteCampaignLimit removes a per-campaign storage override.
	DeleteCampaignLimit(ctx context.Context, campaignID string) error

	// ListUserLimits returns all per-user overrides with display names.
	ListUserLimits(ctx context.Context) ([]UserStorageLimitWithName, error)

	// ListCampaignLimits returns all per-campaign overrides with campaign names.
	ListCampaignLimits(ctx context.Context) ([]CampaignStorageLimitWithName, error)

	// SetUserBypass sets a temporary bypass on a user's storage limits.
	SetUserBypass(ctx context.Context, userID string, maxUpload *int64, expiresAt time.Time, reason, grantedBy string) error

	// ClearUserBypass removes the temporary bypass from a user's storage limits.
	ClearUserBypass(ctx context.Context, userID string) error

	// SetCampaignBypass sets a temporary bypass on a campaign's storage limits.
	SetCampaignBypass(ctx context.Context, campaignID string, maxStorage *int64, maxFiles *int, expiresAt time.Time, reason, grantedBy string) error

	// ClearCampaignBypass removes the temporary bypass from a campaign's storage limits.
	ClearCampaignBypass(ctx context.Context, campaignID string) error
}

// settingsService implements SettingsService.
type settingsService struct {
	repo SettingsRepository
}

// NewSettingsService creates a new settings service.
func NewSettingsService(repo SettingsRepository) SettingsService {
	return &settingsService{repo: repo}
}

// GetStorageLimits reads all storage-related settings and parses them into
// a typed struct. Missing or unparseable values fall back to safe defaults.
func (s *settingsService) GetStorageLimits(ctx context.Context) (*GlobalStorageLimits, error) {
	all, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	return &GlobalStorageLimits{
		MaxUploadSize:          parseInt64(all[KeyMaxUploadSize], 10485760),
		MaxStoragePerUser:      parseInt64(all[KeyMaxStoragePerUser], 0),
		MaxStoragePerCampaign:  parseInt64(all[KeyMaxStoragePerCampaign], 0),
		MaxFilesPerCampaign:    parseInt(all[KeyMaxFilesPerCampaign], 0),
		RateLimitUploadsPerMin: parseInt(all[KeyRateLimitUploadsPerMin], 30),
	}, nil
}

// UpdateStorageLimits validates the limits and persists each one as a
// separate key-value row. Negative values are rejected.
func (s *settingsService) UpdateStorageLimits(ctx context.Context, limits *GlobalStorageLimits) error {
	if limits.MaxUploadSize < 0 {
		return apperror.NewBadRequest("max upload size cannot be negative")
	}
	if limits.MaxStoragePerUser < 0 {
		return apperror.NewBadRequest("max storage per user cannot be negative")
	}
	if limits.MaxStoragePerCampaign < 0 {
		return apperror.NewBadRequest("max storage per campaign cannot be negative")
	}
	if limits.MaxFilesPerCampaign < 0 {
		return apperror.NewBadRequest("max files per campaign cannot be negative")
	}
	if limits.RateLimitUploadsPerMin < 0 {
		return apperror.NewBadRequest("rate limit cannot be negative")
	}

	// Persist each setting individually so partial updates work.
	settings := map[string]string{
		KeyMaxUploadSize:          strconv.FormatInt(limits.MaxUploadSize, 10),
		KeyMaxStoragePerUser:      strconv.FormatInt(limits.MaxStoragePerUser, 10),
		KeyMaxStoragePerCampaign:  strconv.FormatInt(limits.MaxStoragePerCampaign, 10),
		KeyMaxFilesPerCampaign:    strconv.Itoa(limits.MaxFilesPerCampaign),
		KeyRateLimitUploadsPerMin: strconv.Itoa(limits.RateLimitUploadsPerMin),
	}

	for key, value := range settings {
		if err := s.repo.Set(ctx, key, value); err != nil {
			return fmt.Errorf("persisting %s: %w", key, err)
		}
	}
	return nil
}

// GetEffectiveLimits resolves the final limits for a user+campaign context.
// Override priority: per-campaign overrides > per-user overrides > global.
// A NULL override field means "inherit from next tier up". A value of 0
// means "unlimited" (no cap enforced).
func (s *settingsService) GetEffectiveLimits(ctx context.Context, userID, campaignID string) (*EffectiveLimits, error) {
	global, err := s.GetStorageLimits(ctx)
	if err != nil {
		return nil, err
	}

	// Start with global defaults.
	effective := &EffectiveLimits{
		MaxUploadSize:   global.MaxUploadSize,
		MaxTotalStorage: global.MaxStoragePerUser,
		MaxFiles:        global.MaxFilesPerCampaign,
	}

	// Layer per-user overrides if a userID was provided.
	var userLimit *UserStorageLimit
	if userID != "" {
		userLimit, err = s.repo.GetUserLimit(ctx, userID)
		if err != nil {
			return nil, err
		}
		if userLimit != nil {
			if userLimit.MaxUploadSize != nil {
				effective.MaxUploadSize = *userLimit.MaxUploadSize
			}
			if userLimit.MaxTotalStorage != nil {
				effective.MaxTotalStorage = *userLimit.MaxTotalStorage
			}
		}
	}

	// Layer per-campaign overrides if a campaignID was provided.
	var campaignLimit *CampaignStorageLimit
	if campaignID != "" {
		campaignLimit, err = s.repo.GetCampaignLimit(ctx, campaignID)
		if err != nil {
			return nil, err
		}
		if campaignLimit != nil {
			if campaignLimit.MaxTotalStorage != nil {
				effective.MaxTotalStorage = *campaignLimit.MaxTotalStorage
			}
			if campaignLimit.MaxFiles != nil {
				effective.MaxFiles = *campaignLimit.MaxFiles
			}
		}
	}

	// Apply active bypasses as the highest-priority overrides.
	// Bypasses are time-limited and auto-expire; only apply if not expired.
	if userLimit.HasActiveBypass() {
		if userLimit.BypassMaxUpload != nil {
			effective.MaxUploadSize = *userLimit.BypassMaxUpload
		}
	}
	if campaignLimit.HasActiveBypass() {
		if campaignLimit.BypassMaxStorage != nil {
			effective.MaxTotalStorage = *campaignLimit.BypassMaxStorage
		}
		if campaignLimit.BypassMaxFiles != nil {
			effective.MaxFiles = *campaignLimit.BypassMaxFiles
		}
	}

	return effective, nil
}

// --- Per-User Limit Pass-Through ---

// GetUserLimit delegates to the repository.
func (s *settingsService) GetUserLimit(ctx context.Context, userID string) (*UserStorageLimit, error) {
	if userID == "" {
		return nil, apperror.NewBadRequest("user ID is required")
	}
	return s.repo.GetUserLimit(ctx, userID)
}

// SetUserLimit validates and upserts a per-user override.
func (s *settingsService) SetUserLimit(ctx context.Context, limit *UserStorageLimit) error {
	if limit.UserID == "" {
		return apperror.NewBadRequest("user ID is required")
	}
	// Validate that override values are non-negative when set.
	if limit.MaxUploadSize != nil && *limit.MaxUploadSize < 0 {
		return apperror.NewBadRequest("max upload size cannot be negative")
	}
	if limit.MaxTotalStorage != nil && *limit.MaxTotalStorage < 0 {
		return apperror.NewBadRequest("max total storage cannot be negative")
	}
	return s.repo.SetUserLimit(ctx, limit)
}

// DeleteUserLimit removes a per-user override.
func (s *settingsService) DeleteUserLimit(ctx context.Context, userID string) error {
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}
	return s.repo.DeleteUserLimit(ctx, userID)
}

// --- Per-Campaign Limit Pass-Through ---

// GetCampaignLimit delegates to the repository.
func (s *settingsService) GetCampaignLimit(ctx context.Context, campaignID string) (*CampaignStorageLimit, error) {
	if campaignID == "" {
		return nil, apperror.NewBadRequest("campaign ID is required")
	}
	return s.repo.GetCampaignLimit(ctx, campaignID)
}

// SetCampaignLimit validates and upserts a per-campaign override.
func (s *settingsService) SetCampaignLimit(ctx context.Context, limit *CampaignStorageLimit) error {
	if limit.CampaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}
	if limit.MaxTotalStorage != nil && *limit.MaxTotalStorage < 0 {
		return apperror.NewBadRequest("max total storage cannot be negative")
	}
	if limit.MaxFiles != nil && *limit.MaxFiles < 0 {
		return apperror.NewBadRequest("max files cannot be negative")
	}
	return s.repo.SetCampaignLimit(ctx, limit)
}

// DeleteCampaignLimit removes a per-campaign override.
func (s *settingsService) DeleteCampaignLimit(ctx context.Context, campaignID string) error {
	if campaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}
	return s.repo.DeleteCampaignLimit(ctx, campaignID)
}

// --- Admin List Views ---

// ListUserLimits returns all per-user overrides with display names.
func (s *settingsService) ListUserLimits(ctx context.Context) ([]UserStorageLimitWithName, error) {
	return s.repo.ListUserLimits(ctx)
}

// ListCampaignLimits returns all per-campaign overrides with campaign names.
func (s *settingsService) ListCampaignLimits(ctx context.Context) ([]CampaignStorageLimitWithName, error) {
	return s.repo.ListCampaignLimits(ctx)
}

// --- Temporary Bypass Methods ---

// SetUserBypass validates and sets a temporary bypass on a user's storage limits.
func (s *settingsService) SetUserBypass(ctx context.Context, userID string, maxUpload *int64, expiresAt time.Time, reason, grantedBy string) error {
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}
	if grantedBy == "" {
		return apperror.NewBadRequest("granted_by is required")
	}
	if expiresAt.Before(time.Now()) {
		return apperror.NewBadRequest("expiration must be in the future")
	}
	if maxUpload != nil && *maxUpload < 0 {
		return apperror.NewBadRequest("bypass max upload cannot be negative")
	}
	return s.repo.SetUserBypass(ctx, userID, maxUpload, expiresAt, reason, grantedBy)
}

// ClearUserBypass removes the temporary bypass from a user's storage limits.
func (s *settingsService) ClearUserBypass(ctx context.Context, userID string) error {
	if userID == "" {
		return apperror.NewBadRequest("user ID is required")
	}
	return s.repo.ClearUserBypass(ctx, userID)
}

// SetCampaignBypass validates and sets a temporary bypass on a campaign's storage limits.
func (s *settingsService) SetCampaignBypass(ctx context.Context, campaignID string, maxStorage *int64, maxFiles *int, expiresAt time.Time, reason, grantedBy string) error {
	if campaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}
	if grantedBy == "" {
		return apperror.NewBadRequest("granted_by is required")
	}
	if expiresAt.Before(time.Now()) {
		return apperror.NewBadRequest("expiration must be in the future")
	}
	if maxStorage != nil && *maxStorage < 0 {
		return apperror.NewBadRequest("bypass max storage cannot be negative")
	}
	if maxFiles != nil && *maxFiles < 0 {
		return apperror.NewBadRequest("bypass max files cannot be negative")
	}
	return s.repo.SetCampaignBypass(ctx, campaignID, maxStorage, maxFiles, expiresAt, reason, grantedBy)
}

// ClearCampaignBypass removes the temporary bypass from a campaign's storage limits.
func (s *settingsService) ClearCampaignBypass(ctx context.Context, campaignID string) error {
	if campaignID == "" {
		return apperror.NewBadRequest("campaign ID is required")
	}
	return s.repo.ClearCampaignBypass(ctx, campaignID)
}

// --- Parsing Helpers ---

// parseInt64 parses a string to int64, returning the fallback on failure.
func parseInt64(s string, fallback int64) int64 {
	if s == "" {
		return fallback
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

// parseInt parses a string to int, returning the fallback on failure.
func parseInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
