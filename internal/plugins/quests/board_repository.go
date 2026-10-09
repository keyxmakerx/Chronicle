// Hand-written SQL for the notice-board tables.
//
// SECURITY: every statement filters on campaign_id, and board and item
// lookups also on the owning page / board, so an id from another campaign,
// page or board is simply not found (no cross-tenant or cross-page access).
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
	GetLooks(ctx context.Context, campaignID, entityID string) (Looks, error)
	SetLooks(ctx context.Context, campaignID, entityID string, l Looks) error

	ListBoards(ctx context.Context, campaignID, entityID string) ([]Board, error)
	// GetBoard returns NotFound unless the board belongs to the page and campaign.
	GetBoard(ctx context.Context, campaignID, entityID, boardID string) (*Board, error)
	InsertBoard(ctx context.Context, b Board) error
	UpdateBoard(ctx context.Context, b Board) error
	DeleteBoard(ctx context.Context, campaignID, entityID, boardID string) error
	// SetOrder writes sort_order by position, atomically.
	SetOrder(ctx context.Context, campaignID, entityID string, ids []string) error

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

func (r *sqlBoardRepository) GetLooks(ctx context.Context, campaignID, entityID string) (Looks, error) {
	l := Looks{Board: LookLit, Ledger: LookLit}
	err := r.db.QueryRowContext(ctx,
		`SELECT board_look, ledger_look FROM quest_board_pages WHERE entity_id = ? AND campaign_id = ?`,
		entityID, campaignID).Scan(&l.Board, &l.Ledger)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Looks{}, fmt.Errorf("get board looks: %w", err)
	}
	return l, nil
}

func (r *sqlBoardRepository) SetLooks(ctx context.Context, campaignID, entityID string, l Looks) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO quest_board_pages (entity_id, campaign_id, board_look, ledger_look) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE board_look = VALUES(board_look), ledger_look = VALUES(ledger_look)`,
		entityID, campaignID, l.Board, l.Ledger)
	if err != nil {
		return fmt.Errorf("set board looks: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) ListBoards(ctx context.Context, campaignID, entityID string) ([]Board, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, campaign_id, entity_id, name, who, sort_order FROM quest_boards
		 WHERE entity_id = ? AND campaign_id = ? ORDER BY sort_order, created_at, id`, entityID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list boards: %w", err)
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.CampaignID, &b.EntityID, &b.Name, &b.Who, &b.SortOrder); err != nil {
			return nil, fmt.Errorf("scan board: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *sqlBoardRepository) GetBoard(ctx context.Context, campaignID, entityID, boardID string) (*Board, error) {
	var b Board
	err := r.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, entity_id, name, who, sort_order FROM quest_boards
		 WHERE id = ? AND entity_id = ? AND campaign_id = ?`, boardID, entityID, campaignID).
		Scan(&b.ID, &b.CampaignID, &b.EntityID, &b.Name, &b.Who, &b.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound("board")
	}
	if err != nil {
		return nil, fmt.Errorf("get board: %w", err)
	}
	return &b, nil
}

func (r *sqlBoardRepository) InsertBoard(ctx context.Context, b Board) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO quest_boards (id, campaign_id, entity_id, name, who, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.CampaignID, b.EntityID, b.Name, b.Who, b.SortOrder)
	if err != nil {
		return fmt.Errorf("insert board: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) UpdateBoard(ctx context.Context, b Board) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE quest_boards SET name = ?, who = ? WHERE id = ? AND entity_id = ? AND campaign_id = ?`,
		b.Name, b.Who, b.ID, b.EntityID, b.CampaignID)
	if err != nil {
		return fmt.Errorf("update board: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) DeleteBoard(ctx context.Context, campaignID, entityID, boardID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM quest_boards WHERE id = ? AND entity_id = ? AND campaign_id = ?`, boardID, entityID, campaignID)
	if err != nil {
		return fmt.Errorf("delete board: %w", err)
	}
	return nil
}

func (r *sqlBoardRepository) SetOrder(ctx context.Context, campaignID, entityID string, ids []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("order boards: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE quest_boards SET sort_order = ? WHERE id = ? AND entity_id = ? AND campaign_id = ?`,
			i, id, entityID, campaignID); err != nil {
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
