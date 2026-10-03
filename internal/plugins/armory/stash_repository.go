// stash_repository.go provides data access for stashes, the move history and
// the downtime switch. All SQL lives here. Every statement that touches a
// stash is scoped by campaign_id so an id from another campaign matches
// nothing, and every balance change is a conditional UPDATE so a debit can
// never take more than is there even if two writers race.
package armory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// StashRepository defines the data access contract for stashes and moves.
type StashRepository interface {
	// CreateStash inserts a stash and sets its ID. A duplicate name in the
	// campaign is a Conflict.
	CreateStash(ctx context.Context, s *Stash) error
	// GetStash returns one stash in the campaign, or NotFound.
	GetStash(ctx context.Context, campaignID string, id int) (*Stash, error)
	// ListStashes returns every stash in the campaign by name.
	ListStashes(ctx context.Context, campaignID string) ([]Stash, error)
	// UpdateStash writes name and location. A duplicate name is a Conflict.
	UpdateStash(ctx context.Context, s *Stash) error
	// DeleteStash removes a stash; its items and viewers go with it.
	DeleteStash(ctx context.Context, campaignID string, id int) error

	// SetViewers replaces the characters that can see a stash, atomically.
	SetViewers(ctx context.Context, campaignID string, stashID int, characterIDs []string) error
	// ListViewers returns every stash's viewer character ids in the campaign.
	ListViewers(ctx context.Context, campaignID string) (map[int][]string, error)

	// ListItems returns every stash's item lines in the campaign.
	ListItems(ctx context.Context, campaignID string) (map[int][]StashItemRow, error)
	// CreditItem adds quantity of an item to a stash.
	CreditItem(ctx context.Context, campaignID string, stashID int, itemID string, quantity int) error
	// DebitItem removes quantity of an item, reporting false (and changing
	// nothing) when the stash holds less.
	DebitItem(ctx context.Context, campaignID string, stashID int, itemID string, quantity int) (bool, error)
	// CreditMoney adds money to a stash.
	CreditMoney(ctx context.Context, campaignID string, stashID int, amount Cents) error
	// DebitMoney removes money, reporting false when the stash holds less.
	DebitMoney(ctx context.Context, campaignID string, stashID int, amount Cents) (bool, error)

	// InsertMove writes a history row and sets its ID.
	InsertMove(ctx context.Context, m *Move) error
	// GetMove returns one move in the campaign, or NotFound.
	GetMove(ctx context.Context, campaignID string, id int64) (*Move, error)
	// SettleMove records the outcome of a PENDING move. It reports false when
	// the move was no longer pending, so two decisions can't both win.
	SettleMove(ctx context.Context, campaignID string, id int64, status, reason, decidedBy string) (bool, error)
	// ListMoves returns history rows newest first.
	ListMoves(ctx context.Context, campaignID string, f MoveFilter) ([]Move, error)
	// ListPending returns the campaign's pending requests, oldest first.
	ListPending(ctx context.Context, campaignID string) ([]Move, error)

	// GetDowntime returns the switch; a campaign with no row is closed.
	GetDowntime(ctx context.Context, campaignID string) (*Downtime, error)
	// SetDowntime writes the switch.
	SetDowntime(ctx context.Context, campaignID string, open bool, by string) error
}

type stashRepository struct {
	db *sql.DB
}

// NewStashRepository creates a stash repository backed by db.
func NewStashRepository(db *sql.DB) StashRepository {
	return &stashRepository{db: db}
}

func isDuplicateKey(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}

// stashSelect reads money as whole cents so the Go side never parses a decimal.
const stashSelect = `SELECT id, campaign_id, name, COALESCE(location, ''),
	CAST(ROUND(money * 100) AS SIGNED), COALESCE(created_by, ''), created_at, updated_at
	FROM stashes`

