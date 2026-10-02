package syncapi

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SyncChange is one row of the change feed. It carries ids only: a client
// refetches the resource through the normal REST reads, which is where
// visibility is applied.
type SyncChange struct {
	Seq        int64  `json:"seq"`
	Type       string `json:"type"`
	ResourceID string `json:"resourceId"`
	Op         string `json:"op"`
}

// SyncChangeRepository owns the sync_changes and sync_change_watermarks
// tables.
type SyncChangeRepository interface {
	// Append records a change and returns its sequence number.
	Append(ctx context.Context, campaignID, resourceType, resourceID, op string) (int64, error)
	// List returns up to limit changes with seq > since, oldest first.
	List(ctx context.Context, campaignID string, since int64, limit int) ([]SyncChange, error)
	// PrunedThrough is the highest seq retention has deleted for the
	// campaign, or 0 if nothing has been pruned.
	PrunedThrough(ctx context.Context, campaignID string) (int64, error)
	// Head is the highest seq recorded for the campaign, or 0 if none.
	Head(ctx context.Context, campaignID string) (int64, error)
	// PruneOlderThan deletes rows older than cutoff and raises each affected
	// campaign's watermark first, so a crash between the two steps leaves a
	// watermark that over-reports a gap rather than hiding one. Returns the
	// number of rows deleted.
	PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type syncChangeRepo struct {
	db *sql.DB
}

// NewSyncChangeRepository creates the MariaDB-backed change feed repository.
func NewSyncChangeRepository(db *sql.DB) SyncChangeRepository {
	return &syncChangeRepo{db: db}
}

func (r *syncChangeRepo) Append(ctx context.Context, campaignID, resourceType, resourceID, op string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO sync_changes (campaign_id, resource_type, resource_id, op) VALUES (?, ?, ?, ?)`,
		campaignID, resourceType, resourceID, op)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("append sync change: %w", err))
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("sync change id: %w", err))
	}
	return seq, nil
}

func (r *syncChangeRepo) List(ctx context.Context, campaignID string, since int64, limit int) ([]SyncChange, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT seq, resource_type, resource_id, op FROM sync_changes
		 WHERE campaign_id = ? AND seq > ? ORDER BY seq ASC LIMIT ?`,
		campaignID, since, limit)
	if err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("list sync changes: %w", err))
	}
	defer rows.Close()

	out := []SyncChange{}
	for rows.Next() {
		var ch SyncChange
		if err := rows.Scan(&ch.Seq, &ch.Type, &ch.ResourceID, &ch.Op); err != nil {
			return nil, apperror.NewInternal(fmt.Errorf("scan sync change: %w", err))
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(fmt.Errorf("iterate sync changes: %w", err))
	}
	return out, nil
}

func (r *syncChangeRepo) PrunedThrough(ctx context.Context, campaignID string) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx,
		`SELECT pruned_through FROM sync_change_watermarks WHERE campaign_id = ?`, campaignID).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("read sync watermark: %w", err))
	}
	return n, nil
}

func (r *syncChangeRepo) Head(ctx context.Context, campaignID string) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM sync_changes WHERE campaign_id = ?`, campaignID).Scan(&n)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("read sync head: %w", err))
	}
	return n, nil
}

func (r *syncChangeRepo) PruneOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT campaign_id, MAX(seq) FROM sync_changes WHERE created_at < ? GROUP BY campaign_id`, cutoff)
	if err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("find prunable sync changes: %w", err))
	}
	type target struct {
		campaignID string
		maxSeq     int64
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.campaignID, &t.maxSeq); err != nil {
			rows.Close()
			return 0, apperror.NewInternal(fmt.Errorf("scan prunable sync changes: %w", err))
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, apperror.NewInternal(fmt.Errorf("iterate prunable sync changes: %w", err))
	}

	var total int64
	for _, t := range targets {
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO sync_change_watermarks (campaign_id, pruned_through) VALUES (?, ?)
			 ON DUPLICATE KEY UPDATE pruned_through = GREATEST(pruned_through, VALUES(pruned_through))`,
			t.campaignID, t.maxSeq); err != nil {
			return total, apperror.NewInternal(fmt.Errorf("raise sync watermark: %w", err))
		}
		res, err := r.db.ExecContext(ctx,
			`DELETE FROM sync_changes WHERE campaign_id = ? AND seq <= ?`, t.campaignID, t.maxSeq)
		if err != nil {
			return total, apperror.NewInternal(fmt.Errorf("prune sync changes: %w", err))
		}
		n, _ := res.RowsAffected()
		total += n
	}
	return total, nil
}
