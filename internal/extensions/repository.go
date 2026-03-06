package extensions

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ExtensionRepository handles persistence for extensions and related tables.
type ExtensionRepository interface {
	// Extension CRUD.
	Create(ctx context.Context, ext *Extension) error
	FindByID(ctx context.Context, id string) (*Extension, error)
	FindByExtID(ctx context.Context, extID string) (*Extension, error)
	List(ctx context.Context) ([]Extension, error)
	UpdateVersion(ctx context.Context, id, version string, manifest json.RawMessage) error
	UpdateStatus(ctx context.Context, id, status string) error
	Delete(ctx context.Context, id string) error

	// Campaign extension CRUD.
	EnableForCampaign(ctx context.Context, ce *CampaignExtension) error
	DisableForCampaign(ctx context.Context, campaignID, extensionID string) error
	GetCampaignExtension(ctx context.Context, campaignID, extensionID string) (*CampaignExtension, error)
	ListForCampaign(ctx context.Context, campaignID string) ([]CampaignExtension, error)
	UpdateAppliedContents(ctx context.Context, campaignID, extensionID string, contents json.RawMessage) error
	CountCampaignsUsing(ctx context.Context, extensionID string) (int, error)
	RemoveFromAllCampaigns(ctx context.Context, extensionID string) error

	// Provenance tracking.
	CreateProvenance(ctx context.Context, p *Provenance) error
	CreateProvenanceBatch(ctx context.Context, records []Provenance) error
	ListProvenance(ctx context.Context, campaignID, extensionID string) ([]Provenance, error)
	DeleteProvenance(ctx context.Context, campaignID, extensionID string) error

	// Extension data.
	SetData(ctx context.Context, d *ExtensionData) error
	GetData(ctx context.Context, campaignID, extensionID, namespace, key string) (*ExtensionData, error)
	ListData(ctx context.Context, campaignID, extensionID, namespace string) ([]ExtensionData, error)
	DeleteData(ctx context.Context, campaignID, extensionID string) error
	DeleteDataByKey(ctx context.Context, campaignID, extensionID, namespace, key string) error
}

// extensionRepository implements ExtensionRepository with MariaDB.
type extensionRepository struct {
	db *sql.DB
}

// NewExtensionRepository creates a new extension repository.
func NewExtensionRepository(db *sql.DB) ExtensionRepository {
	return &extensionRepository{db: db}
}

// --- Extension CRUD ---

// Create inserts a new extension.
func (r *extensionRepository) Create(ctx context.Context, ext *Extension) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO extensions (id, ext_id, name, version, description, manifest, installed_by, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ext.ID, ext.ExtID, ext.Name, ext.Version, ext.Description,
		ext.Manifest, ext.InstalledBy, ext.Status,
		ext.CreatedAt, ext.UpdatedAt,
	)
	return err
}

// FindByID returns an extension by its internal UUID.
func (r *extensionRepository) FindByID(ctx context.Context, id string) (*Extension, error) {
	var ext Extension
	err := r.db.QueryRowContext(ctx,
		`SELECT id, ext_id, name, version, description, manifest, installed_by, status, created_at, updated_at
		 FROM extensions WHERE id = ?`, id,
	).Scan(&ext.ID, &ext.ExtID, &ext.Name, &ext.Version, &ext.Description,
		&ext.Manifest, &ext.InstalledBy, &ext.Status,
		&ext.CreatedAt, &ext.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, apperror.NewNotFound("extension not found")
	}
	return &ext, err
}

// FindByExtID returns an extension by its manifest ID (e.g., "dnd5e-srd-monsters").
func (r *extensionRepository) FindByExtID(ctx context.Context, extID string) (*Extension, error) {
	var ext Extension
	err := r.db.QueryRowContext(ctx,
		`SELECT id, ext_id, name, version, description, manifest, installed_by, status, created_at, updated_at
		 FROM extensions WHERE ext_id = ?`, extID,
	).Scan(&ext.ID, &ext.ExtID, &ext.Name, &ext.Version, &ext.Description,
		&ext.Manifest, &ext.InstalledBy, &ext.Status,
		&ext.CreatedAt, &ext.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &ext, err
}

// List returns all installed extensions.
func (r *extensionRepository) List(ctx context.Context) ([]Extension, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, ext_id, name, version, description, manifest, installed_by, status, created_at, updated_at
		 FROM extensions ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Extension
	for rows.Next() {
		var ext Extension
		if err := rows.Scan(&ext.ID, &ext.ExtID, &ext.Name, &ext.Version, &ext.Description,
			&ext.Manifest, &ext.InstalledBy, &ext.Status,
			&ext.CreatedAt, &ext.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, ext)
	}
	return result, rows.Err()
}

// UpdateVersion updates an extension's version and manifest after upgrade.
func (r *extensionRepository) UpdateVersion(ctx context.Context, id, version string, manifest json.RawMessage) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE extensions SET version = ?, manifest = ?, updated_at = ? WHERE id = ?`,
		version, manifest, time.Now().UTC(), id,
	)
	return err
}

// UpdateStatus enables or disables an extension site-wide.
func (r *extensionRepository) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE extensions SET status = ?, updated_at = ? WHERE id = ?`,
		status, time.Now().UTC(), id,
	)
	return err
}

