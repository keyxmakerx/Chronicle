// Hand-written SQL for tracker kinds, trackers and their log.
//
// SECURITY: every statement filters on campaign_id, so an id from another
// campaign never reads or changes anything here.
package trackers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// errDuplicate is a unique-key clash: a kind or tracker name already taken.
var errDuplicate = errors.New("duplicate name")

// Repository is the persistence boundary for trackers.
type Repository interface {
	ListKinds(ctx context.Context, campaignID string) ([]Kind, error)
	GetKind(ctx context.Context, campaignID, id string) (Kind, bool, error)
	CountKinds(ctx context.Context, campaignID string) (int, error)
	// CreateKind returns errDuplicate when the name is taken.
	CreateKind(ctx context.Context, k Kind) error
	// UpdateKind writes k if its stored version is still expected; ok is
	// false on a lost race. errDuplicate when the new name is taken.
	UpdateKind(ctx context.Context, k Kind, expected int) (ok bool, err error)
	// DeleteKind removes an unused kind; inUse is true (and nothing is
	// removed) while a tracker still uses it.
	DeleteKind(ctx context.Context, campaignID, id string) (inUse bool, err error)

	ListTrackers(ctx context.Context, campaignID, entityID string) ([]Tracker, error)
	GetTracker(ctx context.Context, campaignID, id string) (Tracker, bool, error)
	TrackerForKind(ctx context.Context, campaignID, entityID, kindID string) (Tracker, bool, error)
	// CreateTracker returns errDuplicate when the page already has a
	// tracker of that name.
	CreateTracker(ctx context.Context, t Tracker) error
	// UpdateTracker saves name and visibility; errDuplicate on a name clash.
	UpdateTracker(ctx context.Context, t Tracker) error
	DeleteTracker(ctx context.Context, campaignID, id string) error

	// ListEntries returns each tracker's log, oldest first.
	ListEntries(ctx context.Context, campaignID string, trackerIDs []string) (map[string][]Entry, error)
	CountEntries(ctx context.Context, campaignID, trackerID string) (int, error)
	AddEntry(ctx context.Context, campaignID string, e Entry) error
	GetEntry(ctx context.Context, campaignID string, id int64) (Entry, bool, error)
	// StrikeEntry strikes a line, or restores it when struck is false.
	StrikeEntry(ctx context.Context, campaignID string, id int64, struck bool, userID string) error
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

const kindCols = `id, campaign_id, name, shape, min_value, max_value, steps, bands, visibility, every_type_id, version`

func scanKind(sc interface{ Scan(...any) error }) (Kind, error) {
	var k Kind
	var lo, hi, every sql.NullInt64
	var steps, bands string
	if err := sc.Scan(&k.ID, &k.CampaignID, &k.Name, &k.Shape, &lo, &hi, &steps, &bands, &k.Visibility, &every, &k.Version); err != nil {
		return k, err
	}
	k.Min, k.Max = intPtr(lo), intPtr(hi)
	if every.Valid {
		k.EveryTypeID = int(every.Int64)
	}
	k.Steps, k.Bands = []Step{}, []Band{}
	if err := json.Unmarshal([]byte(steps), &k.Steps); err != nil {
		return k, fmt.Errorf("kind %s steps: %w", k.ID, err)
	}
	if err := json.Unmarshal([]byte(bands), &k.Bands); err != nil {
		return k, fmt.Errorf("kind %s bands: %w", k.ID, err)
	}
	return k, nil
}

func (r *sqlRepository) ListKinds(ctx context.Context, campaignID string) ([]Kind, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+kindCols+`,
		        (SELECT COUNT(*) FROM trackers t WHERE t.kind_id = k.id AND t.campaign_id = k.campaign_id)
		 FROM tracker_kinds k WHERE k.campaign_id = ? ORDER BY k.name`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list tracker kinds: %w", err)
	}
	defer rows.Close()
	out := []Kind{}
	for rows.Next() {
		var k Kind
		var lo, hi, every sql.NullInt64
		var steps, bands string
		if err := rows.Scan(&k.ID, &k.CampaignID, &k.Name, &k.Shape, &lo, &hi, &steps, &bands, &k.Visibility, &every, &k.Version, &k.InUse); err != nil {
			return nil, fmt.Errorf("scan tracker kind: %w", err)
		}
		k.Min, k.Max = intPtr(lo), intPtr(hi)
		if every.Valid {
			k.EveryTypeID = int(every.Int64)
		}
		k.Steps, k.Bands = []Step{}, []Band{}
		if err := json.Unmarshal([]byte(steps), &k.Steps); err != nil {
			return nil, fmt.Errorf("kind %s steps: %w", k.ID, err)
		}
		if err := json.Unmarshal([]byte(bands), &k.Bands); err != nil {
			return nil, fmt.Errorf("kind %s bands: %w", k.ID, err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (r *sqlRepository) GetKind(ctx context.Context, campaignID, id string) (Kind, bool, error) {
	k, err := scanKind(r.db.QueryRowContext(ctx,
		`SELECT `+kindCols+` FROM tracker_kinds WHERE id = ? AND campaign_id = ?`, id, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Kind{}, false, nil
	}
	if err != nil {
		return Kind{}, false, fmt.Errorf("get tracker kind: %w", err)
	}
	return k, true, nil
}

func (r *sqlRepository) CountKinds(ctx context.Context, campaignID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tracker_kinds WHERE campaign_id = ?`, campaignID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count tracker kinds: %w", err)
	}
	return n, nil
}

func kindJSON(k Kind) (string, string, error) {
	steps, err := json.Marshal(k.Steps)
	if err != nil {
		return "", "", err
	}
	bands, err := json.Marshal(k.Bands)
	if err != nil {
		return "", "", err
	}
	return string(steps), string(bands), nil
}

func everyType(k Kind) any {
	if k.EveryTypeID == 0 {
		return nil
	}
	return k.EveryTypeID
}

func (r *sqlRepository) CreateKind(ctx context.Context, k Kind) error {
	steps, bands, err := kindJSON(k)
	if err != nil {
		return fmt.Errorf("encode tracker kind: %w", err)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO tracker_kinds (id, campaign_id, name, shape, min_value, max_value, steps, bands, visibility, every_type_id, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		k.ID, k.CampaignID, k.Name, k.Shape, nullInt(k.Min), nullInt(k.Max), steps, bands, k.Visibility, everyType(k))
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert tracker kind: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateKind(ctx context.Context, k Kind, expected int) (bool, error) {
	steps, bands, err := kindJSON(k)
	if err != nil {
		return false, fmt.Errorf("encode tracker kind: %w", err)
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE tracker_kinds SET name = ?, shape = ?, min_value = ?, max_value = ?, steps = ?, bands = ?,
		        visibility = ?, every_type_id = ?, version = version + 1
		 WHERE id = ? AND campaign_id = ? AND version = ?`,
		k.Name, k.Shape, nullInt(k.Min), nullInt(k.Max), steps, bands, k.Visibility, everyType(k),
		k.ID, k.CampaignID, expected)
	if isDuplicate(err) {
		return false, errDuplicate
	}
	if err != nil {
		return false, fmt.Errorf("update tracker kind: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update tracker kind: %w", err)
	}
	return n == 1, nil
}

func (r *sqlRepository) DeleteKind(ctx context.Context, campaignID, id string) (bool, error) {
	// One statement, so a tracker placed between a check and the delete can't
	// be swept away by the key's cascade (the cascade is there so deleting a
	// campaign never trips over the order InnoDB removes its rows in).
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM tracker_kinds WHERE id = ? AND campaign_id = ?
		 AND NOT EXISTS (SELECT 1 FROM trackers WHERE kind_id = ?)`, id, campaignID, id)
	if err != nil {
		return false, fmt.Errorf("delete tracker kind: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return false, nil
	}
	var used int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM trackers WHERE kind_id = ? AND campaign_id = ?`, id, campaignID).Scan(&used); err != nil {
		return false, fmt.Errorf("count trackers of kind: %w", err)
	}
	return used > 0, nil
}

const trackerCols = `id, campaign_id, entity_id, kind_id, name, visibility, sort_order`

func scanTracker(sc interface{ Scan(...any) error }) (Tracker, error) {
	var t Tracker
	err := sc.Scan(&t.ID, &t.CampaignID, &t.EntityID, &t.KindID, &t.Name, &t.Visibility, &t.SortOrder)
	return t, err
}

func (r *sqlRepository) ListTrackers(ctx context.Context, campaignID, entityID string) ([]Tracker, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+trackerCols+` FROM trackers WHERE campaign_id = ? AND entity_id = ?
		 ORDER BY sort_order, created_at, name`, campaignID, entityID)
	if err != nil {
		return nil, fmt.Errorf("list trackers: %w", err)
	}
	defer rows.Close()
	out := []Tracker{}
	for rows.Next() {
		t, err := scanTracker(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tracker: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *sqlRepository) getTracker(ctx context.Context, where string, args ...any) (Tracker, bool, error) {
	t, err := scanTracker(r.db.QueryRowContext(ctx, `SELECT `+trackerCols+` FROM trackers WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return Tracker{}, false, nil
	}
	if err != nil {
		return Tracker{}, false, fmt.Errorf("get tracker: %w", err)
	}
	return t, true, nil
}

func (r *sqlRepository) GetTracker(ctx context.Context, campaignID, id string) (Tracker, bool, error) {
	return r.getTracker(ctx, `id = ? AND campaign_id = ?`, id, campaignID)
}

func (r *sqlRepository) TrackerForKind(ctx context.Context, campaignID, entityID, kindID string) (Tracker, bool, error) {
	return r.getTracker(ctx, `campaign_id = ? AND entity_id = ? AND kind_id = ? ORDER BY created_at LIMIT 1`,
		campaignID, entityID, kindID)
}

func (r *sqlRepository) CreateTracker(ctx context.Context, t Tracker) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO trackers (id, campaign_id, entity_id, kind_id, name, visibility, sort_order)
		 SELECT ?, ?, ?, ?, ?, ?, COALESCE(MAX(sort_order), 0) + 1 FROM trackers WHERE entity_id = ?`,
		t.ID, t.CampaignID, t.EntityID, t.KindID, t.Name, t.Visibility, t.EntityID)
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("insert tracker: %w", err)
	}
	return nil
}

func (r *sqlRepository) UpdateTracker(ctx context.Context, t Tracker) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE trackers SET name = ?, visibility = ? WHERE id = ? AND campaign_id = ?`,
		t.Name, t.Visibility, t.ID, t.CampaignID)
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("update tracker: %w", err)
	}
	return nil
}

func (r *sqlRepository) DeleteTracker(ctx context.Context, campaignID, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM trackers WHERE id = ? AND campaign_id = ?`, id, campaignID); err != nil {
		return fmt.Errorf("delete tracker: %w", err)
	}
	return nil
}

const entryCols = `id, tracker_id, op, amount, note, source, game_year, game_month, game_day, game_label, COALESCE(created_by, ''), created_at, struck_at`

func scanEntry(sc interface{ Scan(...any) error }) (Entry, error) {
	var e Entry
	var y, m, d sql.NullInt64
	var label string
	var struck sql.NullTime
	if err := sc.Scan(&e.ID, &e.TrackerID, &e.Op, &e.Amount, &e.Note, &e.Source, &y, &m, &d, &label, &e.CreatedBy, &e.CreatedAt, &struck); err != nil {
		return e, err
	}
	if y.Valid && m.Valid && d.Valid {
		e.Game = &GameDate{Year: int(y.Int64), Month: int(m.Int64), Day: int(d.Int64), Label: label}
	}
	if struck.Valid {
		t := struck.Time
		e.StruckAt = &t
	}
	return e, nil
}

func (r *sqlRepository) ListEntries(ctx context.Context, campaignID string, trackerIDs []string) (map[string][]Entry, error) {
	out := map[string][]Entry{}
	if len(trackerIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(trackerIDs)+1)
	args = append(args, campaignID)
	for _, id := range trackerIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+entryCols+` FROM tracker_entries
		 WHERE campaign_id = ? AND tracker_id IN (?`+strings.Repeat(",?", len(trackerIDs)-1)+`)
		 ORDER BY tracker_id, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list tracker entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tracker entry: %w", err)
		}
		out[e.TrackerID] = append(out[e.TrackerID], e)
	}
	return out, rows.Err()
}

func (r *sqlRepository) CountEntries(ctx context.Context, campaignID, trackerID string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tracker_entries WHERE campaign_id = ? AND tracker_id = ?`, campaignID, trackerID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count tracker entries: %w", err)
	}
	return n, nil
}

func (r *sqlRepository) AddEntry(ctx context.Context, campaignID string, e Entry) error {
	var y, m, d any
	label := ""
	if e.Game != nil {
		y, m, d, label = e.Game.Year, e.Game.Month, e.Game.Day, e.Game.Label
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tracker_entries (tracker_id, campaign_id, op, amount, note, source, game_year, game_month, game_day, game_label, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TrackerID, campaignID, e.Op, e.Amount, e.Note, e.Source, y, m, d, label, nullString(e.CreatedBy))
	if err != nil {
		return fmt.Errorf("insert tracker entry: %w", err)
	}
	return nil
}

func (r *sqlRepository) GetEntry(ctx context.Context, campaignID string, id int64) (Entry, bool, error) {
	e, err := scanEntry(r.db.QueryRowContext(ctx,
		`SELECT `+entryCols+` FROM tracker_entries WHERE id = ? AND campaign_id = ?`, id, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("get tracker entry: %w", err)
	}
	return e, true, nil
}

func (r *sqlRepository) StrikeEntry(ctx context.Context, campaignID string, id int64, struck bool, userID string) error {
	var at, by any
	if struck {
		at, by = time.Now().UTC(), nullString(userID)
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE tracker_entries SET struck_at = ?, struck_by = ? WHERE id = ? AND campaign_id = ?`,
		at, by, id, campaignID); err != nil {
		return fmt.Errorf("strike tracker entry: %w", err)
	}
	return nil
}