func scanStash(sc interface{ Scan(...any) error }) (*Stash, error) {
	var s Stash
	var cents int64
	if err := sc.Scan(&s.ID, &s.CampaignID, &s.Name, &s.Location, &cents, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.Money = Cents(cents)
	return &s, nil
}

func (r *stashRepository) CreateStash(ctx context.Context, s *Stash) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO stashes (campaign_id, name, location, created_by) VALUES (?, ?, ?, ?)`,
		s.CampaignID, s.Name, nullStr(s.Location), nullStr(s.CreatedBy))
	if err != nil {
		if isDuplicateKey(err) {
			return apperror.NewConflict("A stash with that name already exists.")
		}
		return fmt.Errorf("inserting stash: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting stash id: %w", err)
	}
	s.ID = int(id)
	return nil
}

func (r *stashRepository) GetStash(ctx context.Context, campaignID string, id int) (*Stash, error) {
	s, err := scanStash(r.db.QueryRowContext(ctx, stashSelect+` WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("stash")
	}
	if err != nil {
		return nil, fmt.Errorf("getting stash: %w", err)
	}
	return s, nil
}

func (r *stashRepository) ListStashes(ctx context.Context, campaignID string) ([]Stash, error) {
	rows, err := r.db.QueryContext(ctx, stashSelect+` WHERE campaign_id = ? ORDER BY name ASC, id ASC`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing stashes: %w", err)
	}
	defer rows.Close()
	var out []Stash
	for rows.Next() {
		s, err := scanStash(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning stash: %w", err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *stashRepository) UpdateStash(ctx context.Context, s *Stash) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE stashes SET name = ?, location = ? WHERE campaign_id = ? AND id = ?`,
		s.Name, nullStr(s.Location), s.CampaignID, s.ID)
	if err != nil {
		if isDuplicateKey(err) {
			return apperror.NewConflict("A stash with that name already exists.")
		}
		return fmt.Errorf("updating stash: %w", err)
	}
	// MariaDB reports CHANGED rows, so an unchanged save is 0 too; the caller
	// has already loaded the stash in this campaign, so 0 is not "missing".
	_, _ = res.RowsAffected()
	return nil
}

func (r *stashRepository) DeleteStash(ctx context.Context, campaignID string, id int) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM stashes WHERE campaign_id = ? AND id = ?`, campaignID, id)
	if err != nil {
		return fmt.Errorf("deleting stash: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return apperror.NewNotFound("stash")
	}
	return nil
}

func (r *stashRepository) SetViewers(ctx context.Context, campaignID string, stashID int, characterIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning viewer update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the stash row inside the campaign so a delete can't interleave.
	var locked int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM stashes WHERE campaign_id = ? AND id = ? FOR UPDATE`, campaignID, stashID).Scan(&locked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperror.NewNotFound("stash")
		}
		return fmt.Errorf("locking stash: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM stash_viewers WHERE stash_id = ?`, stashID); err != nil {
		return fmt.Errorf("clearing viewers: %w", err)
	}
	for _, id := range characterIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO stash_viewers (stash_id, character_entity_id) VALUES (?, ?)`, stashID, id); err != nil {
			return fmt.Errorf("adding viewer: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing viewers: %w", err)
	}
	return nil
}

func (r *stashRepository) ListViewers(ctx context.Context, campaignID string) (map[int][]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT sv.stash_id, sv.character_entity_id
		   FROM stash_viewers sv
		   INNER JOIN stashes s ON s.id = sv.stash_id
		  WHERE s.campaign_id = ?
		  ORDER BY sv.stash_id, sv.character_entity_id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing stash viewers: %w", err)
	}
	defer rows.Close()
	out := map[int][]string{}
	for rows.Next() {
		var sid int
		var cid string
		if err := rows.Scan(&sid, &cid); err != nil {
			return nil, fmt.Errorf("scanning stash viewer: %w", err)
		}
		out[sid] = append(out[sid], cid)
	}
	return out, rows.Err()
}

func (r *stashRepository) ListItems(ctx context.Context, campaignID string) (map[int][]StashItemRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT si.stash_id, si.item_entity_id, si.quantity
		   FROM stash_items si
		   INNER JOIN stashes s ON s.id = si.stash_id
		  WHERE s.campaign_id = ?
		  ORDER BY si.stash_id, si.item_entity_id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing stash items: %w", err)
	}
	defer rows.Close()
	out := map[int][]StashItemRow{}
	for rows.Next() {
		var sid int
		var row StashItemRow
		if err := rows.Scan(&sid, &row.ItemEntityID, &row.Quantity); err != nil {
			return nil, fmt.Errorf("scanning stash item: %w", err)
		}
		out[sid] = append(out[sid], row)
	}
	return out, rows.Err()
}

func (r *stashRepository) CreditItem(ctx context.Context, campaignID string, stashID int, itemID string, quantity int) error {
	// INSERT ... SELECT keeps the campaign scope in the statement itself: a
	// stash id from another campaign inserts nothing.
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO stash_items (stash_id, item_entity_id, quantity)
		 SELECT id, ?, ? FROM stashes WHERE id = ? AND campaign_id = ?
		 ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)`,
		itemID, quantity, stashID, campaignID)
	if err != nil {
		return fmt.Errorf("crediting stash item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("stash")
	}
	return nil
}

