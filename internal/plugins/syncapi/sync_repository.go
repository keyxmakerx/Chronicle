package syncapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SyncMappingRepository defines data access for sync mappings.
type SyncMappingRepository interface {
	Create(ctx context.Context, m *SyncMapping) error
	FindByID(ctx context.Context, id string) (*SyncMapping, error)
	FindByChronicle(ctx context.Context, campaignID, chronicleType, chronicleID, externalSystem string) (*SyncMapping, error)
	FindByExternal(ctx context.Context, campaignID, externalSystem, externalID string) (*SyncMapping, error)
	ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]SyncMapping, int, error)
	ListByType(ctx context.Context, campaignID, chronicleType string) ([]SyncMapping, error)
	UpdateVersion(ctx context.Context, id string, newVersion int) error
	Delete(ctx context.Context, id string) error
	DeleteByCampaign(ctx context.Context, campaignID string) error
	ListModifiedSince(ctx context.Context, campaignID string, since time.Time, limit int) ([]SyncMapping, error)
	CountByType(ctx context.Context, campaignID string) (map[string]int, error)
	LastSyncActivity(ctx context.Context, campaignID string) (*time.Time, error)
	ListMappingsWithNames(ctx context.Context, campaignID string, opts SyncMappingListOptions) ([]SyncMappingRow, int, error)
	ListCampaignSyncStats(ctx context.Context) ([]CampaignSyncStats, error)
}

// syncMappingRepo implements SyncMappingRepository with MariaDB.
type syncMappingRepo struct {
	db *sql.DB
}

// NewSyncMappingRepository creates a new sync mapping repository.
func NewSyncMappingRepository(db *sql.DB) SyncMappingRepository {
	return &syncMappingRepo{db: db}
}

// Create inserts a new sync mapping.
func (r *syncMappingRepo) Create(ctx context.Context, m *SyncMapping) error {
	metaJSON, err := json.Marshal(m.SyncMetadata)
	if err != nil {
		metaJSON = []byte("null")
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO sync_mappings (id, campaign_id, chronicle_type, chronicle_id,
			external_system, external_id, sync_version, last_synced_at,
			sync_direction, sync_metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.CampaignID, m.ChronicleType, m.ChronicleID,
		m.ExternalSystem, m.ExternalID, m.SyncVersion, m.LastSyncedAt,
		m.SyncDirection, metaJSON,
	)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// FindByID returns a sync mapping by its primary key.
func (r *syncMappingRepo) FindByID(ctx context.Context, id string) (*SyncMapping, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, campaign_id, chronicle_type, chronicle_id, external_system,
			external_id, sync_version, last_synced_at, sync_direction,
			sync_metadata, created_at, updated_at
		FROM sync_mappings WHERE id = ?`, id)
	return r.scanMapping(row)
}

// FindByChronicle looks up a mapping by Chronicle object identity.
func (r *syncMappingRepo) FindByChronicle(ctx context.Context, campaignID, chronicleType, chronicleID, externalSystem string) (*SyncMapping, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, campaign_id, chronicle_type, chronicle_id, external_system,
			external_id, sync_version, last_synced_at, sync_direction,
			sync_metadata, created_at, updated_at
		FROM sync_mappings
		WHERE campaign_id = ? AND chronicle_type = ? AND chronicle_id = ? AND external_system = ?`,
		campaignID, chronicleType, chronicleID, externalSystem)
	return r.scanMapping(row)
}

// FindByExternal looks up a mapping by external system identity.
func (r *syncMappingRepo) FindByExternal(ctx context.Context, campaignID, externalSystem, externalID string) (*SyncMapping, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, campaign_id, chronicle_type, chronicle_id, external_system,
			external_id, sync_version, last_synced_at, sync_direction,
			sync_metadata, created_at, updated_at
		FROM sync_mappings
		WHERE campaign_id = ? AND external_system = ? AND external_id = ?`,
		campaignID, externalSystem, externalID)
	return r.scanMapping(row)
}

// ListByCampaign returns all sync mappings for a campaign with pagination.
func (r *syncMappingRepo) ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]SyncMapping, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sync_mappings WHERE campaign_id = ?`, campaignID,
	).Scan(&total)
	if err != nil {
		return nil, 0, apperror.NewInternal(err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, campaign_id, chronicle_type, chronicle_id, external_system,
			external_id, sync_version, last_synced_at, sync_direction,
			sync_metadata, created_at, updated_at
		FROM sync_mappings WHERE campaign_id = ?
		ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		campaignID, limit, offset)
	if err != nil {
		return nil, 0, apperror.NewInternal(err)
	}
	defer rows.Close()

	mappings, err := r.scanMappings(rows)
	if err != nil {
		return nil, 0, err
	}
	return mappings, total, nil
}

// ListByType returns all mappings for a specific Chronicle type.
func (r *syncMappingRepo) ListByType(ctx context.Context, campaignID, chronicleType string) ([]SyncMapping, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, campaign_id, chronicle_type, chronicle_id, external_system,
			external_id, sync_version, last_synced_at, sync_direction,
			sync_metadata, created_at, updated_at
		FROM sync_mappings
		WHERE campaign_id = ? AND chronicle_type = ?
		ORDER BY created_at DESC`,
		campaignID, chronicleType)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()
	return r.scanMappings(rows)
}

// UpdateVersion increments the sync version and updates last_synced_at.
func (r *syncMappingRepo) UpdateVersion(ctx context.Context, id string, newVersion int) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE sync_mappings SET sync_version = ?, last_synced_at = NOW()
		WHERE id = ?`, newVersion, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("sync mapping not found: " + id)
	}
	return nil
}

