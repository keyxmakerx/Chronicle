package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AuditRepository defines the data access contract for audit log operations.
// All SQL lives in the concrete implementation -- no SQL leaks out.
type AuditRepository interface {
	// Log inserts a new audit entry into the database.
	Log(ctx context.Context, entry *AuditEntry) error

	// ListByCampaign returns paginated audit entries for a campaign, most
	// recent first. Joins the users table to include display_name. Returns
	// the entries, total count (for pagination), and any error.
	ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]AuditEntry, int, error)

	// ListByEntity returns the most recent audit entries for a specific entity.
	// Used for entity-level change history.
	ListByEntity(ctx context.Context, entityID string, limit int) ([]AuditEntry, error)

	// CountByCampaign returns the total number of audit entries for a campaign.
	CountByCampaign(ctx context.Context, campaignID string) (int, error)

	// GetCampaignStats returns aggregate statistics for a campaign including
	// entity count, approximate word count, last edit time, and active editors.
	GetCampaignStats(ctx context.Context, campaignID string) (*CampaignStats, error)
}

// auditRepository implements AuditRepository with MariaDB queries.
type auditRepository struct {
	db *sql.DB
}

// NewAuditRepository creates a new repository backed by the given DB pool.
func NewAuditRepository(db *sql.DB) AuditRepository {
	return &auditRepository{db: db}
}

// Log inserts a new audit entry. The details map is serialized to JSON
// before storage. Nil details are stored as SQL NULL.
func (r *auditRepository) Log(ctx context.Context, entry *AuditEntry) error {
	query := `INSERT INTO audit_log (campaign_id, user_id, action, entity_type, entity_id, entity_name, details, created_at)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	var detailsJSON []byte
	if entry.Details != nil {
		var err error
		detailsJSON, err = json.Marshal(entry.Details)
		if err != nil {
			return fmt.Errorf("marshaling audit details: %w", err)
		}
	}

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	result, err := r.db.ExecContext(ctx, query,
		entry.CampaignID, entry.UserID, entry.Action,
		entry.EntityType, entry.EntityID, entry.EntityName,
		detailsJSON, entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting audit entry: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting audit entry id: %w", err)
	}
	entry.ID = id

	return nil
}

// ListByCampaign returns audit entries for a campaign ordered by most recent
// first. Joins users table to include display_name for the activity feed.
func (r *auditRepository) ListByCampaign(ctx context.Context, campaignID string, limit, offset int) ([]AuditEntry, int, error) {
	// Count total entries for pagination.
	countQuery := `SELECT COUNT(*) FROM audit_log WHERE campaign_id = ?`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, campaignID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting audit entries: %w", err)
	}

	query := `SELECT a.id, a.campaign_id, a.user_id, a.action,
	                 a.entity_type, a.entity_id, a.entity_name,
	                 a.details, a.created_at,
	                 COALESCE(u.display_name, 'Unknown User') AS user_name
	          FROM audit_log a
	          LEFT JOIN users u ON u.id = a.user_id
	          WHERE a.campaign_id = ?
	          ORDER BY a.created_at DESC
	          LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, campaignID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing audit entries: %w", err)
	}
	defer rows.Close()

	entries, err := scanAuditRows(rows)
	if err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// ListByEntity returns the most recent audit entries for a specific entity.
// Joins users table for display names.
func (r *auditRepository) ListByEntity(ctx context.Context, entityID string, limit int) ([]AuditEntry, error) {
	query := `SELECT a.id, a.campaign_id, a.user_id, a.action,
	                 a.entity_type, a.entity_id, a.entity_name,
	                 a.details, a.created_at,
	                 COALESCE(u.display_name, 'Unknown User') AS user_name
	          FROM audit_log a
	          LEFT JOIN users u ON u.id = a.user_id
	          WHERE a.entity_id = ?
	          ORDER BY a.created_at DESC
	          LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing entity audit entries: %w", err)
	}
	defer rows.Close()

	return scanAuditRows(rows)
}

// CountByCampaign returns the total number of audit entries for a campaign.
func (r *auditRepository) CountByCampaign(ctx context.Context, campaignID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE campaign_id = ?`, campaignID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting campaign audit entries: %w", err)
	}
	return count, nil
}

// GetCampaignStats computes aggregate statistics for a campaign by querying
// across entities and audit_log tables. The word count is approximated by
// counting spaces in the HTML content (fast but rough).
func (r *auditRepository) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignStats, error) {
	stats := &CampaignStats{}

	// Entity count and approximate word count from entities table.
	// Word count = number of spaces + 1 for each non-empty entry.
	entityQuery := `SELECT COUNT(*),
	                       COALESCE(SUM(LENGTH(entry_html) - LENGTH(REPLACE(entry_html, ' ', '')) + 1), 0)
	                FROM entities WHERE campaign_id = ?`
	if err := r.db.QueryRowContext(ctx, entityQuery, campaignID).Scan(
		&stats.TotalEntities, &stats.TotalWords,
	); err != nil {
		return nil, fmt.Errorf("querying entity stats: %w", err)
	}

	// Most recent audit entry timestamp.
	lastEditQuery := `SELECT MAX(created_at) FROM audit_log WHERE campaign_id = ?`
	var lastEdit sql.NullTime
	if err := r.db.QueryRowContext(ctx, lastEditQuery, campaignID).Scan(&lastEdit); err != nil {
		return nil, fmt.Errorf("querying last edit time: %w", err)
	}
	if lastEdit.Valid {
		stats.LastEditedAt = &lastEdit.Time
	}

	// Distinct active editors in the last 30 days.
	editorsQuery := `SELECT COUNT(DISTINCT user_id) FROM audit_log
	                 WHERE campaign_id = ? AND created_at >= DATE_SUB(NOW(), INTERVAL 30 DAY)`
	if err := r.db.QueryRowContext(ctx, editorsQuery, campaignID).Scan(&stats.ActiveEditors); err != nil {
		return nil, fmt.Errorf("querying active editors: %w", err)
	}

	return stats, nil
}

// scanAuditRows scans rows from an audit_log query into AuditEntry slices.
// Expects columns: id, campaign_id, user_id, action, entity_type, entity_id,
// entity_name, details, created_at, user_name.
func scanAuditRows(rows *sql.Rows) ([]AuditEntry, error) {
	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var detailsJSON sql.NullString
		if err := rows.Scan(
			&e.ID, &e.CampaignID, &e.UserID, &e.Action,
			&e.EntityType, &e.EntityID, &e.EntityName,
			&detailsJSON, &e.CreatedAt, &e.UserName,
		); err != nil {
			return nil, fmt.Errorf("scanning audit entry: %w", err)
		}

		// Deserialize JSON details if present.
		if detailsJSON.Valid && detailsJSON.String != "" {
			if err := json.Unmarshal([]byte(detailsJSON.String), &e.Details); err != nil {
				// Non-fatal: log the issue but don't break the feed.
				e.Details = map[string]any{"_parse_error": "invalid JSON"}
			}
		}

		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit rows: %w", err)
	}

	return entries, nil
}