func (r *stashRepository) DebitItem(ctx context.Context, campaignID string, stashID int, itemID string, quantity int) (bool, error) {
	// Exactly-all removes the line (a stored quantity is always > 0); less
	// than all subtracts. The two conditions are mutually exclusive, so at most
	// one statement matches, and neither can take more than is there.
	res, err := r.db.ExecContext(ctx,
		`DELETE si FROM stash_items si
		   INNER JOIN stashes s ON s.id = si.stash_id
		  WHERE si.stash_id = ? AND si.item_entity_id = ? AND si.quantity = ? AND s.campaign_id = ?`,
		stashID, itemID, quantity, campaignID)
	if err != nil {
		return false, fmt.Errorf("removing stash item: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return true, nil
	}
	res, err = r.db.ExecContext(ctx,
		`UPDATE stash_items si
		   INNER JOIN stashes s ON s.id = si.stash_id
		    SET si.quantity = si.quantity - ?
		  WHERE si.stash_id = ? AND si.item_entity_id = ? AND si.quantity > ? AND s.campaign_id = ?`,
		quantity, stashID, itemID, quantity, campaignID)
	if err != nil {
		return false, fmt.Errorf("debiting stash item: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}
	return n == 1, nil
}

func (r *stashRepository) CreditMoney(ctx context.Context, campaignID string, stashID int, amount Cents) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE stashes SET money = money + ? WHERE id = ? AND campaign_id = ?`,
		amount.Decimal(), stashID, campaignID)
	if err != nil {
		return fmt.Errorf("crediting stash money: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("stash")
	}
	return nil
}

func (r *stashRepository) DebitMoney(ctx context.Context, campaignID string, stashID int, amount Cents) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE stashes SET money = money - ? WHERE id = ? AND campaign_id = ? AND money >= ?`,
		amount.Decimal(), stashID, campaignID, amount.Decimal())
	if err != nil {
		return false, fmt.Errorf("debiting stash money: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}
	return n == 1, nil
}

func (r *stashRepository) InsertMove(ctx context.Context, m *Move) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO item_moves
		   (campaign_id, kind, item_entity_id, quantity, amount, from_kind, from_id,
		    to_kind, to_id, status, reason, requested_by, decided_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.CampaignID, m.Kind, nullStr(m.ItemEntityID), nullPositiveInt(m.Quantity), nullAmount(m.Amount),
		m.From.Kind, m.From.ID, m.To.Kind, m.To.ID, m.Status, nullStr(m.Reason),
		m.RequestedBy, nullStr(m.DecidedBy))
	if err != nil {
		return fmt.Errorf("inserting move: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting move id: %w", err)
	}
	m.ID = id
	return nil
}

const moveSelect = `SELECT id, campaign_id, kind, COALESCE(item_entity_id, ''), COALESCE(quantity, 0),
	CAST(ROUND(COALESCE(amount, 0) * 100) AS SIGNED), from_kind, from_id, to_kind, to_id,
	status, COALESCE(reason, ''), requested_by, COALESCE(decided_by, ''), created_at, decided_at
	FROM item_moves`

func scanMove(sc interface{ Scan(...any) error }) (*Move, error) {
	var m Move
	var cents int64
	var decided sql.NullTime
	if err := sc.Scan(&m.ID, &m.CampaignID, &m.Kind, &m.ItemEntityID, &m.Quantity, &cents,
		&m.From.Kind, &m.From.ID, &m.To.Kind, &m.To.ID, &m.Status, &m.Reason,
		&m.RequestedBy, &m.DecidedBy, &m.CreatedAt, &decided); err != nil {
		return nil, err
	}
	m.Amount = Cents(cents)
	if decided.Valid {
		t := decided.Time
		m.DecidedAt = &t
	}
	return &m, nil
}

func (r *stashRepository) GetMove(ctx context.Context, campaignID string, id int64) (*Move, error) {
	m, err := scanMove(r.db.QueryRowContext(ctx, moveSelect+` WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("request")
	}
	if err != nil {
		return nil, fmt.Errorf("getting move: %w", err)
	}
	return m, nil
}

func (r *stashRepository) SettleMove(ctx context.Context, campaignID string, id int64, status, reason, decidedBy string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE item_moves SET status = ?, reason = ?, decided_by = ?, decided_at = CURRENT_TIMESTAMP
		  WHERE campaign_id = ? AND id = ? AND status = 'pending'`,
		status, nullStr(reason), nullStr(decidedBy), campaignID, id)
	if err != nil {
		return false, fmt.Errorf("settling move: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("checking rows affected: %w", err)
	}
	return n == 1, nil
}

func (r *stashRepository) ListMoves(ctx context.Context, campaignID string, f MoveFilter) ([]Move, error) {
	where := []string{"campaign_id = ?"}
	args := []any{campaignID}
	if f.Endpoint != nil {
		where = append(where, "((from_kind = ? AND from_id = ?) OR (to_kind = ? AND to_id = ?))")
		args = append(args, f.Endpoint.Kind, f.Endpoint.ID, f.Endpoint.Kind, f.Endpoint.ID)
	}
	if f.RequestedBy != "" {
		where = append(where, "requested_by = ?")
		args = append(args, f.RequestedBy)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	limit := f.Limit
	if limit < 1 || limit > 200 {
		limit = historyPageSize
	}
	q := moveSelect + " WHERE " + strings.Join(where, " AND ") + " ORDER BY created_at DESC, id DESC LIMIT " + strconv.Itoa(limit)
	return r.queryMoves(ctx, q, args...)
}

func (r *stashRepository) ListPending(ctx context.Context, campaignID string) ([]Move, error) {
	return r.queryMoves(ctx, moveSelect+` WHERE campaign_id = ? AND status = 'pending' ORDER BY created_at ASC, id ASC`, campaignID)
}

func (r *stashRepository) queryMoves(ctx context.Context, q string, args ...any) ([]Move, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing moves: %w", err)
	}
	defer rows.Close()
	var out []Move
	for rows.Next() {
		m, err := scanMove(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning move: %w", err)
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (r *stashRepository) GetDowntime(ctx context.Context, campaignID string) (*Downtime, error) {
	var d Downtime
	var by sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT is_open, changed_by, changed_at FROM campaign_downtime WHERE campaign_id = ?`, campaignID).
		Scan(&d.IsOpen, &by, &d.ChangedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return &Downtime{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading downtime: %w", err)
	}
	d.ChangedBy = by.String
	return &d, nil
}

func (r *stashRepository) SetDowntime(ctx context.Context, campaignID string, open bool, by string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_downtime (campaign_id, is_open, changed_by, changed_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON DUPLICATE KEY UPDATE is_open = VALUES(is_open), changed_by = VALUES(changed_by), changed_at = CURRENT_TIMESTAMP`,
		campaignID, open, nullStr(by))
	if err != nil {
		return fmt.Errorf("writing downtime: %w", err)
	}
	return nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullPositiveInt(n int) any {
	if n <= 0 {
		return nil
	}
	return n
}

func nullAmount(c Cents) any {
	if c <= 0 {
		return nil
	}
	return c.Decimal()
}
