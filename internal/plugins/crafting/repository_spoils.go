package crafting

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SpoilStore keeps drop lists and the spoils tray. Every statement filters
// on campaign_id.
type SpoilStore interface {
	GetDrops(ctx context.Context, campaignID, pageID string) (DropList, bool, error)
	// ListDrops lists drop lists, newest change first, up to limit.
	ListDrops(ctx context.Context, campaignID string, limit int) ([]DropList, error)
	// DropsFor returns the drop lists the given pages have, by page.
	DropsFor(ctx context.Context, campaignID string, pageIDs []string) (map[string]DropList, error)
	// SaveDrops writes a page's drop list: expected 0 makes a new one, any
	// other value replaces the stored list at that version. ok is false on
	// a lost race or when a new list's page already has one.
	SaveDrops(ctx context.Context, campaignID string, d DropList, by string, expected int) (ok bool, err error)
	DeleteDrops(ctx context.Context, campaignID, pageID string) error

	// AddSpoils stores spoils and their lines in one transaction.
	AddSpoils(ctx context.Context, campaignID string, spoils []Spoil) error
	CountWaiting(ctx context.Context, campaignID string) (int, error)
	// ListTray lists the spoils with waiting lines, oldest first, each
	// holding only its waiting lines.
	ListTray(ctx context.Context, campaignID string) ([]Spoil, error)
	GetLine(ctx context.Context, campaignID string, id int64) (SpoilLine, bool, error)
	// ClaimLine marks a waiting line taken for a hero; ok is false when the
	// line was no longer waiting.
	ClaimLine(ctx context.Context, campaignID string, id int64, characterID, by string, qty int, rolled *int, at time.Time) (ok bool, err error)
	// ReleaseLine puts a claimed line back in the tray, after its item
	// failed to reach the pack.
	ReleaseLine(ctx context.Context, campaignID string, id int64) error
	// ClearLine, ClearSpoil and ClearTray clear waiting lines and say how
	// many.
	ClearLine(ctx context.Context, campaignID string, id int64) (int, error)
	ClearSpoil(ctx context.Context, campaignID, spoilID string) (int, error)
	ClearTray(ctx context.Context, campaignID string) (int, error)
	// ListTaken lists lines taken lately, newest first.
	ListTaken(ctx context.Context, campaignID string, limit int) ([]SpoilLine, error)
}

// ---------- Drop lists ----------

func scanDrops(s rowScanner) (DropList, error) {
	var (
		d     DropList
		lines string
	)
	if err := s.Scan(&d.PageID, &d.Also, &lines, &d.Version); err != nil {
		return d, err
	}
	_ = json.Unmarshal([]byte(lines), &d.Lines)
	if d.Lines == nil {
		d.Lines = []DropLine{}
	}
	return d, nil
}

const dropColumns = `page_id, COALESCE(also_page_id, ''), drop_lines, version`

func (r *sqlRepository) GetDrops(ctx context.Context, campaignID, pageID string) (DropList, bool, error) {
	d, err := scanDrops(r.db.QueryRowContext(ctx,
		`SELECT `+dropColumns+` FROM crafting_drops WHERE campaign_id = ? AND page_id = ?`, campaignID, pageID))
	if errors.Is(err, sql.ErrNoRows) {
		return DropList{}, false, nil
	}
	if err != nil {
		return DropList{}, false, fmt.Errorf("get crafting drops: %w", err)
	}
	return d, true, nil
}

func (r *sqlRepository) ListDrops(ctx context.Context, campaignID string, limit int) ([]DropList, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+dropColumns+` FROM crafting_drops WHERE campaign_id = ? ORDER BY updated_at DESC, page_id LIMIT ?`, campaignID, limit)
	if err != nil {
		return nil, fmt.Errorf("list crafting drops: %w", err)
	}
	defer rows.Close()
	out := []DropList{}
	for rows.Next() {
		d, err := scanDrops(rows)
		if err != nil {
			return nil, fmt.Errorf("scan crafting drops: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list crafting drops: %w", err)
	}
	return out, nil
}

func (r *sqlRepository) DropsFor(ctx context.Context, campaignID string, pageIDs []string) (map[string]DropList, error) {
	out := map[string]DropList{}
	if len(pageIDs) == 0 {
		return out, nil
	}
	args := []any{campaignID}
	for _, id := range pageIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+dropColumns+` FROM crafting_drops WHERE campaign_id = ? AND page_id IN (?`+strings.Repeat(", ?", len(pageIDs)-1)+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("crafting drops for pages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		d, err := scanDrops(rows)
		if err != nil {
			return nil, fmt.Errorf("scan crafting drops: %w", err)
		}
		out[d.PageID] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("crafting drops for pages: %w", err)
	}
	return out, nil
}

