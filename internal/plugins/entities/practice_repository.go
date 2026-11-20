package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// mysqlDeadlock is the server's error number for a transaction it chose to
// roll back to break a lock cycle; the statement is safe to try again.
const mysqlDeadlock = 1213

// PracticeRepository owns the practice_heroes table. Every read and write is
// scoped by campaign id, so an id from another campaign finds nothing. It
// decides nothing about who may act; the service does.
type PracticeRepository interface {
	// Insert saves a new practice hero unless the maker already holds max in
	// the campaign, and reports whether it was saved. The count and the insert
	// are one statement, so two requests cannot both slip past the cap.
	Insert(ctx context.Context, h *PracticeHero, max int) (bool, error)
	// Restore puts a practice hero back without the cap, for a promotion that
	// failed after the row was taken.
	Restore(ctx context.Context, h *PracticeHero) error
	// Find returns one practice hero, or nil when the campaign has none by
	// that id.
	Find(ctx context.Context, campaignID, id string) (*PracticeHero, error)
	// ListByUser returns the maker's practice heroes, oldest first.
	ListByUser(ctx context.Context, campaignID, userID string) ([]PracticeHero, error)
	// ListByCampaign returns every practice hero of the campaign, those
	// waiting on a Director first, then newest first. Bounded.
	ListByCampaign(ctx context.Context, campaignID string) ([]PracticeHero, error)
	// Replace sets the name, picks and level of the maker's own practice hero
	// and clears any ask. False when the maker holds none by that id.
	Replace(ctx context.Context, h *PracticeHero) (bool, error)
	// Delete removes the maker's own practice hero; false when not found.
	Delete(ctx context.Context, campaignID, id, userID string) (bool, error)
	// SetAsk moves the maker's practice hero from not-asked or declined to
	// waiting; false when it is not theirs or already waiting.
	SetAsk(ctx context.Context, campaignID, id, userID string) (bool, error)
	// Decline marks a waiting practice hero declined with the Director's note;
	// false when it was not waiting.
	Decline(ctx context.Context, campaignID, id, note string) (bool, error)
	// TakeWaiting removes a waiting practice hero and reports whether this
	// call took it, so two Directors saying yes cannot both promote it.
	TakeWaiting(ctx context.Context, campaignID, id string) (bool, error)
	// DeleteForUser removes everything the person made in one campaign.
	DeleteForUser(ctx context.Context, campaignID, userID string) (int64, error)
	// DeleteForUserEverywhere removes everything an account made, for an
	// account being deleted.
	DeleteForUserEverywhere(ctx context.Context, userID string) (int64, error)
	// MoveUser hands one campaign's practice heroes to another account, then
	// keeps only the newest keep of the receiver's.
	MoveUser(ctx context.Context, campaignID, fromUserID, toUserID string, keep int) error
}

type practiceRepository struct {
	db *sql.DB
}

// NewPracticeRepository creates the MariaDB repository for practice heroes.
func NewPracticeRepository(db *sql.DB) PracticeRepository {
	return &practiceRepository{db: db}
}

// maxPracticeCampaignRows bounds the Directors' list; a table of any size
// holds three each, so this is far above real use.
const maxPracticeCampaignRows = 500

const practiceSelect = `SELECT id, campaign_id, user_id, name, picks_json, level, COALESCE(copied_from_name, ''),
	ask_status, COALESCE(ask_note, ''), asked_at, created_at, updated_at FROM practice_heroes`

func scanPractice(sc interface{ Scan(...any) error }) (PracticeHero, error) {
	var h PracticeHero
	var raw []byte
	var level sql.NullInt64
	var asked sql.NullTime
	if err := sc.Scan(&h.ID, &h.CampaignID, &h.UserID, &h.Name, &raw, &level, &h.CopiedFromName,
		&h.AskStatus, &h.AskNote, &asked, &h.CreatedAt, &h.UpdatedAt); err != nil {
		return h, err
	}
	if level.Valid {
		n := int(level.Int64)
		h.Level = &n
	}
	if asked.Valid {
		t := asked.Time
		h.AskedAt = &t
	}
	if err := json.Unmarshal(raw, &h.Picks); err != nil {
		return h, fmt.Errorf("reading picks of practice hero %s: %w", h.ID, err)
	}
	if h.Picks == nil {
		h.Picks = map[string]HeroPick{}
	}
	return h, nil
}

