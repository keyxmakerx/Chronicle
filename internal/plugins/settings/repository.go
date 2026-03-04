package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SettingsRepository defines the data access contract for site settings
// and per-entity storage limit overrides.
type SettingsRepository interface {
	// Get retrieves a single setting value by key. Returns NotFound if the key does not exist.
	Get(ctx context.Context, key string) (string, error)

	// Set upserts a setting value. Creates the key if it does not exist.
	Set(ctx context.Context, key, value string) error

	// GetAll returns every setting as a key-value map.
	GetAll(ctx context.Context) (map[string]string, error)

	// GetUserLimit returns the per-user storage override, or nil if no override exists.
	GetUserLimit(ctx context.Context, userID string) (*UserStorageLimit, error)

	// SetUserLimit upserts a per-user storage limit override.
	SetUserLimit(ctx context.Context, limit *UserStorageLimit) error

	// DeleteUserLimit removes a per-user storage override, reverting to global defaults.
	DeleteUserLimit(ctx context.Context, userID string) error

	// GetCampaignLimit returns the per-campaign storage override, or nil if none exists.
	GetCampaignLimit(ctx context.Context, campaignID string) (*CampaignStorageLimit, error)

	// SetCampaignLimit upserts a per-campaign storage limit override.
	SetCampaignLimit(ctx context.Context, limit *CampaignStorageLimit) error

	// DeleteCampaignLimit removes a per-campaign storage override, reverting to defaults.
	DeleteCampaignLimit(ctx context.Context, campaignID string) error

	// ListUserLimits returns all per-user overrides with display names for the admin table.
	ListUserLimits(ctx context.Context) ([]UserStorageLimitWithName, error)

	// ListCampaignLimits returns all per-campaign overrides with campaign names for the admin table.
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

// settingsRepository implements SettingsRepository using MariaDB.
type settingsRepository struct {
	db *sql.DB
}

// NewSettingsRepository creates a new settings repository backed by MariaDB.
func NewSettingsRepository(db *sql.DB) SettingsRepository {
	return &settingsRepository{db: db}
}

// Get retrieves a single setting value by its key.
func (r *settingsRepository) Get(ctx context.Context, key string) (string, error) {
	query := `SELECT setting_value FROM site_settings WHERE setting_key = ?`

	var value string
	err := r.db.QueryRowContext(ctx, query, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", apperror.NewNotFound(fmt.Sprintf("setting %q not found", key))
	}
	if err != nil {
		return "", apperror.NewInternal(fmt.Errorf("querying setting %q: %w", key, err))
	}
	return value, nil
}

// Set upserts a setting value using INSERT ... ON DUPLICATE KEY UPDATE.
func (r *settingsRepository) Set(ctx context.Context, key, value string) error {
	query := `INSERT INTO site_settings (setting_key, setting_value)
	          VALUES (?, ?)
	          ON DUPLICATE KEY UPDATE setting_value = VALUES(setting_value)`

	if _, err := r.db.ExecContext(ctx, query, key, value); err != nil {
		return apperror.NewInternal(fmt.Errorf("upserting setting %q: %w", key, err))
	}
	return nil
}

// GetAll returns all settings as a key-value map.
func (r *settingsRepository) GetAll(ctx context.Context) (map[string]string, error) {
	query := `SELECT setting_key, setting_value FROM site_settings`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("querying all settings: %w", err))
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scanning setting row: %w", err))
		}
		result[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("iterating settings: %w", err))
	}
	return result, nil
}

// --- Per-User Limits ---

// GetUserLimit returns the per-user override, or nil if no row exists.
func (r *settingsRepository) GetUserLimit(ctx context.Context, userID string) (*UserStorageLimit, error) {
	query := `SELECT user_id, max_upload_size, max_total_storage, updated_at,
	                 bypass_max_upload, bypass_expires_at, bypass_reason, bypass_granted_by
	          FROM user_storage_limits WHERE user_id = ?`

	var ul UserStorageLimit
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&ul.UserID, &ul.MaxUploadSize, &ul.MaxTotalStorage, &ul.UpdatedAt,
		&ul.BypassMaxUpload, &ul.BypassExpiresAt, &ul.BypassReason, &ul.BypassGrantedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No override -- use global defaults.
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("querying user limit for %s: %w", userID, err))
	}
	return &ul, nil
}

