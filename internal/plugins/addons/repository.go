package addons

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// AddonRepository defines the data access contract for addon operations.
type AddonRepository interface {
	// Global addon registry.
	List(ctx context.Context) ([]Addon, error)
	FindByID(ctx context.Context, id int) (*Addon, error)
	FindBySlug(ctx context.Context, slug string) (*Addon, error)
	Create(ctx context.Context, addon *Addon) error
	Update(ctx context.Context, addon *Addon) error
	Delete(ctx context.Context, id int) error
	UpdateStatus(ctx context.Context, id int, status AddonStatus) error

	// Per-campaign addon settings.
	ListForCampaign(ctx context.Context, campaignID string) ([]CampaignAddon, error)
	EnableForCampaign(ctx context.Context, campaignID string, addonID int, userID string) error
	DisableForCampaign(ctx context.Context, campaignID string, addonID int) error
	IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error)
	UpdateCampaignConfig(ctx context.Context, campaignID string, addonID int, config map[string]any) error
}

// addonRepository implements AddonRepository with MariaDB.
type addonRepository struct {
	db *sql.DB
}

// NewAddonRepository creates a new addon repository.
func NewAddonRepository(db *sql.DB) AddonRepository {
	return &addonRepository{db: db}
}

// List returns all registered addons ordered by category and name.
func (r *addonRepository) List(ctx context.Context) ([]Addon, error) {
	query := `SELECT id, slug, name, description, version, category, status, icon, author,
	                 config_schema, created_at, updated_at
	          FROM addons ORDER BY category, name`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing addons: %w", err)
	}
	defer rows.Close()

	var addons []Addon
	for rows.Next() {
		var a Addon
		var schemaRaw []byte
		if err := rows.Scan(&a.ID, &a.Slug, &a.Name, &a.Description, &a.Version,
			&a.Category, &a.Status, &a.Icon, &a.Author,
			&schemaRaw, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning addon: %w", err)
		}
		if len(schemaRaw) > 0 {
			json.Unmarshal(schemaRaw, &a.ConfigSchema)
		}
		addons = append(addons, a)
	}
	return addons, rows.Err()
}

// FindByID retrieves an addon by its ID.
func (r *addonRepository) FindByID(ctx context.Context, id int) (*Addon, error) {
	query := `SELECT id, slug, name, description, version, category, status, icon, author,
	                 config_schema, created_at, updated_at
	          FROM addons WHERE id = ?`

	a := &Addon{}
	var schemaRaw []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&a.ID, &a.Slug, &a.Name, &a.Description, &a.Version,
		&a.Category, &a.Status, &a.Icon, &a.Author,
		&schemaRaw, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("addon not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding addon: %w", err)
	}
	if len(schemaRaw) > 0 {
		json.Unmarshal(schemaRaw, &a.ConfigSchema)
	}
	return a, nil
}

// FindBySlug retrieves an addon by its slug.
func (r *addonRepository) FindBySlug(ctx context.Context, slug string) (*Addon, error) {
	query := `SELECT id, slug, name, description, version, category, status, icon, author,
	                 config_schema, created_at, updated_at
	          FROM addons WHERE slug = ?`

	a := &Addon{}
	var schemaRaw []byte
	err := r.db.QueryRowContext(ctx, query, slug).Scan(
		&a.ID, &a.Slug, &a.Name, &a.Description, &a.Version,
		&a.Category, &a.Status, &a.Icon, &a.Author,
		&schemaRaw, &a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("addon not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding addon by slug: %w", err)
	}
	if len(schemaRaw) > 0 {
		json.Unmarshal(schemaRaw, &a.ConfigSchema)
	}
	return a, nil
}

// Create inserts a new addon.
func (r *addonRepository) Create(ctx context.Context, addon *Addon) error {
	query := `INSERT INTO addons (slug, name, description, version, category, status, icon, author)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query,
		addon.Slug, addon.Name, addon.Description, addon.Version,
		addon.Category, addon.Status, addon.Icon, addon.Author,
	)
	if err != nil {
		return fmt.Errorf("creating addon: %w", err)
	}
	id, _ := result.LastInsertId()
	addon.ID = int(id)
	return nil
}

// Update modifies an addon's metadata.
func (r *addonRepository) Update(ctx context.Context, addon *Addon) error {
	query := `UPDATE addons SET name = ?, description = ?, version = ?, status = ?, icon = ?
	          WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		addon.Name, addon.Description, addon.Version, addon.Status, addon.Icon, addon.ID,
	)
	if err != nil {
		return fmt.Errorf("updating addon: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("addon not found")
	}
	return nil
}