// Delete removes a sync mapping by ID.
func (r *syncMappingRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM sync_mappings WHERE id = ?`, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return apperror.NewNotFound("sync mapping not found: " + id)
	}
	return nil
}

// DeleteByCampaign removes all sync mappings for a campaign.
func (r *syncMappingRepo) DeleteByCampaign(ctx context.Context, campaignID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM sync_mappings WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

// ListModifiedSince returns mappings updated after a given timestamp.
func (r *syncMappingRepo) ListModifiedSince(ctx context.Context, campaignID string, since time.Time, limit int) ([]SyncMapping, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, campaign_id, chronicle_type, chronicle_id, external_system,
			external_id, sync_version, last_synced_at, sync_direction,
			sync_metadata, created_at, updated_at
		FROM sync_mappings
		WHERE campaign_id = ? AND updated_at > ?
		ORDER BY updated_at ASC LIMIT ?`,
		campaignID, since, limit)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()
	return r.scanMappings(rows)
}

// scanMapping reads a single sync mapping row.
func (r *syncMappingRepo) scanMapping(row *sql.Row) (*SyncMapping, error) {
	var m SyncMapping
	var metaJSON []byte
	err := row.Scan(
		&m.ID, &m.CampaignID, &m.ChronicleType, &m.ChronicleID,
		&m.ExternalSystem, &m.ExternalID, &m.SyncVersion, &m.LastSyncedAt,
		&m.SyncDirection, &metaJSON, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("sync mapping not found")
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if len(metaJSON) > 0 {
		_ = json.Unmarshal(metaJSON, &m.SyncMetadata)
	}
	return &m, nil
}

// CountByType returns mapping counts grouped by chronicle_type for a campaign.
func (r *syncMappingRepo) CountByType(ctx context.Context, campaignID string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT chronicle_type, COUNT(*) FROM sync_mappings
		WHERE campaign_id = ? GROUP BY chronicle_type`,
		campaignID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var t string
		var c int
		if err := rows.Scan(&t, &c); err != nil {
			return nil, apperror.NewInternal(err)
		}
		result[t] = c
	}
	return result, nil
}

// LastSyncActivity returns the most recent last_synced_at for a campaign.
func (r *syncMappingRepo) LastSyncActivity(ctx context.Context, campaignID string) (*time.Time, error) {
	var t sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT MAX(last_synced_at) FROM sync_mappings WHERE campaign_id = ?`,
		campaignID).Scan(&t)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	if t.Valid {
		return &t.Time, nil
	}
	return nil, nil
}