// SetUserLimit upserts a per-user storage limit override.
func (r *settingsRepository) SetUserLimit(ctx context.Context, limit *UserStorageLimit) error {
	query := `INSERT INTO user_storage_limits (user_id, max_upload_size, max_total_storage)
	          VALUES (?, ?, ?)
	          ON DUPLICATE KEY UPDATE
	              max_upload_size = VALUES(max_upload_size),
	              max_total_storage = VALUES(max_total_storage)`

	if _, err := r.db.ExecContext(ctx, query, limit.UserID, limit.MaxUploadSize, limit.MaxTotalStorage); err != nil {
		return apperror.NewInternal(fmt.Errorf("upserting user limit for %s: %w", limit.UserID, err))
	}
	return nil
}

// DeleteUserLimit removes a per-user override row.
func (r *settingsRepository) DeleteUserLimit(ctx context.Context, userID string) error {
	query := `DELETE FROM user_storage_limits WHERE user_id = ?`

	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting user limit for %s: %w", userID, err))
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("no storage override found for this user")
	}
	return nil
}

// --- Per-Campaign Limits ---

// GetCampaignLimit returns the per-campaign override, or nil if no row exists.
func (r *settingsRepository) GetCampaignLimit(ctx context.Context, campaignID string) (*CampaignStorageLimit, error) {
	query := `SELECT campaign_id, max_total_storage, max_files, updated_at,
	                 bypass_max_storage, bypass_max_files, bypass_expires_at, bypass_reason, bypass_granted_by
	          FROM campaign_storage_limits WHERE campaign_id = ?`

	var cl CampaignStorageLimit
	err := r.db.QueryRowContext(ctx, query, campaignID).Scan(
		&cl.CampaignID, &cl.MaxTotalStorage, &cl.MaxFiles, &cl.UpdatedAt,
		&cl.BypassMaxStorage, &cl.BypassMaxFiles, &cl.BypassExpiresAt, &cl.BypassReason, &cl.BypassGrantedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No override -- use global/user defaults.
	}
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("querying campaign limit for %s: %w", campaignID, err))
	}
	return &cl, nil
}

// SetCampaignLimit upserts a per-campaign storage limit override.
func (r *settingsRepository) SetCampaignLimit(ctx context.Context, limit *CampaignStorageLimit) error {
	query := `INSERT INTO campaign_storage_limits (campaign_id, max_total_storage, max_files)
	          VALUES (?, ?, ?)
	          ON DUPLICATE KEY UPDATE
	              max_total_storage = VALUES(max_total_storage),
	              max_files = VALUES(max_files)`

	if _, err := r.db.ExecContext(ctx, query, limit.CampaignID, limit.MaxTotalStorage, limit.MaxFiles); err != nil {
		return apperror.NewInternal(fmt.Errorf("upserting campaign limit for %s: %w", limit.CampaignID, err))
	}
	return nil
}

// DeleteCampaignLimit removes a per-campaign override row.
func (r *settingsRepository) DeleteCampaignLimit(ctx context.Context, campaignID string) error {
	query := `DELETE FROM campaign_storage_limits WHERE campaign_id = ?`

	result, err := r.db.ExecContext(ctx, query, campaignID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("deleting campaign limit for %s: %w", campaignID, err))
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("no storage override found for this campaign")
	}
	return nil
}

// --- Admin List Views (JOINed for display) ---

// ListUserLimits returns all per-user overrides joined with users.display_name.
func (r *settingsRepository) ListUserLimits(ctx context.Context) ([]UserStorageLimitWithName, error) {
	query := `SELECT ul.user_id, ul.max_upload_size, ul.max_total_storage, ul.updated_at,
	                 ul.bypass_max_upload, ul.bypass_expires_at, ul.bypass_reason, ul.bypass_granted_by,
	                 u.display_name
	          FROM user_storage_limits ul
	          JOIN users u ON u.id = ul.user_id
	          ORDER BY u.display_name`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing user limits: %w", err))
	}
	defer rows.Close()

	var limits []UserStorageLimitWithName
	for rows.Next() {
		var l UserStorageLimitWithName
		if err := rows.Scan(
			&l.UserID, &l.MaxUploadSize, &l.MaxTotalStorage, &l.UpdatedAt,
			&l.BypassMaxUpload, &l.BypassExpiresAt, &l.BypassReason, &l.BypassGrantedBy,
			&l.DisplayName,
		); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scanning user limit row: %w", err))
		}
		limits = append(limits, l)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("iterating user limits: %w", err))
	}
	return limits, nil
}

