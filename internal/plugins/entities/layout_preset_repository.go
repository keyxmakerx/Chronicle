package entities

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// LayoutPresetRepository defines data access for layout presets.
type LayoutPresetRepository interface {
	Create(ctx context.Context, p *LayoutPreset) error
	FindByID(ctx context.Context, id int) (*LayoutPreset, error)
	ListForCampaign(ctx context.Context, campaignID string) ([]LayoutPreset, error)
	Update(ctx context.Context, p *LayoutPreset) error
	Delete(ctx context.Context, id int) error
}

type layoutPresetRepository struct {
	db *sql.DB
}

// NewLayoutPresetRepository creates a new layout preset repository.
func NewLayoutPresetRepository(db *sql.DB) LayoutPresetRepository {
	return &layoutPresetRepository{db: db}
}

// Create inserts a new layout preset.
func (r *layoutPresetRepository) Create(ctx context.Context, p *LayoutPreset) error {
	query := `INSERT INTO layout_presets
		(campaign_id, name, description, layout_json, icon, sort_order, is_builtin)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query,
		p.CampaignID, p.Name, p.Description,
		p.LayoutJSON, p.Icon, p.SortOrder, p.IsBuiltin)
	if err != nil {
		return fmt.Errorf("inserting layout preset: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting last insert id: %w", err)
	}
	p.ID = int(id)
	return nil
}

// FindByID retrieves a layout preset by its ID.
func (r *layoutPresetRepository) FindByID(ctx context.Context, id int) (*LayoutPreset, error) {
	query := `SELECT id, campaign_id, name, description, layout_json, icon,
		sort_order, is_builtin, created_at, updated_at
		FROM layout_presets WHERE id = ?`

	var p LayoutPreset
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.CampaignID, &p.Name, &p.Description,
		&p.LayoutJSON, &p.Icon, &p.SortOrder, &p.IsBuiltin,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.NewNotFound("layout preset not found")
		}
		return nil, fmt.Errorf("finding layout preset: %w", err)
	}
	return &p, nil
}

// ListForCampaign returns all layout presets for a campaign.
func (r *layoutPresetRepository) ListForCampaign(ctx context.Context, campaignID string) ([]LayoutPreset, error) {
	query := `SELECT id, campaign_id, name, description, layout_json, icon,
		sort_order, is_builtin, created_at, updated_at
		FROM layout_presets
		WHERE campaign_id = ?
		ORDER BY is_builtin DESC, sort_order, name`

	rows, err := r.db.QueryContext(ctx, query, campaignID)
	if err != nil {
		return nil, fmt.Errorf("querying layout presets: %w", err)
	}
	defer rows.Close()

	var presets []LayoutPreset
	for rows.Next() {
		var p LayoutPreset
		if err := rows.Scan(
			&p.ID, &p.CampaignID, &p.Name, &p.Description,
			&p.LayoutJSON, &p.Icon, &p.SortOrder, &p.IsBuiltin,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning layout preset: %w", err)
		}
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

// Update modifies an existing layout preset.
func (r *layoutPresetRepository) Update(ctx context.Context, p *LayoutPreset) error {
	query := `UPDATE layout_presets
		SET name = ?, description = ?, layout_json = ?, icon = ?
		WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query,
		p.Name, p.Description, p.LayoutJSON, p.Icon, p.ID)
	if err != nil {
		return fmt.Errorf("updating layout preset: %w", err)
	}
	return nil
}

// Delete removes a layout preset by ID.
func (r *layoutPresetRepository) Delete(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM layout_presets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting layout preset: %w", err)
	}
	return nil
}
