package maps

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// MapRepository defines persistence operations for maps and markers.
type MapRepository interface {
	// Map CRUD.
	CreateMap(ctx context.Context, m *Map) error
	GetMap(ctx context.Context, id string) (*Map, error)
	UpdateMap(ctx context.Context, m *Map) error
	DeleteMap(ctx context.Context, id string) error
	ListMaps(ctx context.Context, campaignID string) ([]Map, error)
	SearchMaps(ctx context.Context, campaignID, query string) ([]Map, error)

	// Marker CRUD.
	CreateMarker(ctx context.Context, mk *Marker) error
	GetMarker(ctx context.Context, id string) (*Marker, error)
	UpdateMarker(ctx context.Context, mk *Marker) error
	DeleteMarker(ctx context.Context, id string) error
	ListMarkers(ctx context.Context, mapID string, role int, userID string) ([]Marker, error)
}

// mapRepo is the MariaDB implementation of MapRepository.
type mapRepo struct {
	db *sql.DB
}

// NewMapRepository creates a new MariaDB-backed map repository.
func NewMapRepository(db *sql.DB) MapRepository {
	return &mapRepo{db: db}
}

// mapCols is the column list for map queries.
const mapCols = `id, campaign_id, name, description, image_id,
       image_width, image_height, sort_order, created_at, updated_at`

// scanMap reads a row into a Map struct.
func scanMap(scanner interface{ Scan(...any) error }) (*Map, error) {
	m := &Map{}
	err := scanner.Scan(&m.ID, &m.CampaignID, &m.Name, &m.Description, &m.ImageID,
		&m.ImageWidth, &m.ImageHeight, &m.SortOrder, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// CreateMap inserts a new map.
func (r *mapRepo) CreateMap(ctx context.Context, m *Map) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO maps (id, campaign_id, name, description, image_id,
		        image_width, image_height, sort_order)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.CampaignID, m.Name, m.Description, m.ImageID,
		m.ImageWidth, m.ImageHeight, m.SortOrder,
	)
	return err
}

// GetMap returns a single map by ID.
func (r *mapRepo) GetMap(ctx context.Context, id string) (*Map, error) {
	return scanMap(r.db.QueryRowContext(ctx,
		`SELECT `+mapCols+` FROM maps WHERE id = ?`, id))
}

// UpdateMap modifies an existing map.
func (r *mapRepo) UpdateMap(ctx context.Context, m *Map) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE maps SET name = ?, description = ?, image_id = ?,
		        image_width = ?, image_height = ?
		 WHERE id = ?`,
		m.Name, m.Description, m.ImageID,
		m.ImageWidth, m.ImageHeight, m.ID,
	)
	return err
}

// DeleteMap removes a map and all its markers (cascaded by FK).
func (r *mapRepo) DeleteMap(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM maps WHERE id = ?`, id)
	return err
}

// ListMaps returns all maps for a campaign, ordered by sort_order.
func (r *mapRepo) ListMaps(ctx context.Context, campaignID string) ([]Map, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+mapCols+` FROM maps WHERE campaign_id = ? ORDER BY sort_order, name`,
		campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Map
	for rows.Next() {
		m, err := scanMap(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *m)
	}
	return result, rows.Err()
}

// markerCols is the column list for marker queries (with entity join fields).
const markerCols = `m.id, m.map_id, m.name, m.description,
       m.x, m.y, m.icon, m.color, m.pin_category,
       m.entity_id, m.visibility, m.visibility_rules,
       m.created_by, m.created_at, m.updated_at,
       COALESCE(ent.name, ''), COALESCE(et.icon, '')`

// markerJoins is the LEFT JOIN clause for entity display data.
const markerJoins = `LEFT JOIN entities ent ON ent.id = m.entity_id
     LEFT JOIN entity_types et ON et.id = ent.entity_type_id`

// scanMarker reads a row into a Marker struct.
func scanMarker(scanner interface{ Scan(...any) error }) (*Marker, error) {
	mk := &Marker{}
	err := scanner.Scan(&mk.ID, &mk.MapID, &mk.Name, &mk.Description,
		&mk.X, &mk.Y, &mk.Icon, &mk.Color, &mk.PinCategory,
		&mk.EntityID, &mk.Visibility, &mk.VisibilityRules,
		&mk.CreatedBy, &mk.CreatedAt, &mk.UpdatedAt,
		&mk.EntityName, &mk.EntityIcon)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return mk, err
}

