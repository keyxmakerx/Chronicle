package playerchanges

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// liveScan is the most live rows one search reads before filtering; a
// month of one campaign's player changes stays well under it.
const liveScan = 5000

// Repository owns player_changes and player_change_packs. It decides nothing
// about who may see or undo a change; the service does.
type Repository interface {
	Insert(ctx context.Context, c Change) error
	// LastFor returns the newest live, not undone change by the same person
	// to the same page, kind, field and source made at or after since, or
	// nil. Record folds quick successive saves into it.
	LastFor(ctx context.Context, c Change, since time.Time) (*Change, error)
	// UpdateNew rewrites a live change's page name, texts, new value and
	// time after a fold.
	UpdateNew(ctx context.Context, c Change) error
	// Delete removes a live change whose fold came back to where it started.
	Delete(ctx context.Context, campaignID string, id int64) error
	// Live returns the campaign's live changes since q.Since that match the
	// query's page, AI and removed parts, newest first, at most liveScan.
	Live(ctx context.Context, campaignID string, q Query) ([]Change, error)
	// Get returns one live change of the campaign, or nil.
	Get(ctx context.Context, campaignID string, id int64) (*Change, error)
	// MarkUndone records that the owner undid a live change; false when it
	// was already undone or is not there.
	MarkUndone(ctx context.Context, campaignID string, id int64, by string, at time.Time) (bool, error)
	// Recent lists the pages and fields the campaign's live changes touched,
	// for the search box's suggestions.
	Recent(ctx context.Context, campaignID string, since time.Time) (pages, fields []Suggestion, err error)

	// DueMonths lists the campaign months that hold live rows older than
	// cutoff.
	DueMonths(ctx context.Context, cutoff time.Time) ([]PackKey, error)
	// PackMonth moves the month's live rows older than cutoff into its pack,
	// keeping the pack until keepUntil (or longer, if it already was).
	// keepUntil zero drops them instead.
	PackMonth(ctx context.Context, k PackKey, cutoff time.Time, keepUntil time.Time) (int, error)
	// Packs returns the campaign's packed months from since on.
	Packs(ctx context.Context, campaignID string, since time.Time) ([]Pack, error)
	// RewritePack replaces one packed month's rows (after an Undo in it).
	RewritePack(ctx context.Context, k PackKey, rows []Change) error
	// PrunePacks removes packs, and kept pages, whose keep_until has passed.
	PrunePacks(ctx context.Context, today time.Time) (int, error)
	// KeptPages lists the pages players removed that the Trash must keep:
	// every live, not undone removal, and every kept page whose keep_until
	// is today or later.
	KeptPages(ctx context.Context, today time.Time) ([]string, error)
	// MonthlyBytes is about how much one month of packed changes takes on
	// the site: the packs' average, or the live rows' size shrunk by what
	// packing usually saves. known is false with nothing to go by.
	MonthlyBytes(ctx context.Context) (bytes int64, known bool, err error)
	// ForgetPage stops keeping a removed page (it was brought back).
	ForgetPage(ctx context.Context, campaignID, entityID string) error
}

// PackKey names one packed month.
type PackKey struct {
	CampaignID string
	Month      time.Time // the first of the month, UTC
}

// Pack is one packed month, unpacked.
type Pack struct {
	PackKey
	Rows      []Change
	KeepUntil time.Time
}

type repository struct{ db *sql.DB }

// NewRepository creates the MariaDB repository.
func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

const changeColumns = `id, campaign_id, entity_id, page_name, user_id, user_name, kind, field_key, field_label,
	COALESCE(old_text, ''), COALESCE(new_text, ''), COALESCE(old_json, ''), COALESCE(new_json, ''),
	source, app, created_at, undone_at, COALESCE(undone_by, '')`

