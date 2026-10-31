// copies_repository.go holds the SQL for remembered copies (copies.go) and
// the copy a waiting move request picked. Every statement is scoped by
// campaign_id, so an id from another campaign matches nothing.
package armory

import (
	"context"
	"database/sql"
	"errors"
)

// CopyStore is the data access for remembered copies. Copies come oldest
// first.
type CopyStore interface {
	// List returns a holder's copies; itemID "" lists every item's.
	List(ctx context.Context, campaignID, holderKind, holderID, itemID string) ([]Copy, error)
	// Add stores copies at a holder and returns the row's id.
	Add(ctx context.Context, campaignID, holderKind, holderID string, c Copy) (int64, error)
	// SetQuantity changes a row's count; 0 removes it.
	SetQuantity(ctx context.Context, campaignID string, id int64, qty int) error
	// Rehome moves a row to another holder.
	Rehome(ctx context.Context, campaignID string, id int64, holderKind, holderID string) error
	// Remember sets a row's memory.
	Remember(ctx context.Context, campaignID string, id int64, m Memory) error
	// DueOff lists every copy in the campaign that goes off on or before day.
	DueOff(ctx context.Context, campaignID string, day int64) ([]HeldCopy, error)
	// Pick keeps the copy a waiting move asked for; Picked reads it back (0
	// for none).
	Pick(ctx context.Context, moveID, copyID int64) error
	Picked(ctx context.Context, moveID int64) (int64, error)
}

// HeldCopy is a copy with its holder.
type HeldCopy struct {
	Copy
	HolderKind string
	HolderID   string
}

type copyRepository struct {
	db *sql.DB
}

// NewCopyRepository returns the SQL copy store.
func NewCopyRepository(db *sql.DB) CopyStore { return &copyRepository{db: db} }

func (r *copyRepository) List(ctx context.Context, campaignID, holderKind, holderID, itemID string) ([]Copy, error) {
	q := `SELECT id, item_entity_id, quantity, memory FROM armory_item_copies
		WHERE campaign_id = ? AND holder_kind = ? AND holder_id = ?`
	args := []any{campaignID, holderKind, holderID}
	if itemID != "" {
		q += ` AND item_entity_id = ?`
		args = append(args, itemID)
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Copy
	for rows.Next() {
		var c Copy
		var mem string
		if err := rows.Scan(&c.ID, &c.ItemID, &c.Quantity, &mem); err != nil {
			return nil, err
		}
		c.Memory = decodeMemory(mem)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *copyRepository) Add(ctx context.Context, campaignID, holderKind, holderID string, c Copy) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO armory_item_copies (campaign_id, holder_kind, holder_id, item_entity_id, quantity, memory) VALUES (?, ?, ?, ?, ?, ?)`,
		campaignID, holderKind, holderID, c.ItemID, c.Quantity, encodeMemory(c.Memory))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *copyRepository) SetQuantity(ctx context.Context, campaignID string, id int64, qty int) error {
	if qty <= 0 {
		_, err := r.db.ExecContext(ctx, `DELETE FROM armory_item_copies WHERE campaign_id = ? AND id = ?`, campaignID, id)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE armory_item_copies SET quantity = ? WHERE campaign_id = ? AND id = ?`, qty, campaignID, id)
	return err
}

func (r *copyRepository) Rehome(ctx context.Context, campaignID string, id int64, holderKind, holderID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE armory_item_copies SET holder_kind = ?, holder_id = ? WHERE campaign_id = ? AND id = ?`,
		holderKind, holderID, campaignID, id)
	return err
}

func (r *copyRepository) Remember(ctx context.Context, campaignID string, id int64, m Memory) error {
	_, err := r.db.ExecContext(ctx, `UPDATE armory_item_copies SET memory = ? WHERE campaign_id = ? AND id = ?`, encodeMemory(m), campaignID, id)
	return err
}

func (r *copyRepository) DueOff(ctx context.Context, campaignID string, day int64) ([]HeldCopy, error) {
	// memory is JSON the service writes; the day is read back in Go rather
	// than trusting a JSON path in SQL.
	rows, err := r.db.QueryContext(ctx, `SELECT id, holder_kind, holder_id, item_entity_id, quantity, memory FROM armory_item_copies
		WHERE campaign_id = ? AND memory LIKE '%"offDay"%' ORDER BY id`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HeldCopy
	for rows.Next() {
		var c HeldCopy
		var mem string
		if err := rows.Scan(&c.ID, &c.HolderKind, &c.HolderID, &c.ItemID, &c.Quantity, &mem); err != nil {
			return nil, err
		}
		c.Memory = decodeMemory(mem)
		if c.Memory.OffDay > 0 && c.Memory.OffDay <= day {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

func (r *copyRepository) Pick(ctx context.Context, moveID, copyID int64) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO armory_move_picks (move_id, copy_id) VALUES (?, ?)`, moveID, copyID)
	return err
}

func (r *copyRepository) Picked(ctx context.Context, moveID int64) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT copy_id FROM armory_move_picks WHERE move_id = ?`, moveID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}
