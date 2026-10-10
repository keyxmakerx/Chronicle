// Hand-written SQL for the notice-board tables.
//
// SECURITY: every statement filters on campaign_id, and board and item
// lookups also on the owning home (page or category) / board, so an id from
// another campaign, home or board is simply not found (no cross-tenant or
// cross-home access).
package quests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// BoardRepository is the persistence boundary for boards and their items.
type BoardRepository interface {
	GetLooks(ctx context.Context, campaignID string, h Home) (Looks, error)
	SetLooks(ctx context.Context, campaignID string, h Home, l Looks) error

	ListBoards(ctx context.Context, campaignID string, h Home) ([]Board, error)
	// ListHomes returns every home in the campaign with at least one board.
	ListHomes(ctx context.Context, campaignID string) ([]Home, error)
	// GetBoard returns NotFound unless the board belongs to the home and campaign.
	GetBoard(ctx context.Context, campaignID string, h Home, boardID string) (*Board, error)
	InsertBoard(ctx context.Context, b Board) error
	UpdateBoard(ctx context.Context, b Board) error
	DeleteBoard(ctx context.Context, campaignID string, h Home, boardID string) error
	// SetOrder writes sort_order by position, atomically.
	SetOrder(ctx context.Context, campaignID string, h Home, ids []string) error

	ListItems(ctx context.Context, campaignID string, boardIDs []string) ([]Item, error)
	// GetItem returns NotFound unless the item is on that board and campaign.
	GetItem(ctx context.Context, campaignID, boardID, itemID string) (*Item, error)
	CountItems(ctx context.Context, campaignID, boardID string) (int, error)
	InsertItem(ctx context.Context, it Item) error
	UpdateItem(ctx context.Context, it Item) error
	DeleteItems(ctx context.Context, campaignID, boardID string, ids []string) error
}

type sqlBoardRepository struct{ db *sql.DB }

// NewBoardRepository creates a MariaDB-backed repository.
func NewBoardRepository(db *sql.DB) BoardRepository { return &sqlBoardRepository{db: db} }

// homeFilter is the one place that turns a Home into its WHERE fragment. Both
// fragments are constants, so no caller data reaches the SQL text. A Home
// with neither or both set is refused rather than matching everything or
// nothing.
func homeFilter(h Home) (clause string, arg any, err error) {
	switch {
	case h.EntityID != "" && h.TypeID == 0:
		return "entity_id = ?", h.EntityID, nil
	case h.EntityID == "" && h.TypeID > 0:
		return "entity_type_id = ?", h.TypeID, nil
	default:
		return "", nil, fmt.Errorf("board home must be exactly one of page or category")
	}
}

func (r *sqlBoardRepository) GetLooks(ctx context.Context, campaignID string, h Home) (Looks, error) {
	l := Looks{Board: LookLit, Ledger: LookLit}
	if _, _, err := homeFilter(h); err != nil {
		return Looks{}, fmt.Errorf("get board looks: %w", err)
	}
	query, arg := `SELECT board_look, ledger_look FROM quest_board_pages WHERE entity_id = ? AND campaign_id = ?`, any(h.EntityID)
	if h.IsType() {
		query, arg = `SELECT board_look, ledger_look FROM quest_board_type_looks WHERE entity_type_id = ? AND campaign_id = ?`, h.TypeID
	}
	err := r.db.QueryRowContext(ctx, query, arg, campaignID).Scan(&l.Board, &l.Ledger)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Looks{}, fmt.Errorf("get board looks: %w", err)
	}
	return l, nil
}

func (r *sqlBoardRepository) SetLooks(ctx context.Context, campaignID string, h Home, l Looks) error {
	if _, _, err := homeFilter(h); err != nil {
		return fmt.Errorf("set board looks: %w", err)
	}
	query, arg := `INSERT INTO quest_board_pages (entity_id, campaign_id, board_look, ledger_look) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE board_look = VALUES(board_look), ledger_look = VALUES(ledger_look)`, any(h.EntityID)
	if h.IsType() {
		query, arg = `INSERT INTO quest_board_type_looks (entity_type_id, campaign_id, board_look, ledger_look) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE board_look = VALUES(board_look), ledger_look = VALUES(ledger_look)`, h.TypeID
	}
	if _, err := r.db.ExecContext(ctx, query, arg, campaignID, l.Board, l.Ledger); err != nil {
		return fmt.Errorf("set board looks: %w", err)
	}
	return nil
}

const boardColumns = `id, campaign_id, entity_id, entity_type_id, name, who, sort_order`

func scanBoard(sc interface{ Scan(...any) error }) (Board, error) {
	var b Board
	var ent sql.NullString
	var typ sql.NullInt64
	if err := sc.Scan(&b.ID, &b.CampaignID, &ent, &typ, &b.Name, &b.Who, &b.SortOrder); err != nil {
		return Board{}, err
	}
	b.Home = Home{EntityID: ent.String, TypeID: int(typ.Int64)}
	return b, nil
}