// Delete removes an addon from the registry.
func (r *addonRepository) Delete(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM addons WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting addon: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("addon not found")
	}
	return nil
}

// UpdateStatus changes an addon's lifecycle status (active/planned/deprecated).
func (r *addonRepository) UpdateStatus(ctx context.Context, id int, status AddonStatus) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE addons SET status = ? WHERE id = ?`, status, id,
	)
	if err != nil {
		return fmt.Errorf("updating addon status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("addon not found")
	}
	return nil
}

// ListForCampaign returns all addons with their per-campaign enabled state.
// Includes all active addons, with enabled=true/false based on campaign_addons.
func (r *addonRepository) ListForCampaign(ctx context.Context, campaignID string) ([]CampaignAddon, error) {
	query := `SELECT a.id, a.slug, a.name, a.icon, a.category, a.status, a.description,
	                 COALESCE(ca.enabled, 0), ca.config_json, ca.enabled_at, ca.enabled_by
	          FROM addons a
	          LEFT JOIN campaign_addons ca ON ca.addon_id = a.id AND ca.campaign_id = ?
	          WHERE a.status = 'active'
	          ORDER BY a.category, a.name`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing campaign addons: %w", err)
	}
	defer rows.Close()

	var result []CampaignAddon
	for rows.Next() {
		var ca CampaignAddon
		var desc *string
		var configRaw []byte
		var enabledAt sql.NullTime
		var enabledBy sql.NullString

		if err := rows.Scan(&ca.AddonID, &ca.AddonSlug, &ca.AddonName, &ca.AddonIcon,
			&ca.AddonCategory, &ca.AddonStatus, &desc,
			&ca.Enabled, &configRaw, &enabledAt, &enabledBy); err != nil {
			return nil, fmt.Errorf("scanning campaign addon: %w", err)
		}
		ca.CampaignID = campaignID
		if enabledAt.Valid {
			ca.EnabledAt = enabledAt.Time
		}
		if enabledBy.Valid {
			eb := enabledBy.String
			ca.EnabledBy = &eb
		}
		if len(configRaw) > 0 {
			json.Unmarshal(configRaw, &ca.ConfigJSON)
		}
		result = append(result, ca)
	}
	return result, rows.Err()
}

// EnableForCampaign enables an addon for a campaign (upsert).
func (r *addonRepository) EnableForCampaign(ctx context.Context, campaignID string, addonID int, userID string) error {
	query := `INSERT INTO campaign_addons (campaign_id, addon_id, enabled, enabled_by)
	          VALUES (?, ?, 1, ?)
	          ON DUPLICATE KEY UPDATE enabled = 1, enabled_by = ?`
	_, err := r.db.ExecContext(ctx, query, campaignID, addonID, userID, userID)
	if err != nil {
		return fmt.Errorf("enabling addon for campaign: %w", err)
	}
	return nil
}

// DisableForCampaign disables an addon for a campaign.
func (r *addonRepository) DisableForCampaign(ctx context.Context, campaignID string, addonID int) error {
	query := `UPDATE campaign_addons SET enabled = 0 WHERE campaign_id = ? AND addon_id = ?`
	_, err := r.db.ExecContext(ctx, query, campaignID, addonID)
	if err != nil {
		return fmt.Errorf("disabling addon for campaign: %w", err)
	}
	return nil
}

// IsEnabledForCampaign checks if a specific addon (by slug) is enabled for a campaign.
func (r *addonRepository) IsEnabledForCampaign(ctx context.Context, campaignID string, addonSlug string) (bool, error) {
	query := `SELECT ca.enabled FROM campaign_addons ca
	          INNER JOIN addons a ON a.id = ca.addon_id
	          WHERE ca.campaign_id = ? AND a.slug = ? AND ca.enabled = 1`
	var enabled bool
	err := r.db.QueryRowContext(ctx, query, campaignID, addonSlug).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking addon enabled: %w", err)
	}
	return enabled, nil
}

// UpdateCampaignConfig updates the addon-specific configuration for a campaign.
func (r *addonRepository) UpdateCampaignConfig(ctx context.Context, campaignID string, addonID int, config map[string]any) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshaling addon config: %w", err)
	}
	query := `UPDATE campaign_addons SET config_json = ? WHERE campaign_id = ? AND addon_id = ?`
	_, err = r.db.ExecContext(ctx, query, configJSON, campaignID, addonID)
	if err != nil {
		return fmt.Errorf("updating campaign addon config: %w", err)
	}
	return nil
}