// ListCampaignLimits returns all per-campaign overrides joined with campaigns.name.
func (r *settingsRepository) ListCampaignLimits(ctx context.Context) ([]CampaignStorageLimitWithName, error) {
	query := `SELECT cl.campaign_id, cl.max_total_storage, cl.max_files, cl.updated_at,
	                 cl.bypass_max_storage, cl.bypass_max_files, cl.bypass_expires_at, cl.bypass_reason, cl.bypass_granted_by,
	                 c.name
	          FROM campaign_storage_limits cl
	          JOIN campaigns c ON c.id = cl.campaign_id
	          ORDER BY c.name`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("listing campaign limits: %w", err))
	}
	defer rows.Close()

	var limits []CampaignStorageLimitWithName
	for rows.Next() {
		var l CampaignStorageLimitWithName
		if err := rows.Scan(
			&l.CampaignID, &l.MaxTotalStorage, &l.MaxFiles, &l.UpdatedAt,
			&l.BypassMaxStorage, &l.BypassMaxFiles, &l.BypassExpiresAt, &l.BypassReason, &l.BypassGrantedBy,
			&l.CampaignName,
		); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scanning campaign limit row: %w", err))
		}
		limits = append(limits, l)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("iterating campaign limits: %w", err))
	}
	return limits, nil
}

// --- Temporary Bypass Methods ---

// SetUserBypass sets a time-limited bypass on a user's storage limits.
// If no override row exists for the user, one is created with NULL limits
// so the bypass columns have a row to live on.
func (r *settingsRepository) SetUserBypass(ctx context.Context, userID string, maxUpload *int64, expiresAt time.Time, reason, grantedBy string) error {
	query := `INSERT INTO user_storage_limits (user_id, bypass_max_upload, bypass_expires_at, bypass_reason, bypass_granted_by)
	          VALUES (?, ?, ?, ?, ?)
	          ON DUPLICATE KEY UPDATE
	              bypass_max_upload = VALUES(bypass_max_upload),
	              bypass_expires_at = VALUES(bypass_expires_at),
	              bypass_reason = VALUES(bypass_reason),
	              bypass_granted_by = VALUES(bypass_granted_by)`

	if _, err := r.db.ExecContext(ctx, query, userID, maxUpload, expiresAt, reason, grantedBy); err != nil {
		return apperror.NewInternal(fmt.Errorf("setting user bypass for %s: %w", userID, err))
	}
	return nil
}

// ClearUserBypass removes the temporary bypass from a user's storage limits
// by setting all bypass columns to NULL.
func (r *settingsRepository) ClearUserBypass(ctx context.Context, userID string) error {
	query := `UPDATE user_storage_limits
	          SET bypass_max_upload = NULL, bypass_expires_at = NULL,
	              bypass_reason = NULL, bypass_granted_by = NULL
	          WHERE user_id = ?`

	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("clearing user bypass for %s: %w", userID, err))
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("no storage override found for this user")
	}
	return nil
}

// SetCampaignBypass sets a time-limited bypass on a campaign's storage limits.
// If no override row exists for the campaign, one is created with NULL limits
// so the bypass columns have a row to live on.
func (r *settingsRepository) SetCampaignBypass(ctx context.Context, campaignID string, maxStorage *int64, maxFiles *int, expiresAt time.Time, reason, grantedBy string) error {
	query := `INSERT INTO campaign_storage_limits (campaign_id, bypass_max_storage, bypass_max_files, bypass_expires_at, bypass_reason, bypass_granted_by)
	          VALUES (?, ?, ?, ?, ?, ?)
	          ON DUPLICATE KEY UPDATE
	              bypass_max_storage = VALUES(bypass_max_storage),
	              bypass_max_files = VALUES(bypass_max_files),
	              bypass_expires_at = VALUES(bypass_expires_at),
	              bypass_reason = VALUES(bypass_reason),
	              bypass_granted_by = VALUES(bypass_granted_by)`

	if _, err := r.db.ExecContext(ctx, query, campaignID, maxStorage, maxFiles, expiresAt, reason, grantedBy); err != nil {
		return apperror.NewInternal(fmt.Errorf("setting campaign bypass for %s: %w", campaignID, err))
	}
	return nil
}

// ClearCampaignBypass removes the temporary bypass from a campaign's storage limits
// by setting all bypass columns to NULL.
func (r *settingsRepository) ClearCampaignBypass(ctx context.Context, campaignID string) error {
	query := `UPDATE campaign_storage_limits
	          SET bypass_max_storage = NULL, bypass_max_files = NULL,
	              bypass_expires_at = NULL, bypass_reason = NULL, bypass_granted_by = NULL
	          WHERE campaign_id = ?`

	result, err := r.db.ExecContext(ctx, query, campaignID)
	if err != nil {
		return apperror.NewInternal(fmt.Errorf("clearing campaign bypass for %s: %w", campaignID, err))
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("no storage override found for this campaign")
	}
	return nil
}
