package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// PageLayoutRepository owns entity_type_page_layouts (a category's named
// layouts, one JSON document each) and entity_page_layouts (the layout a page
// wears, by key).
type PageLayoutRepository interface {
	// TypeLayouts returns a category's document; a category with none has
	// an empty one.
	TypeLayouts(ctx context.Context, typeID int) (PageLayouts, error)
	// CampaignTypeLayouts returns every category's document in a campaign
	// that has one, by category id.
	CampaignTypeLayouts(ctx context.Context, campaignID string) (map[int]PageLayouts, error)
	// SaveTypeLayouts stores a category's document; an empty one removes it.
	SaveTypeLayouts(ctx context.Context, campaignID string, typeID int, p PageLayouts) error
	// PageKey returns the key a page chose, or "" when it chose none.
	PageKey(ctx context.Context, entityID string) (string, error)
	// CampaignPageKeys returns every page's choice in a campaign, by page id.
	CampaignPageKeys(ctx context.Context, campaignID string) (map[string]string, error)
	// SetPageKey stores a page's choice; "" removes it.
	SetPageKey(ctx context.Context, campaignID, entityID, key string) error
}

type pageLayoutRepository struct {
	db *sql.DB
}

// NewPageLayoutRepository creates the MariaDB repository for named layouts.
func NewPageLayoutRepository(db *sql.DB) PageLayoutRepository {
	return &pageLayoutRepository{db: db}
}

func (r *pageLayoutRepository) TypeLayouts(ctx context.Context, typeID int) (PageLayouts, error) {
	var doc string
	err := r.db.QueryRowContext(ctx, `SELECT doc FROM entity_type_page_layouts WHERE entity_type_id = ?`, typeID).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return PageLayouts{}, nil
	}
	if err != nil {
		return PageLayouts{}, fmt.Errorf("reading page layouts: %w", err)
	}
	return parsePageLayouts(doc)
}

func (r *pageLayoutRepository) CampaignTypeLayouts(ctx context.Context, campaignID string) (map[int]PageLayouts, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT entity_type_id, doc FROM entity_type_page_layouts WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing page layouts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int]PageLayouts{}
	for rows.Next() {
		var id int
		var doc string
		if err := rows.Scan(&id, &doc); err != nil {
			return nil, fmt.Errorf("scanning page layouts: %w", err)
		}
		p, err := parsePageLayouts(doc)
		if err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

func (r *pageLayoutRepository) SaveTypeLayouts(ctx context.Context, campaignID string, typeID int, p PageLayouts) error {
	if p.Empty() {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM entity_type_page_layouts WHERE entity_type_id = ?`, typeID); err != nil {
			return fmt.Errorf("removing page layouts: %w", err)
		}
		return nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encoding page layouts: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO entity_type_page_layouts (entity_type_id, campaign_id, doc) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE doc = VALUES(doc)`, typeID, campaignID, string(b))
	if err != nil {
		return fmt.Errorf("saving page layouts: %w", err)
	}
	return nil
}

func (r *pageLayoutRepository) PageKey(ctx context.Context, entityID string) (string, error) {
	var key string
	err := r.db.QueryRowContext(ctx, `SELECT layout_key FROM entity_page_layouts WHERE entity_id = ?`, entityID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading a page's layout: %w", err)
	}
	return key, nil
}

func (r *pageLayoutRepository) CampaignPageKeys(ctx context.Context, campaignID string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT entity_id, layout_key FROM entity_page_layouts WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing pages' layouts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, fmt.Errorf("scanning pages' layouts: %w", err)
		}
		out[id] = key
	}
	return out, rows.Err()
}

func (r *pageLayoutRepository) SetPageKey(ctx context.Context, campaignID, entityID, key string) error {
	if key == "" {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM entity_page_layouts WHERE entity_id = ?`, entityID); err != nil {
			return fmt.Errorf("clearing a page's layout: %w", err)
		}
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO entity_page_layouts (entity_id, campaign_id, layout_key) VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE layout_key = VALUES(layout_key)`, entityID, campaignID, key)
	if err != nil {
		return fmt.Errorf("saving a page's layout: %w", err)
	}
	return nil
}

// parsePageLayouts reads a stored document; one that no longer parses is an
// error rather than an empty document, so a save never overwrites it blind.
func parsePageLayouts(doc string) (PageLayouts, error) {
	var p PageLayouts
	if err := json.Unmarshal([]byte(doc), &p); err != nil {
		return PageLayouts{}, fmt.Errorf("decoding page layouts: %w", err)
	}
	return p, nil
}
