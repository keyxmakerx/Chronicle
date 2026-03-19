// Package entities contains the entity favorites model and repository.
// Favorites are per-user, per-campaign bookmarks shown in the sidebar
// drill panel for quick entity access.
package entities

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Favorite represents a user's bookmarked entity within a campaign.
type Favorite struct {
	UserID     string    `json:"user_id"`
	EntityID   string    `json:"entity_id"`
	CampaignID string    `json:"campaign_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// FavoriteItem is the display representation of a favorite, with joined
// entity data for rendering in the sidebar.
type FavoriteItem struct {
	EntityID  string `json:"id"`
	Name      string `json:"name"`
	TypeIcon  string `json:"type_icon"`
	TypeColor string `json:"type_color"`
}

// FavoriteRepository provides CRUD for entity favorites.
type FavoriteRepository interface {
	Toggle(ctx context.Context, userID, entityID, campaignID string) (bool, error)
	List(ctx context.Context, userID, campaignID string) ([]FavoriteItem, error)
	IsFavorite(ctx context.Context, userID, entityID string) (bool, error)
	ListIDs(ctx context.Context, userID, campaignID string) (map[string]bool, error)
}

// favoriteRepository implements FavoriteRepository with MariaDB.
type favoriteRepository struct {
	db *sql.DB
}

// NewFavoriteRepository creates a favorite repository.
func NewFavoriteRepository(db *sql.DB) FavoriteRepository {
	return &favoriteRepository{db: db}
}

// Toggle adds or removes a favorite. Returns true if now favorited.
func (r *favoriteRepository) Toggle(ctx context.Context, userID, entityID, campaignID string) (bool, error) {
	// Check if already favorited.
	var exists bool
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM entity_favorites WHERE user_id = ? AND entity_id = ?)",
		userID, entityID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking favorite: %w", err)
	}

	if exists {
		_, err = r.db.ExecContext(ctx,
			"DELETE FROM entity_favorites WHERE user_id = ? AND entity_id = ?",
			userID, entityID,
		)
		if err != nil {
			return false, fmt.Errorf("removing favorite: %w", err)
		}
		return false, nil
	}

	_, err = r.db.ExecContext(ctx,
		"INSERT INTO entity_favorites (user_id, entity_id, campaign_id, created_at) VALUES (?, ?, ?, ?)",
		userID, entityID, campaignID, time.Now().UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("adding favorite: %w", err)
	}
	return true, nil
}

// List returns all favorites for a user in a campaign, joined with entity
// data for display. Ordered by most recently favorited first.
func (r *favoriteRepository) List(ctx context.Context, userID, campaignID string) ([]FavoriteItem, error) {
	query := `SELECT e.id, e.name, et.icon, et.color
	          FROM entity_favorites f
	          INNER JOIN entities e ON e.id = f.entity_id
	          INNER JOIN entity_types et ON et.id = e.entity_type_id
	          WHERE f.user_id = ? AND f.campaign_id = ?
	          ORDER BY f.created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, userID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing favorites: %w", err)
	}
	defer rows.Close()

	var items []FavoriteItem
	for rows.Next() {
		var item FavoriteItem
		if err := rows.Scan(&item.EntityID, &item.Name, &item.TypeIcon, &item.TypeColor); err != nil {
			return nil, fmt.Errorf("scanning favorite: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// IsFavorite checks if a specific entity is favorited by the user.
func (r *favoriteRepository) IsFavorite(ctx context.Context, userID, entityID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM entity_favorites WHERE user_id = ? AND entity_id = ?)",
		userID, entityID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking favorite: %w", err)
	}
	return exists, nil
}

// ListIDs returns a set of entity IDs that are favorited by the user
// in a campaign. Used for bulk-checking favorite state.
func (r *favoriteRepository) ListIDs(ctx context.Context, userID, campaignID string) (map[string]bool, error) {
	query := `SELECT entity_id FROM entity_favorites WHERE user_id = ? AND campaign_id = ?`
	rows, err := r.db.QueryContext(ctx, query, userID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing favorite IDs: %w", err)
	}
	defer rows.Close()

	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning favorite ID: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