// Delete removes an extension. CASCADE deletes campaign_extensions,
// extension_provenance, and extension_data.
func (r *extensionRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM extensions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return apperror.NewNotFound("extension not found")
	}
	return nil
}

// --- Campaign Extension CRUD ---

// EnableForCampaign creates or re-enables an extension for a campaign.
func (r *extensionRepository) EnableForCampaign(ctx context.Context, ce *CampaignExtension) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_extensions (campaign_id, extension_id, enabled, applied_contents, enabled_at, enabled_by)
		 VALUES (?, ?, TRUE, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE enabled = TRUE, enabled_at = VALUES(enabled_at), enabled_by = VALUES(enabled_by)`,
		ce.CampaignID, ce.ExtensionID, ce.AppliedContents, ce.EnabledAt, ce.EnabledBy,
	)
	return err
}

// DisableForCampaign disables an extension for a campaign (keeps data).
func (r *extensionRepository) DisableForCampaign(ctx context.Context, campaignID, extensionID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_extensions SET enabled = FALSE WHERE campaign_id = ? AND extension_id = ?`,
		campaignID, extensionID,
	)
	return err
}

// GetCampaignExtension returns the campaign extension record if it exists.
func (r *extensionRepository) GetCampaignExtension(ctx context.Context, campaignID, extensionID string) (*CampaignExtension, error) {
	var ce CampaignExtension
	err := r.db.QueryRowContext(ctx,
		`SELECT ce.campaign_id, ce.extension_id, ce.enabled, ce.applied_contents, ce.enabled_at, ce.enabled_by,
		        e.ext_id, e.name, e.version, e.status
		 FROM campaign_extensions ce
		 JOIN extensions e ON e.id = ce.extension_id
		 WHERE ce.campaign_id = ? AND ce.extension_id = ?`,
		campaignID, extensionID,
	).Scan(&ce.CampaignID, &ce.ExtensionID, &ce.Enabled, &ce.AppliedContents, &ce.EnabledAt, &ce.EnabledBy,
		&ce.ExtID, &ce.ExtName, &ce.ExtVersion, &ce.ExtStatus)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &ce, err
}

// ListForCampaign returns all extensions available for a campaign with
// their enabled/disabled status.
func (r *extensionRepository) ListForCampaign(ctx context.Context, campaignID string) ([]CampaignExtension, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT e.id, e.ext_id, e.name, e.version, e.status,
		        COALESCE(ce.enabled, FALSE), ce.applied_contents, ce.enabled_at, ce.enabled_by
		 FROM extensions e
		 LEFT JOIN campaign_extensions ce ON ce.extension_id = e.id AND ce.campaign_id = ?
		 WHERE e.status = 'active'
		 ORDER BY e.name`,
		campaignID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []CampaignExtension
	for rows.Next() {
		var ce CampaignExtension
		var enabledAt sql.NullTime
		var enabledBy sql.NullString
		var appliedContents sql.NullString

		if err := rows.Scan(&ce.ExtensionID, &ce.ExtID, &ce.ExtName, &ce.ExtVersion, &ce.ExtStatus,
			&ce.Enabled, &appliedContents, &enabledAt, &enabledBy); err != nil {
			return nil, err
		}
		ce.CampaignID = campaignID
		if enabledAt.Valid {
			ce.EnabledAt = enabledAt.Time
		}
		if enabledBy.Valid {
			ce.EnabledBy = &enabledBy.String
		}
		if appliedContents.Valid {
			ce.AppliedContents = json.RawMessage(appliedContents.String)
		}
		result = append(result, ce)
	}
	return result, rows.Err()
}

// UpdateAppliedContents updates the JSON tracking which contributes were imported.
func (r *extensionRepository) UpdateAppliedContents(ctx context.Context, campaignID, extensionID string, contents json.RawMessage) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_extensions SET applied_contents = ? WHERE campaign_id = ? AND extension_id = ?`,
		contents, campaignID, extensionID,
	)
	return err
}

