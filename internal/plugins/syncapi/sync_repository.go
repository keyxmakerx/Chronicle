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
