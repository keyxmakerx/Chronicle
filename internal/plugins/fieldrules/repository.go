package fieldrules

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/go-sql-driver/mysql"
)

// errDuplicate is returned when a name (for a rule) or a word in one place or
// era (for a local word) is already taken; the service words it.
var errDuplicate = errors.New("already exists")

// Repository owns the field_rules and local_words tables. It decides nothing
// about who may do what.
type Repository interface {
	ListRules(ctx context.Context, campaignID string) ([]Rule, error)
	GetRule(ctx context.Context, campaignID string, id int) (*Rule, error)
	CountRules(ctx context.Context, campaignID string) (int, error)
	InsertRule(ctx context.Context, campaignID string, r Rule) (int, error)
	UpdateRule(ctx context.Context, campaignID string, r Rule) (bool, error)
	DeleteRule(ctx context.Context, campaignID string, id int) (bool, error)

	ListWords(ctx context.Context, campaignID string) ([]LocalWord, error)
	CountWords(ctx context.Context, campaignID string) (int, error)
	InsertWord(ctx context.Context, campaignID string, w LocalWord) (int, error)
	UpdateWord(ctx context.Context, campaignID string, w LocalWord) (bool, error)
	DeleteWord(ctx context.Context, campaignID string, id int) (bool, error)
}

type repository struct{ db *sql.DB }

// NewRepository creates the repository.
func NewRepository(db *sql.DB) Repository { return &repository{db: db} }

const ruleColumns = `id, name, follows, rows_json, updated_at`

func scanRule(row interface{ Scan(...any) error }) (*Rule, error) {
	var r Rule
	var raw []byte
	if err := row.Scan(&r.ID, &r.Name, &r.Follows, &raw, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &r.Rows); err != nil {
		return nil, fmt.Errorf("reading rule %d rows: %w", r.ID, err)
	}
	return &r, nil
}

func (s *repository) ListRules(ctx context.Context, campaignID string) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+ruleColumns+` FROM field_rules WHERE campaign_id = ? ORDER BY name, id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing rules: %w", err)
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		r.CampaignID = campaignID
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *repository) GetRule(ctx context.Context, campaignID string, id int) (*Rule, error) {
	r, err := scanRule(s.db.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM field_rules WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading rule: %w", err)
	}
	r.CampaignID = campaignID
	return r, nil
}

func (s *repository) CountRules(ctx context.Context, campaignID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM field_rules WHERE campaign_id = ?`, campaignID).Scan(&n)
	return n, err
}

func (s *repository) InsertRule(ctx context.Context, campaignID string, r Rule) (int, error) {
	raw, err := json.Marshal(r.Rows)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO field_rules (campaign_id, name, follows, rows_json) VALUES (?, ?, ?, ?)`,
		campaignID, r.Name, r.Follows, raw)
	if err != nil {
		return 0, dupOr(err, "adding rule")
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (s *repository) UpdateRule(ctx context.Context, campaignID string, r Rule) (bool, error) {
	raw, err := json.Marshal(r.Rows)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE field_rules SET name = ?, follows = ?, rows_json = ? WHERE campaign_id = ? AND id = ?`,
		r.Name, r.Follows, raw, campaignID, r.ID)
	if err != nil {
		return false, dupOr(err, "changing rule")
	}
	return affected(res)
}

func (s *repository) DeleteRule(ctx context.Context, campaignID string, id int) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM field_rules WHERE campaign_id = ? AND id = ?`, campaignID, id)
	if err != nil {
		return false, fmt.Errorf("removing rule: %w", err)
	}
	return affected(res)
}

func (s *repository) ListWords(ctx context.Context, campaignID string) ([]LocalWord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT w.id, w.word, w.local_word, COALESCE(w.place_id, ''), COALESCE(e.name, ''), COALESCE(w.era_id, 0), w.dm_only
		FROM local_words w LEFT JOIN entities e ON e.id = w.place_id
		WHERE w.campaign_id = ? ORDER BY w.word, w.id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing local words: %w", err)
	}
	defer rows.Close()
	var out []LocalWord
	for rows.Next() {
		var w LocalWord
		if err := rows.Scan(&w.ID, &w.Word, &w.Local, &w.PlaceID, &w.PlaceName, &w.EraID, &w.DMOnly); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *repository) CountWords(ctx context.Context, campaignID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_words WHERE campaign_id = ?`, campaignID).Scan(&n)
	return n, err
}

// scopeKey is the one column the unique key reads to allow one local name
// per word per place or era.
func scopeKey(w LocalWord) string {
	if w.PlaceID != "" {
		return "p:" + w.PlaceID
	}
	return "e:" + strconv.Itoa(w.EraID)
}

func nullable(w LocalWord) (any, any) {
	var place, era any
	if w.PlaceID != "" {
		place = w.PlaceID
	}
	if w.EraID != 0 {
		era = w.EraID
	}
	return place, era
}

func (s *repository) InsertWord(ctx context.Context, campaignID string, w LocalWord) (int, error) {
	place, era := nullable(w)
	res, err := s.db.ExecContext(ctx, `INSERT INTO local_words (campaign_id, word, local_word, place_id, era_id, scope_key, dm_only) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		campaignID, w.Word, w.Local, place, era, scopeKey(w), w.DMOnly)
	if err != nil {
		return 0, dupOr(err, "adding local word")
	}
	id, err := res.LastInsertId()
	return int(id), err
}

func (s *repository) UpdateWord(ctx context.Context, campaignID string, w LocalWord) (bool, error) {
	place, era := nullable(w)
	res, err := s.db.ExecContext(ctx, `UPDATE local_words SET word = ?, local_word = ?, place_id = ?, era_id = ?, scope_key = ?, dm_only = ? WHERE campaign_id = ? AND id = ?`,
		w.Word, w.Local, place, era, scopeKey(w), w.DMOnly, campaignID, w.ID)
	if err != nil {
		return false, dupOr(err, "changing local word")
	}
	return affected(res)
}

func (s *repository) DeleteWord(ctx context.Context, campaignID string, id int) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM local_words WHERE campaign_id = ? AND id = ?`, campaignID, id)
	if err != nil {
		return false, fmt.Errorf("removing local word: %w", err)
	}
	return affected(res)
}

func affected(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	return n > 0, err
}

// dupOr turns MariaDB's duplicate-key error into errDuplicate.
func dupOr(err error, doing string) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return errDuplicate
	}
	return fmt.Errorf("%s: %w", doing, err)
}