func (r *sqlBoardRepository) ListBoards(ctx context.Context, campaignID string, h Home) ([]Board, error) {
	where, arg, err := homeFilter(h)
	if err != nil {
		return nil, fmt.Errorf("list boards: %w", err)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+boardColumns+` FROM quest_boards
		 WHERE `+where+` AND campaign_id = ? ORDER BY sort_order, created_at, id`, arg, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list boards: %w", err)
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		b, err := scanBoard(rows)
		if err != nil {
			return nil, fmt.Errorf("scan board: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *sqlBoardRepository) ListHomes(ctx context.Context, campaignID string) ([]Home, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT entity_id, entity_type_id FROM quest_boards WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list board homes: %w", err)
	}
	defer rows.Close()
	var out []Home
	for rows.Next() {
		var ent sql.NullString
		var typ sql.NullInt64
		if err := rows.Scan(&ent, &typ); err != nil {
			return nil, fmt.Errorf("scan board home: %w", err)
		}
		out = append(out, Home{EntityID: ent.String, TypeID: int(typ.Int64)})
	}
	return out, rows.Err()
}

func (r *sqlBoardRepository) GetBoard(ctx context.Context, campaignID string, h Home, boardID string) (*Board, error) {
	where, arg, err := homeFilter(h)
	if err != nil {
		return nil, fmt.Errorf("get board: %w", err)
	}
	b, err := scanBoard(r.db.QueryRowContext(ctx,
		`SELECT `+boardColumns+` FROM quest_boards
		 WHERE id = ? AND `+where+` AND campaign_id = ?`, boardID, arg, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound("board")
	}
	if err != nil {
		return nil, fmt.Errorf("get board: %w", err)
	}
	return &b, nil
}

func (r *sqlBoardRepository) InsertBoard(ctx context.Context, b Board) error {
	if _, _, err := homeFilter(b.Home); err != nil {
		return fmt.Errorf("insert board: %w", err)
	}
	var typeID any
	if b.Home.IsType() {
		typeID = b.Home.TypeID
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO quest_boards (id, campaign_id, entity_id, entity_type_id, name, who, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.CampaignID, nullable(b.Home.EntityID), typeID, b.Name, b.Who, b.SortOrder)
	if err != nil {
		return fmt.Errorf("insert board: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) UpdateBoard(ctx context.Context, b Board) error {
	where, arg, err := homeFilter(b.Home)
	if err != nil {
		return fmt.Errorf("update board: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE quest_boards SET name = ?, who = ? WHERE id = ? AND `+where+` AND campaign_id = ?`,
		b.Name, b.Who, b.ID, arg, b.CampaignID); err != nil {
		return fmt.Errorf("update board: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) DeleteBoard(ctx context.Context, campaignID string, h Home, boardID string) error {
	where, arg, err := homeFilter(h)
	if err != nil {
		return fmt.Errorf("delete board: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM quest_boards WHERE id = ? AND `+where+` AND campaign_id = ?`, boardID, arg, campaignID); err != nil {
		return fmt.Errorf("delete board: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) SetOrder(ctx context.Context, campaignID string, h Home, ids []string) error {
	where, arg, err := homeFilter(h)
	if err != nil {
		return fmt.Errorf("order boards: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("order boards: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE quest_boards SET sort_order = ? WHERE id = ? AND `+where+` AND campaign_id = ?`,
			i, id, arg, campaignID); err != nil {
			return fmt.Errorf("order boards: %w", err)
		}
	}
	return tx.Commit()
}

const itemColumns = `id, board_id, campaign_id, kind, x, y, w, r, owner_user_id, by_dm, hidden, text, ref_id, from_item_id, to_item_id`

func scanItem(sc interface{ Scan(...any) error }) (Item, error) {
	var it Item
	var owner, txt, ref, from, to sql.NullString
	if err := sc.Scan(&it.ID, &it.BoardID, &it.CampaignID, &it.Kind, &it.X, &it.Y, &it.W, &it.R,
		&owner, &it.ByDM, &it.Hidden, &txt, &ref, &from, &to); err != nil {
		return Item{}, err
	}
	it.OwnerUserID, it.Text, it.RefID, it.FromID, it.ToID = owner.String, txt.String, ref.String, from.String, to.String
	return it, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *sqlBoardRepository) ListItems(ctx context.Context, campaignID string, boardIDs []string) ([]Item, error) {
	if len(boardIDs) == 0 {
		return nil, nil
	}
	args := []any{campaignID}
	for _, id := range boardIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+itemColumns+` FROM quest_board_items WHERE campaign_id = ? AND board_id IN (`+
			strings.TrimSuffix(strings.Repeat("?,", len(boardIDs)), ",")+`) ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *sqlBoardRepository) GetItem(ctx context.Context, campaignID, boardID, itemID string) (*Item, error) {
	it, err := scanItem(r.db.QueryRowContext(ctx,
		`SELECT `+itemColumns+` FROM quest_board_items WHERE id = ? AND board_id = ? AND campaign_id = ?`,
		itemID, boardID, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound("item")
	}
	if err != nil {
		return nil, fmt.Errorf("get item: %w", err)
	}
	return &it, nil
}

func (r *sqlBoardRepository) CountItems(ctx context.Context, campaignID, boardID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM quest_board_items WHERE board_id = ? AND campaign_id = ?`,
		boardID, campaignID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count items: %w", err)
	}
	return n, nil
}

func (r *sqlBoardRepository) InsertItem(ctx context.Context, it Item) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO quest_board_items (`+itemColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, it.BoardID, it.CampaignID, it.Kind, it.X, it.Y, it.W, it.R,
		nullable(it.OwnerUserID), it.ByDM, it.Hidden, nullable(it.Text), nullable(it.RefID), nullable(it.FromID), nullable(it.ToID))
	if err != nil {
		return fmt.Errorf("insert item: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) UpdateItem(ctx context.Context, it Item) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE quest_board_items SET x = ?, y = ?, w = ?, r = ?, text = ?, hidden = ?
		 WHERE id = ? AND board_id = ? AND campaign_id = ?`,
		it.X, it.Y, it.W, it.R, nullable(it.Text), it.Hidden, it.ID, it.BoardID, it.CampaignID)
	if err != nil {
		return fmt.Errorf("update item: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) DeleteItems(ctx context.Context, campaignID, boardID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	args := []any{boardID, campaignID}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM quest_board_items WHERE board_id = ? AND campaign_id = ? AND id IN (`+
			strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return fmt.Errorf("delete items: %w", err)
	}
	return nil
}