func scanChange(s interface{ Scan(...any) error }) (Change, error) {
	var c Change
	var undone sql.NullTime
	err := s.Scan(&c.ID, &c.CampaignID, &c.EntityID, &c.PageName, &c.UserID, &c.UserName, &c.Kind, &c.FieldKey, &c.FieldLabel,
		&c.OldText, &c.NewText, &c.OldJSON, &c.NewJSON, &c.Source, &c.App, &c.CreatedAt, &undone, &c.UndoneBy)
	if undone.Valid {
		t := undone.Time
		c.UndoneAt = &t
	}
	return c, err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *repository) Insert(ctx context.Context, c Change) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO player_changes
		(campaign_id, entity_id, page_name, user_id, user_name, kind, field_key, field_label,
		 old_text, new_text, old_json, new_json, source, app, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.CampaignID, c.EntityID, clip(c.PageName, 200), c.UserID, clip(c.UserName, 200), c.Kind,
		clip(c.FieldKey, 100), clip(c.FieldLabel, 200), nullable(c.OldText), nullable(c.NewText),
		nullable(c.OldJSON), nullable(c.NewJSON), c.Source, clip(c.App, 80), c.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("recording a player change: %w", err)
	}
	return nil
}

func (r *repository) LastFor(ctx context.Context, c Change, since time.Time) (*Change, error) {
	p, err := scanChange(r.db.QueryRowContext(ctx, `SELECT `+changeColumns+` FROM player_changes
		WHERE campaign_id = ? AND entity_id = ? AND created_at >= ? AND user_id = ? AND kind = ?
		  AND field_key = ? AND source = ? AND undone_at IS NULL
		ORDER BY created_at DESC, id DESC LIMIT 1`,
		c.CampaignID, c.EntityID, since.UTC(), c.UserID, c.Kind, clip(c.FieldKey, 100), c.Source))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the last player change: %w", err)
	}
	return &p, nil
}

func (r *repository) UpdateNew(ctx context.Context, c Change) error {
	_, err := r.db.ExecContext(ctx, `UPDATE player_changes
		SET page_name = ?, old_text = ?, new_text = ?, old_json = ?, new_json = ?, app = ?, created_at = ?
		WHERE campaign_id = ? AND id = ? AND undone_at IS NULL`,
		clip(c.PageName, 200), nullable(c.OldText), nullable(c.NewText), nullable(c.OldJSON), nullable(c.NewJSON),
		clip(c.App, 80), c.CreatedAt.UTC(), c.CampaignID, c.ID)
	if err != nil {
		return fmt.Errorf("updating a player change: %w", err)
	}
	return nil
}