// ListMappingsWithNames returns sync mappings joined with entity/map names
// for display in the owner dashboard, with search, type filter, and sorting.
func (r *syncMappingRepo) ListMappingsWithNames(ctx context.Context, campaignID string, opts SyncMappingListOptions) ([]SyncMappingRow, int, error) {
	// Build WHERE clause.
	where := "sm.campaign_id = ?"
	args := []any{campaignID}

	if opts.Type != "" {
		where += " AND sm.chronicle_type = ?"
		args = append(args, opts.Type)
	}
	if opts.Search != "" {
		where += " AND (e.name LIKE ? OR m.name LIKE ?)"
		search := "%" + opts.Search + "%"
		args = append(args, search, search)
	}

	// Count.
	var total int
	countQuery := `
		SELECT COUNT(*)
		FROM sync_mappings sm
		LEFT JOIN entities e ON sm.chronicle_type = 'entity' AND sm.chronicle_id = e.id
		LEFT JOIN maps m ON sm.chronicle_type = 'map' AND sm.chronicle_id = m.id
		WHERE ` + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, apperror.NewInternal(err)
	}

	// Sort.
	var orderBy string
	switch opts.Sort {
	case "name":
		orderBy = "COALESCE(e.name, m.name, sm.chronicle_id) ASC"
	case "type":
		orderBy = "sm.chronicle_type ASC, sm.last_synced_at DESC"
	default:
		orderBy = "sm.last_synced_at DESC"
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 25
	}

	query := `
		SELECT sm.id, sm.chronicle_type, sm.chronicle_id,
			COALESCE(e.name, m.name, '') AS chronicle_name,
			sm.external_id, sm.sync_direction, sm.sync_version, sm.last_synced_at
		FROM sync_mappings sm
		LEFT JOIN entities e ON sm.chronicle_type = 'entity' AND sm.chronicle_id = e.id
		LEFT JOIN maps m ON sm.chronicle_type = 'map' AND sm.chronicle_id = m.id
		WHERE ` + where + `
		ORDER BY ` + orderBy + `
		LIMIT ? OFFSET ?`
	args = append(args, limit, opts.Offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, apperror.NewInternal(err)
	}
	defer rows.Close()

	var result []SyncMappingRow
	for rows.Next() {
		var row SyncMappingRow
		if err := rows.Scan(
			&row.ID, &row.ChronicleType, &row.ChronicleID,
			&row.ChronicleName, &row.ExternalID,
			&row.SyncDirection, &row.SyncVersion, &row.LastSyncedAt,
		); err != nil {
			return nil, 0, apperror.NewInternal(err)
		}
		result = append(result, row)
	}
	return result, total, nil
}

// ListCampaignSyncStats returns per-campaign sync statistics for the admin dashboard.
// Joins sync_mappings with campaigns and api_keys to show a full picture.
func (r *syncMappingRepo) ListCampaignSyncStats(ctx context.Context) ([]CampaignSyncStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.id AS campaign_id,
			c.name AS campaign_name,
			COALESCE(ak.active_keys, 0) AS active_keys,
			COALESCE(sm.total_mappings, 0) AS total_mappings,
			sm.last_activity,
			COALESCE(rl.recent_errors, 0) AS recent_errors
		FROM campaigns c
		LEFT JOIN (
			SELECT campaign_id, COUNT(*) AS active_keys
			FROM sync_api_keys WHERE is_active = 1
			GROUP BY campaign_id
		) ak ON ak.campaign_id = c.id
		LEFT JOIN (
			SELECT campaign_id,
				COUNT(*) AS total_mappings,
				MAX(last_synced_at) AS last_activity
			FROM sync_mappings
			GROUP BY campaign_id
		) sm ON sm.campaign_id = c.id
		LEFT JOIN (
			SELECT campaign_id, COUNT(*) AS recent_errors
			FROM api_request_logs
			WHERE status_code >= 400 AND created_at > DATE_SUB(NOW(), INTERVAL 24 HOUR)
			GROUP BY campaign_id
		) rl ON rl.campaign_id = c.id
		WHERE ak.active_keys > 0 OR sm.total_mappings > 0
		ORDER BY sm.last_activity DESC`)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer rows.Close()

	var result []CampaignSyncStats
	for rows.Next() {
		var s CampaignSyncStats
		var lastActivity sql.NullTime
		if err := rows.Scan(
			&s.CampaignID, &s.CampaignName, &s.ActiveKeys,
			&s.TotalMappings, &lastActivity, &s.RecentErrors,
		); err != nil {
			return nil, apperror.NewInternal(err)
		}
		if lastActivity.Valid {
			s.LastActivity = &lastActivity.Time
		}
		result = append(result, s)
	}
	return result, nil
}

// scanMappings reads multiple sync mapping rows.
func (r *syncMappingRepo) scanMappings(rows *sql.Rows) ([]SyncMapping, error) {
	var result []SyncMapping
	for rows.Next() {
		var m SyncMapping
		var metaJSON []byte
		err := rows.Scan(
			&m.ID, &m.CampaignID, &m.ChronicleType, &m.ChronicleID,
			&m.ExternalSystem, &m.ExternalID, &m.SyncVersion, &m.LastSyncedAt,
			&m.SyncDirection, &metaJSON, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &m.SyncMetadata)
		}
		result = append(result, m)
	}
	return result, nil
}
