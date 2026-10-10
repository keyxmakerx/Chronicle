package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Trash batch states. A batch moves trashed -> restoring (Undo claimed it) or
// trashed -> purging (the final delete claimed it). Each move is one
// conditional update, so exactly one of Undo and purge can win, and a run that
// stops partway leaves the batch in the state it must be finished in.
const (
	batchTrashed   = "trashed"
	batchRestoring = "restoring"
	batchPurging   = "purging"
)

// trashKindFiles is the only batch kind today: an admin's file clean-up.
const trashKindFiles = "files"

// TrashBatch is one file clean-up waiting in the Trash.
type TrashBatch struct {
	ID    string
	Kind  string
	Label string // what the files are, e.g. "pictures" or "files"
	Items int
	Bytes int64
	State string
	// DeletedBy / DeletedByName are who ran the clean-up; the name is a copy
	// so the Trash still reads right after the account is gone.
	DeletedBy     string
	DeletedByName string
	CreatedAt     time.Time
}

// TrashBatchRepository owns the trash_batches table. The files themselves are
// marked and restored through the media service, not here.
type TrashBatchRepository interface {
	Create(ctx context.Context, b *TrashBatch) error
	SetTotals(ctx context.Context, id string, items int, bytes int64) error
	Get(ctx context.Context, id string) (*TrashBatch, error)
	List(ctx context.Context) ([]TrashBatch, error)
	// Claim moves a batch from one state to another and reports whether this
	// call made the move.
	Claim(ctx context.Context, id, from, to string) (bool, error)
	Delete(ctx context.Context, id string) error
}

type trashBatchRepository struct {
	db *sql.DB
}

// NewTrashBatchRepository creates the MariaDB trash batch repository.
func NewTrashBatchRepository(db *sql.DB) TrashBatchRepository {
	return &trashBatchRepository{db: db}
}

const trashBatchColumns = `id, kind, label, item_count, byte_count, state,
	COALESCE(deleted_by, ''), COALESCE(deleted_by_name, ''), created_at`

func scanTrashBatch(scan func(dest ...any) error) (*TrashBatch, error) {
	b := &TrashBatch{}
	if err := scan(&b.ID, &b.Kind, &b.Label, &b.Items, &b.Bytes, &b.State,
		&b.DeletedBy, &b.DeletedByName, &b.CreatedAt); err != nil {
		return nil, err
	}
	return b, nil
}

func (r *trashBatchRepository) Create(ctx context.Context, b *TrashBatch) error {
	var by, name any
	if b.DeletedBy != "" {
		by = b.DeletedBy
	}
	if b.DeletedByName != "" {
		name = b.DeletedByName
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO trash_batches (id, kind, label, item_count, byte_count, state, deleted_by, deleted_by_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.Kind, b.Label, b.Items, b.Bytes, batchTrashed, by, name, b.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating trash batch: %w", err)
	}
	return nil
}

func (r *trashBatchRepository) SetTotals(ctx context.Context, id string, items int, bytes int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE trash_batches SET item_count = ?, byte_count = ? WHERE id = ?`, items, bytes, id); err != nil {
		return fmt.Errorf("updating trash batch totals: %w", err)
	}
	return nil
}

// Get returns nil, nil for a batch that does not exist.
func (r *trashBatchRepository) Get(ctx context.Context, id string) (*TrashBatch, error) {
	b, err := scanTrashBatch(r.db.QueryRowContext(ctx,
		`SELECT `+trashBatchColumns+` FROM trash_batches WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading trash batch: %w", err)
	}
	return b, nil
}

func (r *trashBatchRepository) List(ctx context.Context) ([]TrashBatch, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+trashBatchColumns+` FROM trash_batches ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("listing trash batches: %w", err)
	}
	defer rows.Close()
	var out []TrashBatch
	for rows.Next() {
		b, err := scanTrashBatch(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scanning trash batch: %w", err)
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (r *trashBatchRepository) Claim(ctx context.Context, id, from, to string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE trash_batches SET state = ? WHERE id = ? AND state = ?`, to, id, from)
	if err != nil {
		return false, fmt.Errorf("claiming trash batch: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *trashBatchRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM trash_batches WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting trash batch: %w", err)
	}
	return nil
}