func (r *sqlRepository) SaveDrops(ctx context.Context, campaignID string, d DropList, by string, expected int) (bool, error) {
	if expected == 0 {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO crafting_drops (campaign_id, page_id, also_page_id, drop_lines, updated_by) VALUES (?, ?, ?, ?, ?)`,
			campaignID, d.PageID, nullString(d.Also), mustJSON(d.Lines), nullString(by))
		if isDuplicate(err) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("create crafting drops: %w", err)
		}
		return true, nil
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE crafting_drops SET also_page_id = ?, drop_lines = ?, updated_by = ?, updated_at = CURRENT_TIMESTAMP, version = version + 1
		 WHERE campaign_id = ? AND page_id = ? AND version = ?`,
		nullString(d.Also), mustJSON(d.Lines), nullString(by), campaignID, d.PageID, expected)
	if err != nil {
		return false, fmt.Errorf("update crafting drops: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update crafting drops: %w", err)
	}
	return n == 1, nil
}

func (r *sqlRepository) DeleteDrops(ctx context.Context, campaignID, pageID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM crafting_drops WHERE campaign_id = ? AND page_id = ?`, campaignID, pageID); err != nil {
		return fmt.Errorf("delete crafting drops: %w", err)
	}
	return nil
}

// ---------- The tray ----------

func (r *sqlRepository) AddSpoils(ctx context.Context, campaignID string, spoils []Spoil) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("add crafting spoils: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, s := range spoils {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO crafting_spoils (id, campaign_id, page_id, page_name, copy_no, copies, origin, created_by, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, campaignID, nullString(s.PageID), clipText(s.PageName, maxPageName), s.Copy, s.Copies,
			clipText(s.Origin, maxOriginLen), nullString(s.CreatedBy), s.CreatedAt); err != nil {
			return fmt.Errorf("add crafting spoil: %w", err)
		}
		for _, l := range s.Lines {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO crafting_spoil_lines (spoil_id, campaign_id, item_id, item_name, mode, qty, stat, yield_low, yield_mid, yield_high)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				s.ID, campaignID, l.ItemID, clipText(l.ItemName, maxPageName), l.Mode, l.Qty, l.Stat, l.Yields[0], l.Yields[1], l.Yields[2]); err != nil {
				return fmt.Errorf("add crafting spoil line: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("add crafting spoils: %w", err)
	}
	return nil
}

func (r *sqlRepository) CountWaiting(ctx context.Context, campaignID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM crafting_spoil_lines WHERE campaign_id = ? AND status = ?`, campaignID, SpoilWaiting).Scan(&n); err != nil {
		return 0, fmt.Errorf("count crafting spoils: %w", err)
	}
	return n, nil
}

const lineColumns = `l.id, l.spoil_id, l.item_id, l.item_name, l.mode, l.qty, l.stat, l.yield_low, l.yield_mid, l.yield_high,
	l.status, COALESCE(l.character_id, ''), COALESCE(l.taken_by, ''), l.taken_qty, l.rolled, l.taken_at`

func scanLine(s rowScanner, extra ...any) (SpoilLine, error) {
	var (
		l      SpoilLine
		rolled sql.NullInt64
		at     sql.NullTime
	)
	dest := append([]any{&l.ID, &l.SpoilID, &l.ItemID, &l.ItemName, &l.Mode, &l.Qty, &l.Stat, &l.Yields[0], &l.Yields[1], &l.Yields[2],
		&l.Status, &l.CharacterID, &l.TakenBy, &l.TakenQty, &rolled, &at}, extra...)
	if err := s.Scan(dest...); err != nil {
		return l, err
	}
	if rolled.Valid {
		n := int(rolled.Int64)
		l.Rolled = &n
	}
	if at.Valid {
		t := at.Time
		l.TakenAt = &t
	}
	return l, nil
}