func (r *repository) Delete(ctx context.Context, campaignID string, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM player_changes WHERE campaign_id = ? AND id = ?`, campaignID, id); err != nil {
		return fmt.Errorf("removing a player change: %w", err)
	}
	return nil
}

func (r *repository) Live(ctx context.Context, campaignID string, q Query) ([]Change, error) {
	where := []string{"campaign_id = ?", "created_at >= ?"}
	args := []any{campaignID, q.Since.UTC()}
	if len(q.Pages) > 0 {
		where = append(where, "entity_id IN (?"+strings.Repeat(", ?", len(q.Pages)-1)+")")
		for _, p := range q.Pages {
			args = append(args, p)
		}
	}
	if q.AI {
		where = append(where, "source = ?")
		args = append(args, SourceAI)
	}
	if q.Removed {
		where = append(where, "kind = ?")
		args = append(args, KindRemoved)
	}
	args = append(args, liveScan)
	rows, err := r.db.QueryContext(ctx, `SELECT `+changeColumns+` FROM player_changes
		WHERE `+strings.Join(where, " AND ")+` ORDER BY created_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("reading player changes: %w", err)
	}
	defer rows.Close()
	var out []Change
	for rows.Next() {
		c, err := scanChange(rows)
		if err != nil {
			return nil, fmt.Errorf("reading player changes: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *repository) Get(ctx context.Context, campaignID string, id int64) (*Change, error) {
	c, err := scanChange(r.db.QueryRowContext(ctx, `SELECT `+changeColumns+` FROM player_changes WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading a player change: %w", err)
	}
	return &c, nil
}

func (r *repository) MarkUndone(ctx context.Context, campaignID string, id int64, by string, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE player_changes SET undone_at = ?, undone_by = ?
		WHERE campaign_id = ? AND id = ? AND undone_at IS NULL`, at.UTC(), by, campaignID, id)
	if err != nil {
		return false, fmt.Errorf("marking a change undone: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *repository) Recent(ctx context.Context, campaignID string, since time.Time) ([]Suggestion, []Suggestion, error) {
	var pages, fields []Suggestion
	rows, err := r.db.QueryContext(ctx, `SELECT entity_id, MAX(page_name) FROM player_changes
		WHERE campaign_id = ? AND created_at >= ? GROUP BY entity_id ORDER BY MAX(created_at) DESC LIMIT 40`, campaignID, since.UTC())
	if err != nil {
		return nil, nil, fmt.Errorf("reading changed pages: %w", err)
	}
	for rows.Next() {
		var s Suggestion
		if err := rows.Scan(&s.Value, &s.Label); err != nil {
			rows.Close()
			return nil, nil, err
		}
		s.Kind = "page"
		pages = append(pages, s)
	}
	rows.Close()
	rows, err = r.db.QueryContext(ctx, `SELECT kind, field_key, MAX(field_label) FROM player_changes
		WHERE campaign_id = ? AND created_at >= ? AND kind <> ? GROUP BY kind, field_key ORDER BY COUNT(*) DESC LIMIT 40`,
		campaignID, since.UTC(), KindRemoved)
	if err != nil {
		return nil, nil, fmt.Errorf("reading changed fields: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Change
		if err := rows.Scan(&c.Kind, &c.FieldKey, &c.FieldLabel); err != nil {
			return nil, nil, err
		}
		fields = append(fields, Suggestion{Kind: "field", Value: fieldToken(c), Label: c.What()})
	}
	return pages, fields, rows.Err()
}

func (r *repository) DueMonths(ctx context.Context, cutoff time.Time) ([]PackKey, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT campaign_id, DATE_FORMAT(created_at, '%Y-%m-01')
		FROM player_changes WHERE created_at < ?`, cutoff.UTC())
	if err != nil {
		return nil, fmt.Errorf("finding months to pack: %w", err)
	}
	defer rows.Close()
	var out []PackKey
	for rows.Next() {
		var k PackKey
		var month string
		if err := rows.Scan(&k.CampaignID, &month); err != nil {
			return nil, err
		}
		if k.Month, err = time.Parse("2006-01-02", month); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *repository) PackMonth(ctx context.Context, k PackKey, cutoff, keepUntil time.Time) (n int, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	end := k.Month.AddDate(0, 1, 0)
	if cutoff.Before(end) {
		end = cutoff
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+changeColumns+` FROM player_changes
		WHERE campaign_id = ? AND created_at >= ? AND created_at < ? ORDER BY id FOR UPDATE`, k.CampaignID, k.Month, end.UTC())
	if err != nil {
		return 0, fmt.Errorf("reading a month to pack: %w", err)
	}
	var fresh []Change
	for rows.Next() {
		c, err := scanChange(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		fresh = append(fresh, c)
	}
	rows.Close()
	if len(fresh) == 0 {
		return 0, tx.Commit()
	}
	if !keepUntil.IsZero() {
		var body []byte
		var kept time.Time
		err = tx.QueryRowContext(ctx, `SELECT body, keep_until FROM player_change_packs WHERE campaign_id = ? AND month = ? FOR UPDATE`,
			k.CampaignID, k.Month).Scan(&body, &kept)
		var all []Change
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return 0, fmt.Errorf("reading a pack: %w", err)
		default:
			if all, err = unpack(body); err != nil {
				return 0, err
			}
			if kept.After(keepUntil) {
				keepUntil = kept
			}
		}
		all = append(all, fresh...)
		if body, err = pack(all); err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO player_change_packs (campaign_id, month, row_count, body, keep_until, packed_at)
			VALUES (?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE row_count = VALUES(row_count), body = VALUES(body),
			keep_until = VALUES(keep_until), packed_at = VALUES(packed_at)`,
			k.CampaignID, k.Month, len(all), body, keepUntil.Format("2006-01-02"), time.Now().UTC()); err != nil {
			return 0, fmt.Errorf("writing a pack: %w", err)
		}
	}
	if !keepUntil.IsZero() {
		// Pages removed this month stay whole in the Trash as long as the pack.
		for _, c := range fresh {
			if c.Kind != KindRemoved || c.UndoneAt != nil {
				continue
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO player_change_kept_pages (campaign_id, entity_id, keep_until)
				VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE keep_until = GREATEST(keep_until, VALUES(keep_until))`,
				k.CampaignID, c.EntityID, keepUntil.Format("2006-01-02")); err != nil {
				return 0, fmt.Errorf("keeping a removed page: %w", err)
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM player_changes WHERE campaign_id = ? AND created_at >= ? AND created_at < ? AND id <= ?`,
		k.CampaignID, k.Month, end.UTC(), fresh[len(fresh)-1].ID); err != nil {
		return 0, fmt.Errorf("clearing packed rows: %w", err)
	}
	return len(fresh), tx.Commit()
}

func (r *repository) Packs(ctx context.Context, campaignID string, since time.Time) ([]Pack, error) {
	first := time.Date(since.Year(), since.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows, err := r.db.QueryContext(ctx, `SELECT month, body, keep_until FROM player_change_packs
		WHERE campaign_id = ? AND month >= ? ORDER BY month DESC`, campaignID, first)
	if err != nil {
		return nil, fmt.Errorf("reading packed months: %w", err)
	}
	defer rows.Close()
	var out []Pack
	for rows.Next() {
		p := Pack{PackKey: PackKey{CampaignID: campaignID}}
		var body []byte
		if err := rows.Scan(&p.Month, &body, &p.KeepUntil); err != nil {
			return nil, err
		}
		if p.Rows, err = unpack(body); err != nil {
			return nil, err
		}
		for i := range p.Rows {
			p.Rows[i].CampaignID, p.Rows[i].Packed = campaignID, true
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *repository) RewritePack(ctx context.Context, k PackKey, rows []Change) error {
	body, err := pack(rows)
	if err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE player_change_packs SET body = ?, row_count = ? WHERE campaign_id = ? AND month = ?`,
		body, len(rows), k.CampaignID, k.Month); err != nil {
		return fmt.Errorf("rewriting a pack: %w", err)
	}
	return nil
}

func (r *repository) PrunePacks(ctx context.Context, today time.Time) (int, error) {
	day := today.UTC().Format("2006-01-02")
	res, err := r.db.ExecContext(ctx, `DELETE FROM player_change_packs WHERE keep_until < ?`, day)
	if err != nil {
		return 0, fmt.Errorf("removing old packs: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM player_change_kept_pages WHERE keep_until < ?`, day); err != nil {
		return 0, fmt.Errorf("letting removed pages go: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *repository) KeptPages(ctx context.Context, today time.Time) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT entity_id FROM player_changes WHERE kind = ? AND undone_at IS NULL
		UNION SELECT entity_id FROM player_change_kept_pages WHERE keep_until >= ?`,
		KindRemoved, today.UTC().Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("reading kept pages: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *repository) ForgetPage(ctx context.Context, campaignID, entityID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM player_change_kept_pages WHERE campaign_id = ? AND entity_id = ?`,
		campaignID, entityID); err != nil {
		return fmt.Errorf("forgetting a kept page: %w", err)
	}
	return nil
}

// packRatio is roughly how much gzip shrinks a month of changes, used only
// until a first month is packed.
const packRatio = 4

func (r *repository) MonthlyBytes(ctx context.Context) (int64, bool, error) {
	var months, total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT month), COALESCE(SUM(LENGTH(body)), 0) FROM player_change_packs`).
		Scan(&months, &total); err != nil {
		return 0, false, fmt.Errorf("sizing packs: %w", err)
	}
	if months > 0 {
		return total / months, true, nil
	}
	var rows int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(LENGTH(page_name) + LENGTH(user_name) + LENGTH(field_label)
		+ COALESCE(LENGTH(old_text), 0) + COALESCE(LENGTH(new_text), 0) + COALESCE(LENGTH(old_json), 0) + COALESCE(LENGTH(new_json), 0) + 200), 0)
		FROM player_changes`).Scan(&rows, &total); err != nil {
		return 0, false, fmt.Errorf("sizing changes: %w", err)
	}
	if rows == 0 {
		return 0, false, nil
	}
	return total / packRatio, true, nil
}
