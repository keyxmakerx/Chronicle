// Package entities contains the saved filter model and repository.
// Saved filters are per-user, per-campaign tag filter presets shown
// in the sidebar drill panel for quick tag combo switching.
package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SavedFilter represents a user's saved tag filter preset.
type SavedFilter struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	CampaignID   string   `json:"campaign_id"`
	EntityTypeID *int     `json:"entity_type_id,omitempty"` // NULL = all types.
	Name         string   `json:"name"`
	TagSlugs     []string `json:"tag_slugs"`
	CreatedAt    time.Time `json:"created_at"`
}

// SavedFilterRepository provides CRUD for saved tag filter presets.
type SavedFilterRepository interface {
	List(ctx context.Context, userID, campaignID string) ([]SavedFilter, error)
	Create(ctx context.Context, filter *SavedFilter) error
	Delete(ctx context.Context, id, userID string) error
}

// savedFilterRepository implements SavedFilterRepository with MariaDB.
type savedFilterRepository struct {
	db *sql.DB
}

// NewSavedFilterRepository creates a saved filter repository.
func NewSavedFilterRepository(db *sql.DB) SavedFilterRepository {
	return &savedFilterRepository{db: db}
}

// List returns all saved filters for a user in a campaign.
func (r *savedFilterRepository) List(ctx context.Context, userID, campaignID string) ([]SavedFilter, error) {
	query := `SELECT id, user_id, campaign_id, entity_type_id, name, tag_slugs, created_at
	          FROM saved_filters
	          WHERE user_id = ? AND campaign_id = ?
	          ORDER BY name ASC`

	rows, err := r.db.QueryContext(ctx, query, userID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing saved filters: %w", err)
	}
	defer rows.Close()

	var filters []SavedFilter
	for rows.Next() {
		var f SavedFilter
		var tagsRaw []byte
		if err := rows.Scan(&f.ID, &f.UserID, &f.CampaignID, &f.EntityTypeID,
			&f.Name, &tagsRaw, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning saved filter: %w", err)
		}
		if len(tagsRaw) > 0 {
			_ = json.Unmarshal(tagsRaw, &f.TagSlugs)
		}
		filters = append(filters, f)
	}
	return filters, rows.Err()
}

// Create inserts a new saved filter.
func (r *savedFilterRepository) Create(ctx context.Context, filter *SavedFilter) error {
	tagsJSON, err := json.Marshal(filter.TagSlugs)
	if err != nil {
		return fmt.Errorf("marshaling tag slugs: %w", err)
	}

	query := `INSERT INTO saved_filters (id, user_id, campaign_id, entity_type_id, name, tag_slugs, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err = r.db.ExecContext(ctx, query,
		filter.ID, filter.UserID, filter.CampaignID, filter.EntityTypeID,
		filter.Name, tagsJSON, filter.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating saved filter: %w", err)
	}
	return nil
}

// Delete removes a saved filter. Scoped to user for safety.
func (r *savedFilterRepository) Delete(ctx context.Context, id, userID string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM saved_filters WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return fmt.Errorf("deleting saved filter: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("saved filter not found")
	}
	return nil
}

// generateFilterID creates a new UUID for a saved filter.
func generateFilterID() string {
	return uuid.New().String()
}