func (r *sqlRepository) ListTray(ctx context.Context, campaignID string) ([]Spoil, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+lineColumns+`, s.campaign_id, COALESCE(s.page_id, ''), s.page_name, s.copy_no, s.copies, s.origin, COALESCE(s.created_by, ''), s.created_at
		 FROM crafting_spoil_lines l JOIN crafting_spoils s ON s.id = l.spoil_id AND s.campaign_id = l.campaign_id
		 WHERE l.campaign_id = ? AND l.status = ?
		 ORDER BY s.created_at, s.copy_no, s.id, l.id LIMIT ?`, campaignID, SpoilWaiting, maxTrayLines)
	if err != nil {
		return nil, fmt.Errorf("list crafting tray: %w", err)
	}
	defer rows.Close()
	out := []Spoil{}
	for rows.Next() {
		var s Spoil
		l, err := scanLine(rows, &s.CampaignID, &s.PageID, &s.PageName, &s.Copy, &s.Copies, &s.Origin, &s.CreatedBy, &s.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan crafting tray: %w", err)
		}
		if n := len(out); n > 0 && out[n-1].ID == l.SpoilID {
			out[n-1].Lines = append(out[n-1].Lines, l)
			continue
		}
		s.ID, s.Lines = l.SpoilID, []SpoilLine{l}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list crafting tray: %w", err)
	}
	return out, nil
}

func (r *sqlRepository) GetLine(ctx context.Context, campaignID string, id int64) (SpoilLine, bool, error) {
	l, err := scanLine(r.db.QueryRowContext(ctx,
		`SELECT `+lineColumns+` FROM crafting_spoil_lines l WHERE l.campaign_id = ? AND l.id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return SpoilLine{}, false, nil
	}
	if err != nil {
		return SpoilLine{}, false, fmt.Errorf("get crafting spoil line: %w", err)
	}
	return l, true, nil
}

func (r *sqlRepository) ClaimLine(ctx context.Context, campaignID string, id int64, characterID, by string, qty int, rolled *int, at time.Time) (bool, error) {
	var roll any
	if rolled != nil {
		roll = *rolled
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE crafting_spoil_lines SET status = ?, character_id = ?, taken_by = ?, taken_qty = ?, rolled = ?, taken_at = ?
		 WHERE campaign_id = ? AND id = ? AND status = ?`,
		SpoilTaken, characterID, nullString(by), qty, roll, at, campaignID, id, SpoilWaiting)
	if err != nil {
		return false, fmt.Errorf("claim crafting spoil line: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim crafting spoil line: %w", err)
	}
	return n == 1, nil
}

func (r *sqlRepository) ReleaseLine(ctx context.Context, campaignID string, id int64) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE crafting_spoil_lines SET status = ?, character_id = NULL, taken_by = NULL, taken_qty = 0, rolled = NULL, taken_at = NULL
		 WHERE campaign_id = ? AND id = ? AND status = ?`,
		SpoilWaiting, campaignID, id, SpoilTaken); err != nil {
		return fmt.Errorf("release crafting spoil line: %w", err)
	}
	return nil
}

func (r *sqlRepository) clear(ctx context.Context, where string, args ...any) (int, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE crafting_spoil_lines SET status = ? WHERE status = ? AND `+where, append([]any{SpoilCleared, SpoilWaiting}, args...)...)
	if err != nil {
		return 0, fmt.Errorf("clear crafting spoils: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("clear crafting spoils: %w", err)
	}
	return int(n), nil
}

func (r *sqlRepository) ClearLine(ctx context.Context, campaignID string, id int64) (int, error) {
	return r.clear(ctx, `campaign_id = ? AND id = ?`, campaignID, id)
}

func (r *sqlRepository) ClearSpoil(ctx context.Context, campaignID, spoilID string) (int, error) {
	return r.clear(ctx, `campaign_id = ? AND spoil_id = ?`, campaignID, spoilID)
}

func (r *sqlRepository) ClearTray(ctx context.Context, campaignID string) (int, error) {
	return r.clear(ctx, `campaign_id = ?`, campaignID)
}

func (r *sqlRepository) ListTaken(ctx context.Context, campaignID string, limit int) ([]SpoilLine, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+lineColumns+` FROM crafting_spoil_lines l WHERE l.campaign_id = ? AND l.status = ?
		 ORDER BY l.taken_at DESC, l.id DESC LIMIT ?`, campaignID, SpoilTaken, limit)
	if err != nil {
		return nil, fmt.Errorf("list crafting spoils taken: %w", err)
	}
	defer rows.Close()
	out := []SpoilLine{}
	for rows.Next() {
		l, err := scanLine(rows)
		if err != nil {
			return nil, fmt.Errorf("scan crafting spoils taken: %w", err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list crafting spoils taken: %w", err)
	}
	return out, nil
}