func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableLevel(l *int) any {
	if l == nil {
		return nil
	}
	return *l
}

func (r *practiceRepository) Insert(ctx context.Context, h *PracticeHero, max int) (bool, error) {
	raw, err := json.Marshal(h.Picks)
	if err != nil {
		return false, fmt.Errorf("encoding picks: %w", err)
	}
	const q = `INSERT INTO practice_heroes (id, campaign_id, user_id, name, picks_json, level, copied_from_name)
		SELECT ?, ?, ?, ?, ?, ?, ? FROM DUAL
		WHERE (SELECT COUNT(*) FROM practice_heroes WHERE campaign_id = ? AND user_id = ?) < ?`
	args := []any{h.ID, h.CampaignID, h.UserID, h.Name, string(raw), nullableLevel(h.Level), nullableText(h.CopiedFromName),
		h.CampaignID, h.UserID, max}
	// Two requests counting at once can lock each other out; the server
	// rolls one back and the retry then sees the other's row.
	for attempt := 0; ; attempt++ {
		res, err := r.db.ExecContext(ctx, q, args...)
		if err != nil {
			var me *mysql.MySQLError
			if errors.As(err, &me) && me.Number == mysqlDeadlock && attempt < 2 {
				continue
			}
			return false, fmt.Errorf("saving practice hero: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return false, fmt.Errorf("saving practice hero: %w", err)
		}
		return n == 1, nil
	}
}

func (r *practiceRepository) Restore(ctx context.Context, h *PracticeHero) error {
	raw, err := json.Marshal(h.Picks)
	if err != nil {
		return fmt.Errorf("encoding picks: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO practice_heroes (id, campaign_id, user_id, name, picks_json, level, copied_from_name, ask_status, ask_note, asked_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = id`,
		h.ID, h.CampaignID, h.UserID, h.Name, string(raw), nullableLevel(h.Level), nullableText(h.CopiedFromName),
		h.AskStatus, nullableText(h.AskNote), h.AskedAt, h.CreatedAt)
	if err != nil {
		return fmt.Errorf("restoring practice hero: %w", err)
	}
	return nil
}

func (r *practiceRepository) Find(ctx context.Context, campaignID, id string) (*PracticeHero, error) {
	h, err := scanPractice(r.db.QueryRowContext(ctx, practiceSelect+` WHERE id = ? AND campaign_id = ?`, id, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding practice hero: %w", err)
	}
	return &h, nil
}

func (r *practiceRepository) list(ctx context.Context, query string, args ...any) ([]PracticeHero, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing practice heroes: %w", err)
	}
	defer rows.Close()
	var out []PracticeHero
	for rows.Next() {
		h, err := scanPractice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (r *practiceRepository) ListByUser(ctx context.Context, campaignID, userID string) ([]PracticeHero, error) {
	return r.list(ctx, practiceSelect+` WHERE campaign_id = ? AND user_id = ? ORDER BY created_at, id LIMIT ?`,
		campaignID, userID, MaxPracticeHeroes*4)
}

func (r *practiceRepository) ListByCampaign(ctx context.Context, campaignID string) ([]PracticeHero, error) {
	return r.list(ctx, practiceSelect+` WHERE campaign_id = ?
		ORDER BY (ask_status = 'waiting') DESC, created_at DESC, id LIMIT ?`, campaignID, maxPracticeCampaignRows)
}

func (r *practiceRepository) Replace(ctx context.Context, h *PracticeHero) (bool, error) {
	raw, err := json.Marshal(h.Picks)
	if err != nil {
		return false, fmt.Errorf("encoding picks: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE practice_heroes SET name = ?, picks_json = ?, level = ?, ask_status = '', ask_note = NULL, asked_at = NULL,
		 updated_at = CURRENT_TIMESTAMP(3)
		 WHERE id = ? AND campaign_id = ? AND user_id = ?`,
		h.Name, string(raw), nullableLevel(h.Level), h.ID, h.CampaignID, h.UserID)
	changed, err := affected(res, err, "replacing practice hero")
	if err != nil || changed {
		return changed, err
	}
	// The driver counts changed rows, so saving what is already stored reads
	// as zero; the hero is still there.
	var one int
	switch err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM practice_heroes WHERE id = ? AND campaign_id = ? AND user_id = ?`, h.ID, h.CampaignID, h.UserID).Scan(&one); {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("replacing practice hero: %w", err)
	}
	return true, nil
}

func (r *practiceRepository) Delete(ctx context.Context, campaignID, id, userID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM practice_heroes WHERE id = ? AND campaign_id = ? AND user_id = ?`, id, campaignID, userID)
	return affected(res, err, "deleting practice hero")
}

func (r *practiceRepository) SetAsk(ctx context.Context, campaignID, id, userID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE practice_heroes SET ask_status = 'waiting', ask_note = NULL, asked_at = ?
		 WHERE id = ? AND campaign_id = ? AND user_id = ? AND ask_status <> 'waiting'`,
		time.Now().UTC(), id, campaignID, userID)
	return affected(res, err, "asking to play practice hero")
}

func (r *practiceRepository) Decline(ctx context.Context, campaignID, id, note string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE practice_heroes SET ask_status = 'declined', ask_note = ?
		 WHERE id = ? AND campaign_id = ? AND ask_status = 'waiting'`,
		nullableText(note), id, campaignID)
	return affected(res, err, "declining practice hero")
}

func (r *practiceRepository) TakeWaiting(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM practice_heroes WHERE id = ? AND campaign_id = ? AND ask_status = 'waiting'`, id, campaignID)
	return affected(res, err, "taking practice hero")
}

func (r *practiceRepository) DeleteForUser(ctx context.Context, campaignID, userID string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM practice_heroes WHERE campaign_id = ? AND user_id = ?`, campaignID, userID)
	if err != nil {
		return 0, fmt.Errorf("deleting a member's practice heroes: %w", err)
	}
	return res.RowsAffected()
}

func (r *practiceRepository) DeleteForUserEverywhere(ctx context.Context, userID string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM practice_heroes WHERE user_id = ?`, userID)
	if err != nil {
		return 0, fmt.Errorf("deleting an account's practice heroes: %w", err)
	}
	return res.RowsAffected()
}

func (r *practiceRepository) MoveUser(ctx context.Context, campaignID, fromUserID, toUserID string, keep int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting practice hero move: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE practice_heroes SET user_id = ? WHERE campaign_id = ? AND user_id = ?`, toUserID, campaignID, fromUserID); err != nil {
		return fmt.Errorf("moving practice heroes: %w", err)
	}
	// Newest keep stay; the ids are read first because MariaDB will not
	// delete from a table it is also selecting from in a subquery.
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM practice_heroes WHERE campaign_id = ? AND user_id = ? ORDER BY created_at DESC, id DESC`, campaignID, toUserID)
	if err != nil {
		return fmt.Errorf("counting practice heroes: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scanning practice hero: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for i := keep; i < len(ids); i++ {
		if _, err := tx.ExecContext(ctx, `DELETE FROM practice_heroes WHERE id = ?`, ids[i]); err != nil {
			return fmt.Errorf("trimming practice heroes: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing practice hero move: %w", err)
	}
	return nil
}

// affected reports whether a statement changed a row.
func affected(res sql.Result, err error, what string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("%s: %w", what, err)
	}
	return n > 0, nil
}