// CountCampaignsUsing returns how many campaigns have this extension enabled.
func (r *extensionRepository) CountCampaignsUsing(ctx context.Context, extensionID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM campaign_extensions WHERE extension_id = ? AND enabled = TRUE`,
		extensionID,
	).Scan(&count)
	return count, err
}

// RemoveFromAllCampaigns deletes all campaign_extensions records for an extension.
func (r *extensionRepository) RemoveFromAllCampaigns(ctx context.Context, extensionID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_extensions WHERE extension_id = ?`, extensionID,
	)
	return err
}

// --- Provenance Tracking ---

// CreateProvenance records that an extension created a database record.
func (r *extensionRepository) CreateProvenance(ctx context.Context, p *Provenance) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO extension_provenance (campaign_id, extension_id, table_name, record_id, record_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.CampaignID, p.ExtensionID, p.TableName, p.RecordID, p.RecordType, p.CreatedAt,
	)
	return err
}

// CreateProvenanceBatch inserts multiple provenance records in one query.
func (r *extensionRepository) CreateProvenanceBatch(ctx context.Context, records []Provenance) error {
	if len(records) == 0 {
		return nil
	}

	query := `INSERT INTO extension_provenance (campaign_id, extension_id, table_name, record_id, record_type, created_at) VALUES `
	args := make([]any, 0, len(records)*6)
	for i, p := range records {
		if i > 0 {
			query += ", "
		}
		query += "(?, ?, ?, ?, ?, ?)"
		args = append(args, p.CampaignID, p.ExtensionID, p.TableName, p.RecordID, p.RecordType, p.CreatedAt)
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// ListProvenance returns all provenance records for a campaign+extension.
func (r *extensionRepository) ListProvenance(ctx context.Context, campaignID, extensionID string) ([]Provenance, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, campaign_id, extension_id, table_name, record_id, record_type, created_at
		 FROM extension_provenance WHERE campaign_id = ? AND extension_id = ?
		 ORDER BY created_at`,
		campaignID, extensionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Provenance
	for rows.Next() {
		var p Provenance
		if err := rows.Scan(&p.ID, &p.CampaignID, &p.ExtensionID, &p.TableName, &p.RecordID, &p.RecordType, &p.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// DeleteProvenance removes all provenance records for a campaign+extension.
func (r *extensionRepository) DeleteProvenance(ctx context.Context, campaignID, extensionID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM extension_provenance WHERE campaign_id = ? AND extension_id = ?`,
		campaignID, extensionID,
	)
	return err
}

// --- Extension Data ---

// SetData upserts a key-value data record for an extension in a campaign.
func (r *extensionRepository) SetData(ctx context.Context, d *ExtensionData) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO extension_data (campaign_id, extension_id, namespace, data_key, data_value)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE data_value = VALUES(data_value)`,
		d.CampaignID, d.ExtensionID, d.Namespace, d.DataKey, d.DataValue,
	)
	return err
}

// GetData returns a single data record.
func (r *extensionRepository) GetData(ctx context.Context, campaignID, extensionID, namespace, key string) (*ExtensionData, error) {
	var d ExtensionData
	err := r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, extension_id, namespace, data_key, data_value
		 FROM extension_data WHERE campaign_id = ? AND extension_id = ? AND namespace = ? AND data_key = ?`,
		campaignID, extensionID, namespace, key,
	).Scan(&d.ID, &d.CampaignID, &d.ExtensionID, &d.Namespace, &d.DataKey, &d.DataValue)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &d, err
}

// ListData returns all data records for an extension+namespace in a campaign.
func (r *extensionRepository) ListData(ctx context.Context, campaignID, extensionID, namespace string) ([]ExtensionData, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, campaign_id, extension_id, namespace, data_key, data_value
		 FROM extension_data WHERE campaign_id = ? AND extension_id = ? AND namespace = ?
		 ORDER BY data_key`,
		campaignID, extensionID, namespace,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ExtensionData
	for rows.Next() {
		var d ExtensionData
		if err := rows.Scan(&d.ID, &d.CampaignID, &d.ExtensionID, &d.Namespace, &d.DataKey, &d.DataValue); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// DeleteData removes all data records for an extension in a campaign.
func (r *extensionRepository) DeleteData(ctx context.Context, campaignID, extensionID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM extension_data WHERE campaign_id = ? AND extension_id = ?`,
		campaignID, extensionID,
	)
	return err
}

// DeleteDataByKey removes a single data record by namespace+key.
func (r *extensionRepository) DeleteDataByKey(ctx context.Context, campaignID, extensionID, namespace, key string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM extension_data WHERE campaign_id = ? AND extension_id = ? AND namespace = ? AND data_key = ?`,
		campaignID, extensionID, namespace, key,
	)
	return err
}