// CreateMarker inserts a new marker.
func (r *mapRepo) CreateMarker(ctx context.Context, mk *Marker) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO map_markers (id, map_id, name, description,
		        x, y, icon, color, pin_category, entity_id, visibility, visibility_rules, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mk.ID, mk.MapID, mk.Name, mk.Description,
		mk.X, mk.Y, mk.Icon, mk.Color, mk.PinCategory, mk.EntityID, mk.Visibility,
		mk.VisibilityRules, mk.CreatedBy,
	)
	return err
}

// GetMarker returns a single marker by ID.
func (r *mapRepo) GetMarker(ctx context.Context, id string) (*Marker, error) {
	return scanMarker(r.db.QueryRowContext(ctx,
		`SELECT `+markerCols+`
		 FROM map_markers m `+markerJoins+`
		 WHERE m.id = ?`, id))
}

// UpdateMarker modifies an existing marker.
func (r *mapRepo) UpdateMarker(ctx context.Context, mk *Marker) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE map_markers SET name = ?, description = ?,
		        x = ?, y = ?, icon = ?, color = ?, pin_category = ?,
		        entity_id = ?, visibility = ?, visibility_rules = ?
		 WHERE id = ?`,
		mk.Name, mk.Description,
		mk.X, mk.Y, mk.Icon, mk.Color, mk.PinCategory,
		mk.EntityID, mk.Visibility, mk.VisibilityRules, mk.ID,
	)
	return err
}

// DeleteMarker removes a marker.
func (r *mapRepo) DeleteMarker(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM map_markers WHERE id = ?`, id)
	return err
}

// ListMarkers returns all markers for a map, filtered by role and user.
// Owners see everything. Non-owners see 'everyone' markers (respecting
// visibility_rules allow/deny lists) and 'specific' markers they are
// explicitly allowed to see.
func (r *mapRepo) ListMarkers(ctx context.Context, mapID string, role int, userID string) ([]Marker, error) {
	// Owners see all markers regardless of visibility settings.
	if permissions.CanSeeDmOnly(role) {
		query := `
			SELECT ` + markerCols + `
			FROM map_markers m ` + markerJoins + `
			WHERE m.map_id = ?
			ORDER BY m.name`
		rows, err := r.db.QueryContext(ctx, query, mapID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var result []Marker
		for rows.Next() {
			mk, err := scanMarker(rows)
			if err != nil {
				return nil, err
			}
			result = append(result, *mk)
		}
		return result, rows.Err()
	}

	// Non-owners: 'everyone' + per-player rules, or 'specific' with allowed_users.
	query := `
		SELECT ` + markerCols + `
		FROM map_markers m ` + markerJoins + `
		WHERE m.map_id = ?
		  AND m.visibility != 'dm_only'
		  AND (
		    m.visibility_rules IS NULL
		    OR (
		      NOT JSON_CONTAINS(m.visibility_rules, JSON_QUOTE(?), '$.denied_users')
		      AND (
		        JSON_LENGTH(COALESCE(JSON_EXTRACT(m.visibility_rules, '$.allowed_users'), '[]')) = 0
		        OR JSON_CONTAINS(m.visibility_rules, JSON_QUOTE(?), '$.allowed_users')
		      )
		    )
		  )
		ORDER BY m.name`
	rows, err := r.db.QueryContext(ctx, query, mapID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Marker
	for rows.Next() {
		mk, err := scanMarker(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *mk)
	}
	return result, rows.Err()
}

// SearchMaps returns maps matching a name query for a campaign.
func (r *mapRepo) SearchMaps(ctx context.Context, campaignID, query string) ([]Map, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+mapCols+` FROM maps
		 WHERE campaign_id = ? AND name LIKE ?
		 ORDER BY name LIMIT 10`,
		campaignID, "%"+strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)+"%")
	if err != nil {
		return nil, fmt.Errorf("search maps: %w", err)
	}
	defer rows.Close()

	var result []Map
	for rows.Next() {
		m, err := scanMap(rows)
		if err != nil {
			return nil, fmt.Errorf("scan map: %w", err)
		}
		result = append(result, *m)
	}
	return result, rows.Err()
}
